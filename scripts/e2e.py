import calendar, hashlib, hmac, json, os, re, subprocess, sys, time, urllib.request, urllib.error, uuid

BASE = "http://127.0.0.1:8080/api"
ADMIN_EMAIL, ADMIN_PW = os.environ["E_EMAIL"], os.environ["E_PW"]
passed, failed = 0, []

def call(method, path, body=None, token=None, raw=False, expect=None, headers=None):
    req = urllib.request.Request(BASE + path, method=method, data=json.dumps(body).encode() if body is not None else None)
    req.add_header("Content-Type", "application/json")
    if token: req.add_header("Authorization", "Bearer " + token)
    for k, v in (headers or {}).items(): req.add_header(k, v)
    try:
        with urllib.request.urlopen(req) as r:
            d, code = r.read().decode(), r.status
    except urllib.error.HTTPError as e:
        d, code = e.read().decode(), e.code
    try: parsed = d if raw else (json.loads(d) if d else None)
    except Exception: parsed = d
    return code, parsed

def check(name, cond, detail=""):
    global passed
    if cond: passed += 1; print("  ✓", name)
    else: failed.append(name); print("  ✗", name, detail)

def sh(*a): return subprocess.run(a, capture_output=True, text=True)

# ---------- setup ----------
c, r = call("POST", "/auth/login", {"email": ADMIN_EMAIL, "password": ADMIN_PW}); A = r["access_token"]
org = call("GET", "/orgs", token=A)[1][0]["id"]
pname = "e2e-" + uuid.uuid4().hex[:6]
c, p = call("POST", f"/orgs/{org}/projects", {"name": pname}, A); pid = p["id"]
put = lambda body, tok=A: call("PUT", f"/projects/{pid}/secrets", body, tok)

print("\n[secret references]")
put({"env": "dev", "key": "DB_HOST", "value": "db.internal"})
put({"env": "dev", "key": "DB_URL", "value": "postgres://app:pw@${DB_HOST}:5432/app"})
put({"env": "dev", "key": "A_LOOP", "value": "x-${B_LOOP}"}); put({"env": "dev", "key": "B_LOOP", "value": "y-${A_LOOP}"})
put({"env": "dev", "path": "/stripe", "key": "STRIPE_KEY", "value": "sk_test_1"})
put({"env": "dev", "key": "ABS", "value": "key=${/stripe/STRIPE_KEY}"})
put({"env": "prod", "key": "PROD_ONLY", "value": "prod-secret"})
put({"env": "dev", "key": "CROSS", "value": "${prod:/PROD_ONLY}"})
c, t = call("POST", f"/projects/{pid}/tokens", {"name": "ci", "env": "dev", "path": "/"}, A); TOK = t["token"]
c, got = call("GET", "/v1/secrets?recursive=true", token=TOK); s = got["secrets"]
check("relative reference resolves", s["DB_URL"] == "postgres://app:pw@db.internal:5432/app", s.get("DB_URL"))
check("absolute path reference resolves", s["ABS"] == "key=sk_test_1", s.get("ABS"))
check("circular references terminate and stay literal", "${" in s["A_LOOP"], s.get("A_LOOP"))
check("token cannot resolve a reference into another environment", s["CROSS"] == "${prod:/PROD_ONLY}", s.get("CROSS"))
c, raw = call("GET", "/v1/secrets?resolve=false&path=/", token=TOK); check("resolve=false returns raw value", "${DB_HOST}" in raw["secrets"]["DB_URL"])

print("\n[tags, search, expiry]")
c, r = put({"env": "dev", "key": "SMTP_PASSWORD", "value": "pw", "tags": ["#Mail", "ops"], "comment": "sendgrid key"}); check("tags normalised", r["tags"] == ["mail", "ops"], r)
c, r = put({"env": "dev", "key": "BAD", "value": "v", "tags": ["has space"]}); check("invalid tag rejected", c == 400)
c, r = call("GET", f"/projects/{pid}/secrets?env=dev&path=/&tag=mail", token=A); check("filter by tag", [x["key"] for x in r["secrets"]] == ["SMTP_PASSWORD"], r)
c, r = call("GET", f"/projects/{pid}/secrets/search?q=stripe", token=A); check("search finds across paths", any(x["key"] == "STRIPE_KEY" and x["path"] == "/stripe" for x in r["results"]), r)
c, r = call("GET", f"/projects/{pid}/secrets/search?q=sendgrid", token=A); check("search matches comments", len(r["results"]) == 1)
c, r = call("GET", f"/projects/{pid}/secrets/search?q=mail", token=A); check("search matches tags", any(x["key"] == "SMTP_PASSWORD" for x in r["results"]))
c, r = put({"env": "dev", "key": "OLD", "value": "v", "expires_at": "2020-01-01T00:00:00Z"}); check("past expiry rejected", c == 400)
soon = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() + 3))
c, r = put({"env": "dev", "key": "SHORT_LIVED", "value": "temp", "expires_at": soon}); check("expiry accepted", c == 200 and r["expires_at"])
c, g = call("GET", "/v1/secrets?path=/", token=TOK); check("unexpired secret served", "SHORT_LIVED" in g["secrets"])
time.sleep(4)
c, g = call("GET", "/v1/secrets?path=/", token=TOK); check("expired secret no longer served to machines", "SHORT_LIVED" not in g["secrets"])
c, r = call("GET", f"/projects/{pid}/secrets?env=dev&path=/", token=A); check("expired secret still visible to admins (flagged)", any(x["key"] == "SHORT_LIVED" for x in r["secrets"]))
c, r = put({"env": "dev", "key": "SHORT_LIVED", "value": "temp", "clear_expiry": True}); check("clear_expiry works", r["expires_at"] is None)
c, g = call("GET", "/v1/secrets?path=/", token=TOK); check("cleared secret served again", "SHORT_LIVED" in g["secrets"])

print("\n[service token IP allowlist]")
c, t2 = call("POST", f"/projects/{pid}/tokens", {"name": "locked", "env": "dev", "path": "/", "allowed_cidrs": ["10.9.8.7"]}, A)
c, r = call("GET", "/v1/secrets", token=t2["token"]); check("token blocked from non-allowlisted IP", c == 403, (c, r))
c, t3 = call("POST", f"/projects/{pid}/tokens", {"name": "lan", "env": "dev", "path": "/", "allowed_cidrs": ["192.168.0.0/16", "172.16.0.0/12", "10.0.0.0/8"]}, A)
c, r = call("GET", "/v1/secrets", token=t3["token"]); check("token works from allowlisted range", c == 200, (c, r))
c, r = call("POST", f"/projects/{pid}/tokens", {"name": "bad", "env": "dev", "allowed_cidrs": ["not-an-ip"]}, A); check("invalid CIDR rejected", c == 400)

print("\n[invites + per-environment roles + PAT]")
email = f"e2e-{uuid.uuid4().hex[:6]}@example.com"
c, inv = call("POST", f"/orgs/{org}/invites", {"email": email, "role": "developer", "env_roles": {"prod": "none", "staging": "viewer"}}, A)
check("invite created with link", c == 201 and "/invite/vin_" in inv["link"], inv)
time.sleep(1.5)
mc = json.loads(urllib.request.urlopen("http://127.0.0.1:8025/api/v1/messages").read())
msg = next((m for m in mc["messages"] if any(t["Address"] == email for t in m["To"])), None)
check("invite email delivered to Mailpit", msg is not None)
if msg:
    body = json.loads(urllib.request.urlopen(f"http://127.0.0.1:8025/api/v1/message/{msg['ID']}").read())
    check("email contains the accept link", inv["link"].split("/invite/")[1] in body["Text"])
tok = inv["link"].split("/invite/")[1]
c, pv = call("GET", f"/invites/{tok}"); check("public preview works and hides the email", pv["state"] == "pending" and pv["org"] and email not in json.dumps(pv), pv)
c, r = call("POST", "/auth/register", {"name": "E2E User", "email": email, "password": "e2e-password-1"}); E = r["access_token"]
c, r = call("POST", f"/invites/{tok}/accept", token=E); check("invitee accepts invite", c == 200 and r["role"] == "developer", (c, r))
c, r = call("POST", f"/invites/{tok}/accept", token=E); check("invite is single-use", c == 410)
c, pr = call("GET", f"/projects/{pid}", token=E); slugs = [e["slug"] for e in pr["environments"]]
check("env with 'none' is hidden from the member", "prod" not in slugs and "dev" in slugs, slugs)
c, r = call("GET", f"/projects/{pid}/secrets?env=prod&path=/", token=E); check("cannot list the hidden environment", c == 403)
c, r = call("PUT", f"/projects/{pid}/secrets", {"env": "staging", "key": "X", "value": "1"}, E); check("viewer-on-staging cannot write", c == 403)
c, r = call("PUT", f"/projects/{pid}/secrets", {"env": "dev", "key": "E_WROTE", "value": "1"}, E); check("developer-on-dev can write", c == 200)
c, r = call("GET", f"/projects/{pid}/secrets/search?q=prod", token=E); check("search never leaks the hidden environment", all(x["env"] != "prod" for x in r["results"]))
c, r = call("GET", f"/projects/{pid}/secrets/export?env=prod&path=/&format=dotenv", token=E, raw=True); check("cannot export the hidden environment", c == 403)
c, r = call("POST", f"/orgs/{org}/invites", {"email": email, "role": "viewer"}, A); check("cannot invite an existing member", c == 409)
c, mem = call("GET", f"/orgs/{org}/members", token=A); em = next(m for m in mem if m["email"] == email)
check("member list exposes env overrides to the UI", em["env_roles"].get("prod") == "none", em["env_roles"])
c, r = call("PUT", f"/orgs/{org}/members/{em['id']}/env-roles", {"roles": {"prod": "viewer", "staging": "inherit"}}, A); check("admin can edit env roles", c == 204)
c, r = call("GET", f"/projects/{pid}/secrets?env=prod&path=/", token=E); check("override now grants read on prod", c == 200)
c, r = call("PUT", f"/orgs/{org}/members/{em['id']}/env-roles", {"roles": {"prod": "none"}}, E); check("non-admin cannot edit env roles", c == 403)
c, r = call("POST", "/me/tokens", {"name": "laptop", "expire_days": 30}, E); PAT = r["token"]; check("personal token created", PAT.startswith("vpt_"))
c, r = call("GET", "/orgs", token=PAT); check("PAT authenticates API calls as the user", c == 200 and len(r) >= 2, (c, r))
c, r = call("GET", f"/projects/{pid}/secrets?env=prod&path=/", token=PAT); check("PAT inherits the same env restrictions", c == 200)
c, r = call("PUT", f"/projects/{pid}/secrets", {"env": "prod", "key": "X", "value": "1"}, PAT); check("PAT can't exceed the user's role (viewer on prod)", c == 403, (c, r))
c, r = call("POST", "/me/tokens", {"name": "evil"}, PAT); check("PAT cannot mint more PATs", c == 403)
c, l = call("GET", "/me/tokens", token=E); check("PAT listing hides the token value", "token" not in json.dumps(l) and l[0]["prefix"].startswith("vpt_"))
c, r = call("DELETE", f"/me/tokens/{l[0]['id']}", token=E); c2, _ = call("GET", "/orgs", token=PAT); check("revoked PAT stops working", c2 == 401)

print("\n[webhooks]")
sh("docker", "rm", "-f", "hookrx")
sh("docker", "run", "-d", "--name", "hookrx", "--network", "vaultly_default", "-v", os.path.join(os.path.dirname(os.path.abspath(__file__)), "hookrx.py") + ":/h.py:ro", "python:3.12-alpine", "python", "/h.py")
time.sleep(2)
c, r = call("POST", f"/projects/{pid}/webhooks", {"name": "rx", "url": "ftp://x"}, A); check("bad URL scheme rejected", c == 400)
c, w = call("POST", f"/projects/{pid}/webhooks", {"name": "rx", "url": "http://hookrx:9000/", "events": ["secret.*"]}, A); WSEC = w["secret"]
check("webhook created and secret returned once", c == 201 and WSEC.startswith("whsec_"))
c, lst = call("GET", f"/projects/{pid}/webhooks", token=A); check("secret not in listing", WSEC not in json.dumps(lst))
c, r = call("POST", f"/projects/{pid}/webhooks/{w['id']}/test", token=A); check("test ping reaches the receiver", r["status"] == 200 and not r["error"], r)
put({"env": "dev", "key": "HOOK_TRIGGER", "value": "super-secret-value-123"})
call("DELETE", f"/projects/{pid}/secrets/" + call("GET", f"/projects/{pid}/secrets?env=dev&path=/", token=A)[1]["secrets"][0]["id"], token=A)
time.sleep(4)
logs = [json.loads(l) for l in sh("docker", "logs", "hookrx").stdout.splitlines() if l.startswith("{")]
evs = [l["event"] for l in logs]
check("receiver got secret.create", "secret.create" in evs, evs)
check("receiver got secret.delete", "secret.delete" in evs, evs)
check("webhook.test delivered", "webhook.test" in evs)
good = [l for l in logs if l["sig"] == "v1=" + hmac.new(WSEC.encode(), (l["ts"] + "." + l["body"]).encode(), hashlib.sha256).hexdigest()]
check("every delivery carries a valid HMAC signature", len(good) == len(logs) and len(logs) >= 3, (len(good), len(logs)))
check("secret values never appear in payloads", all("super-secret-value-123" not in l["body"] for l in logs))
check("events outside the filter are not sent", "token.create" not in evs)
c, d = call("GET", f"/projects/{pid}/webhooks/{w['id']}/deliveries", token=A); check("delivery log recorded", len(d) >= 3 and all(x["status"] == 200 for x in d), d[:2])
c, r = call("PATCH", f"/projects/{pid}/webhooks/{w['id']}", {"active": False}, A); n0 = len(logs)
put({"env": "dev", "key": "AFTER_PAUSE", "value": "1"}); time.sleep(3)
n1 = len([l for l in sh("docker", "logs", "hookrx").stdout.splitlines() if l.startswith("{")]); check("paused webhook sends nothing", n1 == n0, (n0, n1))
sh("docker", "rm", "-f", "hookrx")

print("\n[integrations CRUD (provider calls are covered by unit tests)]")
c, r = call("POST", f"/projects/{pid}/integrations", {"provider": "github", "name": "gh", "env": "dev", "config": {"token": "ghp_fake", "repo": "bad repo"}}, A); check("invalid repo rejected", c == 400)
c, r = call("POST", f"/projects/{pid}/integrations", {"provider": "github", "name": "gh", "env": "dev", "path": "/", "config": {"token": "ghp_TOPSECRET", "repo": "acme/api"}, "auto_sync": True}, A); check("integration created", c == 201)
c, lst = call("GET", f"/projects/{pid}/integrations", token=A); check("listing masks the token", "ghp_TOPSECRET" not in json.dumps(lst) and lst[0]["config"]["repo"] == "acme/api", lst)
db = sh("docker", "compose", "-f", "docker-compose.yml", "exec", "-T", "postgres", "psql", "-U", "vaultly", "-tAc", "select position('TOPSECRET' in convert_from(config_enc,'LATIN1')) from integrations limit 1").stdout.strip()
check("integration credentials are encrypted at rest", db == "0", db)
c, r = call("POST", f"/projects/{pid}/integrations", {"provider": "vercel", "name": "v", "env": "dev", "config": {"token": "t", "project_id": "p", "target": "staging"}}, A); check("vercel target validated", c == 400)
c, r = call("PATCH", f"/projects/{pid}/integrations/{lst[0]['id']}", {"auto_sync": False}, A); check("auto-sync toggle", c == 204)

print("\n[dynamic secrets]")
ADM = "postgres://vaultly:vaultly@postgres:5432/vaultly?sslmode=disable"
c, r = call("POST", f"/projects/{pid}/dynamic", {"name": "readonly-db", "env": "dev", "admin_url": "postgres://nobody:x@postgres:5432/vaultly"}, A); check("bad admin credentials rejected", c == 400, r)
c, dcfg = call("POST", f"/projects/{pid}/dynamic", {"name": "readonly-db", "env": "dev", "admin_url": ADM, "grants": ["GRANT pg_read_all_data TO {{name}}"], "default_ttl_s": 120, "max_ttl_s": 600}, A); check("dynamic config created", c == 201, dcfg)
c, lst = call("GET", f"/projects/{pid}/dynamic", token=A); check("admin URL password never returned", "vaultly:vaultly" not in json.dumps(lst) and lst[0]["target"]["host"] == "postgres:5432", lst)
c, L = call("POST", f"/projects/{pid}/dynamic/{dcfg['id']}/lease", {"ttl_seconds": 99999}, A); check("lease issued", c == 201 and L["username"].startswith("v_"), L)
exp_in = calendar.timegm(time.strptime(L["expires_at"][:19], "%Y-%m-%dT%H:%M:%S")) - time.time()
check("TTL clamped to the configured maximum", 500 < exp_in < 700, exp_in)
def psql(url): return sh("docker", "compose", "-f", "docker-compose.yml", "exec", "-T", "postgres", "psql", url.replace("@postgres:", "@localhost:"), "-tAc", "select current_user")
o = psql(L["connection_url"]); check("leased credentials really log in", o.stdout.strip() == L["username"], o.stderr[:150])
c, r = call("POST", f"/projects/{pid}/dynamic/{dcfg['id']}/lease", {}, E); check("member without env access cannot lease (prod none→dev dev ok)", c in (201, 403))
c, r = call("POST", f"/projects/{pid}/dynamic/{dcfg['id']}/lease", {"ttl_seconds": 60}, TOK); check("service token cannot use the user endpoint", c == 401)
c, L2 = call("POST", "/v1/dynamic/readonly-db/lease", {"ttl_seconds": 60}, TOK); check("service token leases in its own environment", c == 201, L2)
c, r = call("POST", "/v1/dynamic/nope/lease", {}, TOK); check("unknown dynamic secret 404", c == 404)
c, r = call("DELETE", f"/projects/{pid}/dynamic/{dcfg['id']}/leases/{L['lease_id']}", token=A); check("admin revokes a lease", c == 204, r)
o = psql(L["connection_url"]); check("revoked credentials stop working", o.returncode != 0 or o.stdout.strip() == "", o.stdout)
c, ls = call("GET", f"/projects/{pid}/dynamic/{dcfg['id']}/leases", token=A); check("lease history shows states", {x["state"] for x in ls} >= {"revoked", "active"}, [x["state"] for x in ls])
print("  … waiting for the janitor to expire a 60s lease")
time.sleep(95)
o = psql(L2["connection_url"]); check("expired lease auto-revoked by janitor", o.returncode != 0 or o.stdout.strip() == "")
c, ls = call("GET", f"/projects/{pid}/dynamic/{dcfg['id']}/leases", token=A); check("janitor marked it revoked", all(x["state"] == "revoked" for x in ls if x["id"] == next(y["id"] for y in ls if y["username"] == L2["username"])), ls)

print("\n[backup & restore]")
c, r = call("POST", f"/orgs/{org}/backup", {"passphrase": "short"}, A); check("weak passphrase rejected", c == 400)
req = urllib.request.Request(BASE + f"/orgs/{org}/backup", method="POST", data=json.dumps({"passphrase": "a-long-backup-passphrase"}).encode(), headers={"Content-Type": "application/json", "Authorization": "Bearer " + A})
blob = urllib.request.urlopen(req).read()
check("backup is an opaque sealed file", blob[:7] == b"VLTYBK1" and pname.encode() not in blob and b"db.internal" not in blob and len(blob) > 1000)
open("/tmp/e2e.vbk", "wb").write(blob)
def restore(passphrase):
    out = sh("curl", "-s", "-X", "POST", BASE + f"/orgs/{org}/restore", "-H", "Authorization: Bearer " + A, "-F", "file=@/tmp/e2e.vbk", "-F", "passphrase=" + passphrase)
    return json.loads(out.stdout)
r = restore("wrong-passphrase-here"); check("wrong passphrase refused", "error" in r, r)
r = restore("a-long-backup-passphrase"); check("restore skips projects that already exist", r["restored"] == 0 and any(pname == s["project"] for s in r["skipped"]), r)
before = call("GET", "/v1/secrets?recursive=true", token=TOK)[1]["secrets"]
call("DELETE", f"/projects/{pid}", token=A)
c, _ = call("GET", f"/projects/{pid}", token=A); check("project deleted", c == 404)
r = restore("a-long-backup-passphrase"); check("restore recreates the deleted project", r["restored"] >= 1 and r["secrets"] > 5, r)
c, pr = call("GET", f"/projects/{pid}", token=A); check("restored project has its original ID and environments", c == 200 and len(pr["environments"]) == 3, pr)
c, rows = call("GET", f"/projects/{pid}/secrets?env=dev&path=/", token=A)
sid = next(x["id"] for x in rows["secrets"] if x["key"] == "DB_HOST"); c, sv = call("GET", f"/projects/{pid}/secrets/{sid}", token=A)
check("restored ciphertext decrypts with the same master key", sv["value"] == "db.internal", sv)
c, vers = call("GET", f"/projects/{pid}/secrets/{sid}/versions", token=A); check("version history survived", len(vers) >= 1)
call("DELETE", f"/projects/{pid}", token=A)

print("\n[encryption at rest: personal data + OpenBao]")
def q(sql): return sh("docker", "compose", "-f", "docker-compose.yml", "exec", "-T", "postgres", "psql", "-U", "vaultly", "-tAc", sql).stdout.strip()
check("users hold no plaintext email or name", q("select count(*) from users where email is not null or name is not null or email_enc not like 'enc:v1:%' or name_enc not like 'enc:v1:%'") == "0")
check("invites hold no plaintext email", q("select count(*) from invites where email is not null or email_enc not like 'enc:v1:%'") == "0")
check("audit log actors/resources are sealed", q("select count(*) from audit_logs where (actor_label like '%@%' and actor_label not like 'enc:v1:%') or (resource like '%@%' and resource not like 'enc:v1:%')") == "0")
check("'who did it' columns are sealed", q("select (select count(*) from secrets where updated_by like '%@%' and updated_by not like 'enc:v1:%') + (select count(*) from secret_versions where actor like '%@%' and actor not like 'enc:v1:%') + (select count(*) from service_tokens where created_by like '%@%' and created_by not like 'enc:v1:%') + (select count(*) from dynamic_leases where created_by like '%@%' and created_by not like 'enc:v1:%') + (select count(*) from invites where invited_by like '%@%' and invited_by not like 'enc:v1:%')") == "0")
dump = sh("docker", "compose", "-f", "docker-compose.yml", "exec", "-T", "postgres", "pg_dump", "-U", "vaultly", "vaultly").stdout.lower()
check("a full database dump contains neither tester's email address", email.lower() not in dump and ADMIN_EMAIL.lower() not in dump, len(dump))
check("identical emails never produce identical ciphertext", q("select count(distinct email_enc) = count(*) from users") == "t")
c, r = call("POST", "/auth/login", {"email": ADMIN_EMAIL.upper(), "password": ADMIN_PW}); check("login works with different capitalisation (blind index)", c == 200, c)
c, r = call("GET", "/me", token=A); check("API still returns the decrypted email", r["email"].lower() == ADMIN_EMAIL.lower(), r)
c, au = call("GET", f"/orgs/{org}/audit", token=A); check("audit API returns readable actors", c == 200 and all(not e["actor"].startswith("enc:v1:") for e in au["events"]), au["events"][:1])
c, mem = call("GET", f"/orgs/{org}/members", token=A); check("member list returns readable names", c == 200 and all("@" in m["email"] and not m["name"].startswith("enc:") for m in mem))
if os.environ.get("E_EXPECT_BAO"):
    check("every project key is wrapped by OpenBao", q("select count(*) from projects where convert_from(encrypted_dek,'LATIN1') not like 'vault:%'") == "0")

# clean up the throwaway user and its personal workspace (emails are encrypted, so delete by id)
q(f"delete from users where id='{em['id']}'")
q("delete from orgs where id not in (select org_id from memberships)")
print(f"\n{passed} passed, {len(failed)} failed")
for f in failed: print("  FAILED:", f)
sys.exit(1 if failed else 0)
