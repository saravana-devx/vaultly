import json, os, random, urllib.request, urllib.error, time
BASE = "http://127.0.0.1:8080/api"
random.seed(11)
def call(m, path, body=None, tok=None):
    r = urllib.request.Request(BASE + path, method=m, data=json.dumps(body).encode() if body is not None else None)
    r.add_header("Content-Type", "application/json")
    if tok: r.add_header("Authorization", "Bearer " + tok)
    try:
        with urllib.request.urlopen(r) as x: d = x.read().decode(); return x.status, (json.loads(d) if d else None)
    except urllib.error.HTTPError as e: return e.code, e.read().decode()
c, r = call("POST", "/auth/login", {"email": os.environ["E"], "password": os.environ["P"]}); A = r["access_token"]
org = call("GET", "/orgs", tok=A)[1][0]["id"]
projs = {p["name"]: p for p in call("GET", f"/orgs/{org}/projects", tok=A)[1]}
pid = projs["payments-api"]["id"]; web = projs["web-app"]["id"]

def put(project, env, key, value, **kw):
    return call("PUT", f"/projects/{project}/secrets", {"env": env, "key": key, "value": value, **kw}, A)

# references, tags, expiry on payments-api
for env in ("dev", "staging", "prod"):
    host = {"dev": "pg.dev.internal", "staging": "pg.stg.internal", "prod": "pg.prod.internal"}[env]
    put(pid, env, "PG_HOST", host, tags=["database"], comment="Shared by every connection string below")
    put(pid, env, "DATABASE_URL", "postgres://payments_app:" + "x" * 12 + "@${PG_HOST}:5432/payments?sslmode=require", tags=["database"], comment="Built from PG_HOST")
    put(pid, env, "REPLICA_URL", "postgres://readonly:" + "y" * 12 + "@${/database/replica/DB_REPLICA_HOST}:5432/payments", tags=["database"])
in_days = lambda d: time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() + d * 86400))
put(pid, "prod", "STRIPE_SECRET_KEY", "sk_live_" + "a1b2c3d4" * 6, tags=["stripe", "payments"], expires_at=in_days(5), comment="Rotate before the date shown")
put(pid, "staging", "SENDGRID_TEMP_KEY", "SG.temporary-demo-key", tags=["mail"], expires_at=in_days(2))
put(pid, "dev", "STRIPE_SECRET_KEY", "sk_test_" + "z9y8x7w6" * 6, tags=["stripe", "payments"])
put(pid, "prod", "JWT_SIGNING_KEY", "k" * 64, tags=["auth"], expires_at=in_days(11))
put(web, "prod", "SESSION_SECRET", "s" * 48, tags=["auth"], expires_at=in_days(30))
put(web, "dev", "NEXT_PUBLIC_API_URL", "http://localhost:8080", tags=["frontend"])

# webhook (local receiver), dynamic secret (compose postgres), integration (no auto-sync, never called)
call("POST", f"/projects/{pid}/webhooks", {"name": "Audit relay (local test receiver)", "url": "http://hookrx:9000/", "events": ["secret.*", "token.*"]}, A)
call("POST", f"/projects/{pid}/dynamic", {"name": "readonly-db", "env": "dev", "admin_url": "postgres://vaultly:vaultly@postgres:5432/vaultly?sslmode=disable",
     "grants": ["GRANT pg_read_all_data TO {{name}}"], "default_ttl_s": 3600, "max_ttl_s": 86400}, A)
call("POST", f"/projects/{web}/integrations", {"provider": "vercel", "name": "Vercel – northwind-web", "env": "prod", "path": "/", "auto_sync": False,
     "config": {"token": "demo-not-a-real-token", "project_id": "prj_northwind", "target": "production"}}, A)
call("POST", f"/projects/{web}/integrations", {"provider": "github", "name": "GitHub Actions – northwind/web", "env": "staging", "path": "/", "auto_sync": False,
     "config": {"token": "demo-not-a-real-token", "repo": "northwind/web"}}, A)

# restrict one teammate and add a pending invite
mem = {m["email"]: m for m in call("GET", f"/orgs/{org}/members", tok=A)[1]}
if "sofia.alvarez@vaultly.dev" in mem: call("PUT", f"/orgs/{org}/members/{mem['sofia.alvarez@vaultly.dev']['id']}/env-roles", {"roles": {"prod": "none", "staging": "viewer"}}, A)
if "priya.nair@vaultly.dev" in mem: call("PUT", f"/orgs/{org}/members/{mem['priya.nair@vaultly.dev']['id']}/env-roles", {"roles": {"prod": "viewer"}}, A)
print(call("POST", f"/orgs/{org}/invites", {"email": "contractor@example.com", "role": "developer", "env_roles": {"prod": "none", "staging": "none"}}, A)[0])
print("done")
