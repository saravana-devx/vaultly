import json, os, random, secrets, string, sys, urllib.request, urllib.error

BASE = "http://127.0.0.1:8080/api"
EMAIL, PASSWORD = os.environ["SEED_EMAIL"], os.environ["SEED_PASSWORD"]
random.seed(7)

def call(method, path, body=None, token=None, raw=False):
    req = urllib.request.Request(BASE + path, method=method, data=json.dumps(body).encode() if body is not None else None)
    req.add_header("Content-Type", "application/json")
    if token: req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req) as r:
            d = r.read().decode()
            return d if raw else (json.loads(d) if d else None)
    except urllib.error.HTTPError as e:
        raise SystemExit(f"{method} {path} -> {e.code} {e.read().decode()}")

def rnd(n=32, alpha=string.ascii_letters + string.digits): return "".join(secrets.choice(alpha) for _ in range(n))
def hexs(n): return secrets.token_hex(n // 2)

# ---- people ----
me = call("POST", "/auth/login", {"email": EMAIL, "password": PASSWORD})
admin = me["access_token"]
org = call("GET", "/orgs", token=admin)[0]["id"]
team = [("Priya Nair", "priya.nair@vaultly.dev", "developer"), ("Arjun Mehta", "arjun.mehta@vaultly.dev", "developer"),
        ("Sofia Alvarez", "sofia.alvarez@vaultly.dev", "viewer"), ("Daniel Okafor", "daniel.okafor@vaultly.dev", "admin")]
tok = {"admin": admin}
for name, email, role in team:
    pw = rnd(24)
    try:
        r = call("POST", "/auth/register", {"name": name, "email": email, "password": pw, "org_name": name.split()[0] + "'s sandbox"})
    except SystemExit:
        continue  # already seeded
    tok[email] = r["access_token"]
    call("POST", f"/orgs/{org}/members", {"email": email, "role": role}, admin)
print("team ready")

# ---- projects ----
H = {"dev": "dev", "staging": "stg", "prod": "prod"}
def hosts(env, svc): return f"{svc}.{H[env]}.internal"
def pg(env, db, user="app"): return f"postgres://{user}:{rnd(20)}@{hosts(env,'pg')}:5432/{db}?sslmode=require"

def payments(env):
    live = "live" if env == "prod" else "test"
    return {"/": {"DATABASE_URL": pg(env, "payments"), "REDIS_URL": f"redis://:{rnd(20)}@{hosts(env,'redis')}:6379/0", "JWT_SIGNING_KEY": rnd(64),
                  "SENTRY_DSN": f"https://{hexs(32)}@o{random.randint(10000,99999)}.ingest.sentry.io/{random.randint(1000000,9999999)}",
                  "LOG_LEVEL": "info" if env == "prod" else "debug", "PORT": "8080"},
            "/stripe": {"STRIPE_SECRET_KEY": f"sk_{live}_{rnd(48)}", "STRIPE_PUBLISHABLE_KEY": f"pk_{live}_{rnd(48)}", "STRIPE_WEBHOOK_SECRET": f"whsec_{rnd(40)}"},
            "/database": {"DB_HOST": hosts(env, "pg"), "DB_USER": "payments_app", "DB_PASSWORD": rnd(28, string.ascii_letters + string.digits + "!#%^*_-"), "DB_NAME": "payments"},
            "/database/replica": {"DB_REPLICA_HOST": hosts(env, "pg-ro"), "DB_REPLICA_PASSWORD": rnd(28)},
            "/smtp": {"SMTP_HOST": "smtp.sendgrid.net", "SMTP_USER": "apikey", "SMTP_PASSWORD": f"SG.{rnd(22)}.{rnd(43)}"}}

def webapp(env):
    d = {"dev": "localhost:3000", "staging": "staging.northwind.app", "prod": "northwind.app"}[env]
    return {"/": {"NEXT_PUBLIC_API_URL": f"https://api.{d}" if env != "dev" else "http://localhost:8080", "SESSION_SECRET": rnd(48), "DATABASE_URL": pg(env, "web"),
                  "NEXT_PUBLIC_POSTHOG_KEY": f"phc_{rnd(43)}", "SENTRY_AUTH_TOKEN": f"sntrys_{rnd(64)}", "NODE_ENV": "production" if env != "dev" else "development"},
            "/auth": {"GOOGLE_CLIENT_ID": f"{random.randint(10**11,10**12)}-{rnd(32,string.ascii_lowercase+string.digits)}.apps.googleusercontent.com", "GOOGLE_CLIENT_SECRET": f"GOCSPX-{rnd(28)}",
                      "GITHUB_CLIENT_ID": rnd(20, string.hexdigits.lower()), "GITHUB_CLIENT_SECRET": hexs(40)},
            "/storage": {"S3_BUCKET": f"northwind-{H[env]}-uploads", "AWS_ACCESS_KEY_ID": "AKIA" + rnd(16, string.ascii_uppercase + string.digits), "AWS_SECRET_ACCESS_KEY": rnd(40, string.ascii_letters + string.digits + "+/"), "AWS_REGION": "us-east-1"},
            "/email": {"RESEND_API_KEY": f"re_{rnd(32)}"}}

def pipeline(env):
    return {"/": {"KAFKA_BROKERS": f"kafka-0.{H[env]}.internal:9092,kafka-1.{H[env]}.internal:9092", "WAREHOUSE_URL": f"snowflake://etl:{rnd(20)}@acme-{H[env]}/ANALYTICS", "AIRFLOW_FERNET_KEY": rnd(43) + "=", "DBT_PROFILES_TOKEN": rnd(40)},
            "/aws": {"AWS_ACCESS_KEY_ID": "AKIA" + rnd(16, string.ascii_uppercase + string.digits), "AWS_SECRET_ACCESS_KEY": rnd(40), "S3_RAW_BUCKET": f"acme-{H[env]}-raw", "GLUE_ROLE_ARN": f"arn:aws:iam::{random.randint(10**11,10**12-1)}:role/etl-{H[env]}"},
            "/kafka": {"KAFKA_SASL_USERNAME": f"etl-{H[env]}", "KAFKA_SASL_PASSWORD": rnd(32), "SCHEMA_REGISTRY_KEY": rnd(24)}}

def mobile(env):
    return {"/": {"DATABASE_URL": pg(env, "mobile"), "JWT_SECRET": rnd(64), "API_RATE_LIMIT": "600" if env == "prod" else "6000", "CDN_BASE_URL": f"https://cdn-{H[env]}.example.com"},
            "/push": {"FCM_SERVER_KEY": f"AAAA{rnd(7)}:{rnd(140)}", "APNS_KEY_ID": rnd(10, string.ascii_uppercase + string.digits), "APNS_TEAM_ID": rnd(10, string.ascii_uppercase + string.digits)},
            "/analytics": {"MIXPANEL_TOKEN": hexs(32), "AMPLITUDE_API_KEY": hexs(32)}}

def auth(env):
    return {"/": {"DATABASE_URL": pg(env, "auth"), "ISSUER_URL": f"https://auth.{H[env]}.example.com", "COOKIE_SECRET": rnd(48), "ARGON2_PEPPER": rnd(32)},
            "/signing": {"JWT_PRIVATE_KEY_ID": f"key-{rnd(8, string.ascii_lowercase + string.digits)}", "JWT_PRIVATE_KEY": "-----BEGIN PRIVATE KEY-----\n" + "\n".join(rnd(64, string.ascii_letters + string.digits + "+/") for _ in range(4)) + "\n-----END PRIVATE KEY-----"},
            "/saml": {"SAML_IDP_CERT": rnd(80), "SAML_SP_KEY": rnd(80)}}

def internal(env):
    return {"/": {"RETOOL_API_KEY": f"retool_{rnd(40)}", "SLACK_BOT_TOKEN": f"xoxb-{random.randint(10**10,10**11)}-{random.randint(10**12,10**13)}-{rnd(24)}", "SLACK_SIGNING_SECRET": hexs(32),
                  "PAGERDUTY_TOKEN": rnd(20), "LINEAR_API_KEY": f"lin_api_{rnd(40)}"},
            "/github": {"GITHUB_APP_ID": str(random.randint(100000, 999999)), "GITHUB_APP_PRIVATE_KEY": "-----BEGIN RSA PRIVATE KEY-----\n" + rnd(64) + "\n-----END RSA PRIVATE KEY-----", "GITHUB_WEBHOOK_SECRET": hexs(40)}}

PROJECTS = [("payments-api", payments), ("web-app", webapp), ("data-pipeline", pipeline), ("mobile-backend", mobile), ("auth-service", auth), ("internal-tools", internal)]
COMMENTS = {"DATABASE_URL": "Primary connection string", "JWT_SIGNING_KEY": "Rotate every 90 days", "STRIPE_WEBHOOK_SECRET": "From the Stripe dashboard → Webhooks",
            "SMTP_PASSWORD": "SendGrid key, mail-send scope only", "AWS_SECRET_ACCESS_KEY": "IAM user etl-writer", "SESSION_SECRET": "Min 32 chars", "KAFKA_SASL_PASSWORD": "Managed by platform team"}
writers = [("admin", admin)] + [(e, tok[e]) for e in tok if e != "admin" and e.startswith(("priya", "arjun", "daniel"))]

existing = {p["name"]: p for p in call("GET", f"/orgs/{org}/projects", token=admin)}
total = 0
for pname, gen in PROJECTS:
    if pname in existing:
        print("skip", pname); continue
    p = call("POST", f"/orgs/{org}/projects", {"name": pname}, admin)
    pid = p["id"]
    if pname == "web-app":
        call("POST", f"/projects/{pid}/environments", {"name": "QA"}, admin)
    for env in ("dev", "staging", "prod"):
        for path, kv in gen(env).items():
            for k, v in kv.items():
                _, t = random.choice(writers)
                body = {"env": env, "path": path, "key": k, "value": v, "comment": COMMENTS.get(k, "")}
                s = call("PUT", f"/projects/{pid}/secrets", body, t); total += 1
                # some secrets get a history
                if random.random() < 0.28:
                    for _ in range(random.randint(1, 3)):
                        body["value"] = v + rnd(2) if "://" not in v else v.replace("sslmode=require", "sslmode=verify-full")
                        _, t2 = random.choice(writers)
                        call("PUT", f"/projects/{pid}/secrets", body, t2); total += 1
    # rotate a handful in prod
    lst = call("GET", f"/projects/{pid}/secrets?env=prod&path=/", token=admin)["secrets"]
    for s in random.sample(lst, min(2, len(lst))):
        call("POST", f"/projects/{pid}/secrets/{s['id']}/rotate", {}, admin)
    # tokens + machine reads
    specs = [("github-actions", "prod", "/", False, 90), ("k8s-staging", "staging", "/", False, 0), ("local-dev", "dev", "/", True, 30)]
    for n, env, path, w, days in specs:
        t = call("POST", f"/projects/{pid}/tokens", {"name": n, "env": env, "path": path, "can_write": w, "expire_days": days}, admin)["token"]
        for _ in range(random.randint(2, 6)):
            call("GET", "/v1/secrets?recursive=true&format=json", token=t)
    # reveals / exports for audit texture
    lst = call("GET", f"/projects/{pid}/secrets?env=staging&path=/", token=admin)["secrets"]
    for s in random.sample(lst, min(3, len(lst))):
        call("GET", f"/projects/{pid}/secrets/{s['id']}", token=random.choice(writers)[1])
    call("GET", f"/projects/{pid}/secrets/export?env=dev&path=/&recursive=true&format=dotenv", token=admin, raw=True)
    print("seeded", pname)
print("secret writes:", total)
