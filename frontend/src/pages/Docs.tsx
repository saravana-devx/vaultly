import type { ReactNode } from 'react'
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../auth'
import { Icon, Logo, ThemeToggle, copy } from '../components/ui'

const GITHUB = 'https://github.com/saravana-devx/vaultly'
const SECTIONS = [
  ['quickstart', 'Quickstart'],
  ['concepts', 'Concepts'],
  ['cli', 'CLI'],
  ['api', 'REST API'],
  ['self-host', 'Self-hosting'],
  ['security', 'Security'],
]

// A command or snippet: plain text beside an accent rule, with a copy button.
function Cmd({ children }: { children: ReactNode }) {
  const text = String(children).trim()
  return (
    <div className="cmd">
      <pre>{text}</pre>
      <button className="iconbtn" onClick={() => copy(text, 'Copied')} aria-label="Copy" title="Copy"><Icon name="copy" size={15} /></button>
    </div>
  )
}

function Section({ id, title, lead, children }: { id: string; title: string; lead?: string; children: ReactNode }) {
  return (
    <section id={id} className="doc-sec">
      <h2>{title}</h2>
      {lead && <p className="lead">{lead}</p>}
      {children}
    </section>
  )
}

export default function Docs() {
  const { user } = useAuth()
  const [active, setActive] = useState('quickstart')

  useEffect(() => {
    const seen = new Map()
    const io = new IntersectionObserver((entries) => {
      entries.forEach((e) => seen.set(e.target.id, e.isIntersecting))
      const first = SECTIONS.find(([id]) => seen.get(id))
      if (first) setActive(first[0])
    }, { rootMargin: '-80px 0px -60% 0px' })
    SECTIONS.forEach(([id]) => { const el = document.getElementById(id); if (el) io.observe(el) })
    return () => io.disconnect()
  }, [])

  return (
    <div className="lp docs">
      <header className="lp-nav scrolled">
        <div className="lp-wrap">
          <Link to="/"><Logo /></Link>
          <nav className="lp-links"><Link to="/">Home</Link><a href="#quickstart">Docs</a><a href={GITHUB} target="_blank" rel="noreferrer">GitHub</a></nav>
          <div className="right">
            <ThemeToggle />
            {user ? <Link className="btn primary sm" to="/app">Open dashboard</Link> : <>
              <Link className="btn ghost sm" to="/login">Sign in</Link><Link className="btn primary sm" to="/register">Get started</Link></>}
          </div>
        </div>
      </header>

      <div className="lp-wrap docs-grid">
        <aside className="docs-nav" aria-label="Documentation sections">
          <span className="docs-label">Documentation</span>
          {SECTIONS.map(([id, label]) => <a key={id} href={`#${id}`} className={active === id ? 'on' : ''}>{label}</a>)}
          <a className="docs-ext" href={`${GITHUB}/tree/main/docs`} target="_blank" rel="noreferrer">Full docs on GitHub <Icon name="right" size={13} /></a>
        </aside>

        <main className="docs-main">
          <header className="docs-head">
            <h1>Vaultly documentation</h1>
            <p>Everything you need to store secrets, hand them to your apps and run it yourself. Short on purpose: the repository has the deep dives.</p>
          </header>

          <Section id="quickstart" title="Quickstart" lead="From zero to an app running with secrets in about five minutes.">
            <ol className="steps-doc">
              <li><b>Create a workspace.</b> <Link to="/register">Sign up</Link> (or run it yourself, below). You get a workspace and become its admin.</li>
              <li><b>Create a project.</b> Projects hold secrets. Each one gets its own encryption key and three environments: development, staging and production.</li>
              <li><b>Add secrets.</b> Add them one by one, or choose <em>Import</em> and paste a <code>.env</code> file.</li>
              <li><b>Create a service token.</b> In the project, open <em>Service tokens</em> and pick an environment and a folder. Copy the token now, it is shown once.</li>
              <li><b>Use it from your app or CI.</b></li>
            </ol>
            <Cmd>{`export VAULTLY_URL=https://your-vaultly-host
export VAULTLY_TOKEN=vst_xxxxxxxxxxxx

vaultly pull > .env              # write a .env file
vaultly run -- node server.js    # or inject the secrets into one process`}</Cmd>
          </Section>

          <Section id="concepts" title="Concepts">
            <dl className="defs">
              <dt>Workspace</dt><dd>Your team. Members have a role: <b>admin</b> (everything), <b>developer</b> (read and write secrets) or <b>viewer</b> (read). An admin can override a member's role per environment, for example developer in dev and no access to production.</dd>
              <dt>Project and environments</dt><dd>A project is one application or service. Environments (development, staging, production, or your own) keep the same keys with different values.</dd>
              <dt>Folders</dt><dd>Secrets live in folders such as <code>/stripe</code> or <code>/database</code>, so a service only receives what it needs.</dd>
              <dt>Versions</dt><dd>Every change creates a new version. Restoring an old version creates another new one, so history is never rewritten. Secrets can also be rotated to a fresh random value and given an expiry date.</dd>
              <dt>References</dt><dd>One secret can point at another: <code>{'${DB_HOST}'}</code>, <code>{'${/database/PASSWORD}'}</code> or <code>{'${prod:/database/PASSWORD}'}</code> for another environment. They are resolved when secrets are exported or fetched.</dd>
              <dt>Tokens</dt><dd><b>Service tokens</b> (<code>vst_…</code>) are for machines: limited to one environment and folder, read-only unless you allow writes, with an optional expiry and IP allowlist. <b>Personal tokens</b> (<code>vpt_…</code>) act as you, for scripts, and are created in Settings.</dd>
              <dt>Audit log</dt><dd>Logins, reads, writes, exports and token use are recorded. Admins can browse them by project.</dd>
            </dl>
          </Section>

          <Section id="cli" title="CLI" lead="One small binary that talks to the machine API with a service token.">
            <Cmd>{`cd backend && go build -o vaultly ./cmd/vaultly
# or, with the Docker setup:  docker compose exec backend vaultly pull`}</Cmd>
            <table className="doc-table">
              <thead><tr><th>Command</th><th>What it does</th></tr></thead>
              <tbody>
                <tr><td><code>vaultly pull [--path /p] [--format dotenv|shell|json]</code></td><td>Print the secrets of the token's environment (folders included).</td></tr>
                <tr><td><code>vaultly run [--path /p] -- cmd</code></td><td>Start <code>cmd</code> with the secrets as environment variables.</td></tr>
                <tr><td><code>vaultly set KEY VALUE [--path /p]</code></td><td>Write one secret. The token must allow writes.</td></tr>
              </tbody>
            </table>
            <p>Settings come from <code>VAULTLY_URL</code> and <code>VAULTLY_TOKEN</code>. The folder defaults to the one the token is scoped to, and a token can never read outside it.</p>
          </Section>

          <Section id="api" title="REST API" lead="All endpoints are under /api and return JSON in one shape.">
            <Cmd>{`{ "success": true,  "message": "Secrets exported", "data": { ... } }
{ "success": false, "message": "Secret not found" }`}</Cmd>
            <p>Plain-text outputs (<code>format=dotenv</code> or <code>shell</code>) and file downloads are the exceptions.</p>
            <h3>Machine endpoints (service token)</h3>
            <Cmd>{`# read
curl -H "Authorization: Bearer $VAULTLY_TOKEN" \\
  "$VAULTLY_URL/api/v1/secrets?recursive=true&format=json"

# write (token must allow writes)
curl -X PUT -H "Authorization: Bearer $VAULTLY_TOKEN" -H "Content-Type: application/json" \\
  -d '{"path":"/","key":"API_KEY","value":"s3cret","comment":"payments"}' \\
  "$VAULTLY_URL/api/v1/secrets"

# short-lived database credentials (dynamic secret)
curl -X POST -H "Authorization: Bearer $VAULTLY_TOKEN" -H "Content-Type: application/json" \\
  -d '{"ttl_seconds":300}' "$VAULTLY_URL/api/v1/dynamic/readonly-db/lease"`}</Cmd>
            <p>Query options for reading: <code>path</code>, <code>recursive</code>, <code>resolve</code> (expand references, default true) and <code>format</code> (<code>json</code>, <code>dotenv</code>, <code>shell</code>).</p>
            <h3>Everything else</h3>
            <p>The web app uses the same API with a session. For scripts, send a personal token as <code>Authorization: Bearer vpt_…</code>. Rate limits apply (600 requests a minute per token or user by default); when you hit one you get <code>429</code> and a <code>Retry-After</code> header. The full endpoint list is in the <a href={`${GITHUB}/blob/main/docs/06_API_REFERENCE.txt`} target="_blank" rel="noreferrer">API reference</a>.</p>
          </Section>

          <Section id="self-host" title="Self-hosting" lead="The hosted workspace and your own server run the same code.">
            <Cmd>{`git clone ${GITHUB}.git && cd vaultly
cp .env.example .env      # set JWT_SECRET (openssl rand -hex 32)
make up`}</Cmd>
            <p>This starts PostgreSQL, Redis, Kafka, OpenBao (the key service), a rate-limiter service, the API and the web app. Open <code>http://localhost:3000</code>. Invitation emails are caught at <code>http://localhost:8025</code>. <code>make down</code> stops everything and keeps your data; <code>make help</code> lists every command.</p>
            <h3>On a server</h3>
            <p>Use <code>docker-compose.prod.yml</code>: Caddy gets an HTTPS certificate automatically and only ports 80 and 443 are public. Settings are in <code>.env</code> (see <code>deploy/env.prod.example</code>). Follow the <a href={`${GITHUB}/blob/main/docs/14_GO_LIVE_RUNBOOK.txt`} target="_blank" rel="noreferrer">go-live runbook</a>, and run <code>make backup</code> regularly: it saves the database and the OpenBao state, which together are the only way to restore. Workspace admins can also download an encrypted backup from <em>Settings</em>.</p>
          </Section>

          <Section id="security" title="Security" lead="What is protected, and what is not.">
            <ul className="doc-list">
              <li><b>Secret values</b> are encrypted with AES-256-GCM using a separate key per project. Those keys are wrapped by OpenBao, a separate key service whose root key never leaves it. A copy of the database alone shows only ciphertext.</li>
              <li><b>People and sensitive text</b> are encrypted too: emails and names, workspace names, secret key names and comments, token prefixes, IP allowlists, webhook URLs and the audit actor and IP address.</li>
              <li><b>Left readable on purpose:</b> project, environment and folder names, tags and token names, so they can be browsed and sorted. Anyone with a database dump could see how a team is organised, not what it stores.</li>
              <li><b>Passwords</b> use bcrypt. <b>Tokens</b> are stored as hashes and shown once. Sessions use a 15-minute access token and a rotating refresh cookie.</li>
              <li><b>Limits:</b> the running server holds the keys, so a compromised server can read what it serves. The hosted workspace is a free demo, so please do not store real production secrets in it.</li>
            </ul>
            <h3>Who encrypts what</h3>
            <p>Vaultly does its own encryption: the backend encrypts every secret value with AES-256-GCM, using a key that belongs to one project. OpenBao never sees a secret value. It only generates those project keys and holds the root key that wraps them. This is envelope encryption, the same pattern hosted secret managers use with a cloud key service. Here the key service is self-hosted.</p>
            <p>OpenBao is asked only to unwrap a project key, never for each secret, and unwrapped keys are cached for 60 seconds. Reads do not wait on it, and a short outage does not stop the app.</p>
            <h3>Without OpenBao</h3>
            <p>For a simple self-hosted install you can skip it: the root key then comes from <code>MASTER_KEY</code> in the environment. This is simpler but protects the root key less well, so use OpenBao for production.</p>
            <Cmd>{`KEY_PROVIDER=local
MASTER_KEY=$(openssl rand -base64 32)`}</Cmd>
            <p>Either way, back up the root key (the OpenBao volumes, or <code>MASTER_KEY</code>). If it is lost, every secret becomes unreadable.</p>
            <p>The reasoning and the exact list of fields are in the <a href={`${GITHUB}/blob/main/docs/13_OPENBAO_AND_PII_ENCRYPTION.txt`} target="_blank" rel="noreferrer">encryption notes</a>.</p>
          </Section>

          <footer className="docs-foot">
            <span>© {new Date().getFullYear()} Vaultly</span>
            <span><Link to="/">Home</Link> · <a href={GITHUB} target="_blank" rel="noreferrer">GitHub</a></span>
          </footer>
        </main>
      </div>
    </div>
  )
}
