import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Logo } from '../components/ui.jsx'

function Preview() {
  const rows = [['DATABASE_URL', 'postgres://app:s3cr3t@10.0.4.12/app', true, 'v4', '2 hr ago'], ['STRIPE_SECRET_KEY', '', false, 'v2', '3 d ago'], ['SENDGRID_API_KEY', '', false, 'v1', '3 d ago'], ['JWT_SIGNING_KEY', '', false, 'v7', '12 d ago'], ['REDIS_URL', '', false, 'v1', '12 d ago']]
  return (
    <div className="pv" aria-hidden="true">
      <div className="pv-side">
        <div className="ws"><span className="sq">A</span>Acme</div>
        <a>Overview</a><a className="on">Projects</a><a>Members</a><a>Audit log</a>
      </div>
      <div className="pv-main">
        <div className="pv-top"><b>payments-api</b><span className="btn primary sm">Add secret</span></div>
        <div className="pv-tabs"><span>Development</span><span>Staging</span><span className="on">Production</span></div>
        <table>
          <thead><tr><th>Key</th><th>Value</th><th>Version</th><th>Updated</th></tr></thead>
          <tbody>{rows.map(([k, v, show, ver, t]) => (
            <tr key={k}><td>{k}</td><td>{show ? <span className="rev">{v}</span> : <span className="dots">••••••••••••••</span>}</td><td className="muted">{ver}</td><td className="muted">{t}</td></tr>))}</tbody>
        </table>
      </div>
    </div>
  )
}

export default function Landing({ signedIn }) {
  const [scrolled, setScrolled] = useState(false)
  useEffect(() => {
    const on = () => setScrolled(window.scrollY > 8)
    on(); window.addEventListener('scroll', on, { passive: true })
    return () => window.removeEventListener('scroll', on)
  }, [])
  const start = signedIn ? '/app' : '/register'
  return (
    <div className="lp">
      <header className={`lp-nav ${scrolled ? 'scrolled' : ''}`}>
        <div className="lp-wrap">
          <Link to="/"><Logo /></Link>
          <nav className="lp-links"><a href="#product">Product</a><a href="#security">Security</a><a href="#cli">CLI &amp; API</a><a href="#faq">FAQ</a></nav>
          <div className="right">
            {signedIn ? <Link className="btn primary sm" to="/app">Open dashboard</Link> : <>
              <Link className="btn ghost sm" to="/login">Sign in</Link><Link className="btn primary sm" to="/register">Get started</Link></>}
          </div>
        </div>
      </header>

      <section className="lp-hero">
        <div className="lp-wrap">
          <div className="pill"><b>New</b> Path-scoped service tokens &amp; one-click rotation</div>
          <h1>Stop leaking secrets. <span className="grad">Start shipping securely.</span></h1>
          <p className="lede">Vaultly is the self-hosted home for your API keys, database passwords and environment variables. Encrypted, versioned, and audited across every environment.</p>
          <div className="lp-cta">
            <Link className="btn primary lg" to={start}>{signedIn ? 'Open dashboard' : 'Create free workspace'}</Link>
            <a className="btn lg" href="#product">See how it works</a>
          </div>
          <div className="term">
            <div className="term-bar"><i /><i /><i /><span>~/projects/api</span></div>
<pre><span className="t-dim">$ </span><span className="t-g">vaultly</span>{` pull --format dotenv
`}<span className="t-dim">{`# fetched 4 secrets from production /`}</span>{`
DATABASE_URL=`}<span className="t-y">postgres://app:••••••••@db:5432/app</span>{`
STRIPE_KEY=`}<span className="t-y">sk_live_••••••••••••</span>{`
JWT_SECRET=`}<span className="t-y">••••••••••••••••••••</span>{`

`}<span className="t-dim">$ </span><span className="t-g">vaultly</span>{` run -- node server.js
`}<span className="t-b">▸</span>{` injected 4 secrets · listening on :3000`}</pre>
          </div>
        </div>
      </section>

      <section className="lp-stage"><div className="lp-wrap"><Preview /></div></section>

      <section className="lp-strip">
        <div className="lp-wrap">
          <p>Works anywhere you can set an environment variable.</p>
          <ul><li>Node.js</li><li>Go</li><li>Python</li><li>Docker</li><li>Kubernetes</li><li>GitHub Actions</li><li>GitLab CI</li></ul>
        </div>
      </section>

      <div className="lp-wrap" id="product">
        <div className="row">
          <div className="copy">
            <span className="tag">01 — ORGANIZE</span>
            <h2>Environments and folders that match how you ship.</h2>
            <p>Every project starts with development, staging and production. Add your own, then group secrets into paths like <span className="mono">/stripe</span> or <span className="mono">/database</span> so each service only sees what it needs.</p>
            <ul><li>Paste in an existing .env file to import it</li><li>Export any environment back to .env in one click</li><li>Comments on each secret so nobody has to ask what it’s for</li></ul>
          </div>
          <div className="vis">
            <div className="vis-h"><span>payments-api</span><span>3 environments</span></div>
            <div className="tree">
              <div><span>development</span><span className="n">12 secrets</span></div>
              <div><span>staging</span><span className="n">12 secrets</span></div>
              <div><span>production</span><span className="n">14 secrets</span></div>
              <div className="d1"><span>/stripe</span><span className="n">3</span></div>
              <div className="d1"><span>/database</span><span className="n">4</span></div>
              <div className="d2"><span>/database/replica</span><span className="n">2</span></div>
              <div className="d1"><span>/smtp</span><span className="n">2</span></div>
            </div>
          </div>
        </div>

        <div className="row flip">
          <div className="copy">
            <span className="tag">02 — HISTORY</span>
            <h2>Every change is a version you can go back to.</h2>
            <p>Someone pasted the wrong key into production? Open the history, see who changed it and when, and restore the previous value. Restoring creates a new version, so the record is never rewritten.</p>
            <ul><li>Rotate any secret to a freshly generated random value</li><li>Values are only decrypted when someone chooses to reveal them</li></ul>
          </div>
          <div className="vis">
            <div className="vis-h"><span>STRIPE_SECRET_KEY</span><span>4 versions</span></div>
            <div className="tl">
              <div><b>v4</b><span>Rotated<small>maya@acme.io</small></span><small>2 hr ago</small></div>
              <div><b>v3</b><span>Updated<small>dev@acme.io</small></span><span className="lnk">Restore</span></div>
              <div><b>v2</b><span>Updated<small>maya@acme.io</small></span><span className="lnk">Restore</span></div>
              <div><b>v1</b><span>Created<small>maya@acme.io</small></span><span className="lnk">Restore</span></div>
            </div>
          </div>
        </div>

        <div className="row" id="cli">
          <div className="copy">
            <span className="tag">03 — ACCESS</span>
            <h2>One token for your pipeline. Nothing more than it needs.</h2>
            <p>Service tokens are limited to a single environment and path, read-only unless you say otherwise, and can expire. Use the CLI to inject secrets into a process, or call the API at runtime.</p>
            <ul><li>Admin, developer and viewer roles for people</li><li>Tokens are stored hashed and shown once</li><li>Revoke a token and it stops working immediately</li></ul>
          </div>
          <div className="code"><span className="c"># inject secrets into any process</span>{'\n'}<span className="g">$</span> vaultly run --path /stripe -- node server.js{'\n\n'}<span className="c"># or write a .env file</span>{'\n'}<span className="g">$</span> vaultly pull {'>'} .env{'\n\n'}<span className="c"># or call the API</span>{'\n'}<span className="g">$</span> curl -H <span className="y">"Authorization: Bearer $VAULTLY_TOKEN"</span> \{'\n'}    https://vault.acme.io/api/v1/secrets</div>
        </div>

        <div className="row flip">
          <div className="copy">
            <span className="tag">04 — AUDIT</span>
            <h2>A record of who touched what, and when.</h2>
            <p>Logins, reads, writes, exports and token use all produce an event. Events are published to Kafka and written by a separate consumer, so auditing never slows a request down.</p>
            <ul><li>Filter by project, browse by time</li><li>If Kafka is unreachable, events fall back to the database instead of being dropped</li></ul>
          </div>
          <div className="vis log">
            <div className="vis-h"><span>audit log</span><span>live</span></div>
            <div><time>14:02:11</time><span>maya@acme.io <b>secret.update</b> /stripe/KEY</span><em>production</em></div>
            <div><time>14:01:48</time><span>token:github-actions <b>secret.fetch</b> /</span><em>staging</em></div>
            <div><time>13:58:20</time><span>dev@acme.io <b>secret.read</b> /database/URL</span><em>development</em></div>
            <div><time>13:40:02</time><span>maya@acme.io <b>token.create</b> deploy-bot</span><em>production</em></div>
            <div><time>13:12:37</time><span>dev@acme.io <b>auth.login</b></span><em>—</em></div>
          </div>
        </div>
      </div>

      <section className="sec" id="security">
        <div className="lp-wrap">
          <h2>If someone gets your database, they get ciphertext.</h2>
          <p className="sub">Secrets use envelope encryption. The key that unlocks everything never sits next to the data it protects.</p>
          <div className="sec-grid">
            <div className="keys">
              <div className="k"><span className="n">01</span><b>Master key</b><span>Supplied through the environment and never written to the database. This is the single place to plug in a KMS.</span></div>
              <div className="k"><span className="n">02</span><b>Project key</b><span>A random 256-bit key per project, stored only in wrapped (encrypted) form.</span></div>
              <div className="k"><span className="n">03</span><b>Secret value</b><span>Sealed with AES-256-GCM and bound to its project, so ciphertext can’t be moved between projects.</span></div>
            </div>
            <div className="facts">
              <div><b>Passwords</b><span>bcrypt, cost 12</span></div>
              <div><b>Sessions</b><span>15-minute access token, rotating refresh token in an httpOnly cookie</span></div>
              <div><b>Service tokens</b><span>SHA-256 hashed at rest, shown once</span></div>
              <div><b>Sign-in</b><span>Rate limited per IP</span></div>
              <div><b>Reads</b><span>Every reveal and export is audited</span></div>
            </div>
          </div>
        </div>
      </section>

      <div className="lp-wrap">
        <section className="faq" id="faq">
          <h2>Questions, answered.</h2>
          <div>
            <details><summary>Where does the master key live?</summary><p>Today it is read from the MASTER_KEY environment variable at startup. The keyring is a small, isolated piece of code, which is where you would swap in AWS KMS or similar.</p></details>
            <details><summary>Why is Kafka involved?</summary><p>Audit events are published to a topic and persisted by a consumer group, so they stay ordered per workspace and writing them never sits in the request path. If the broker is down, events are written straight to Postgres.</p></details>
            <details><summary>Can I bring my existing .env files?</summary><p>Yes. Paste the contents into the import dialog and pick an environment and path. Quoted values, comments and export prefixes are handled.</p></details>
            <details><summary>Does it support SSO?</summary><p>Not yet. Sign-in is email and password, with role-based access inside each workspace.</p></details>
          </div>
        </section>
      </div>

      <section className="lp-final">
        <div className="lp-wrap">
          <h2>Get the secrets out of Slack.</h2>
          <Link className="btn primary lg" to={start}>{signedIn ? 'Open dashboard' : 'Create a workspace'}</Link>
        </div>
      </section>

      <footer className="lp-foot">
        <div className="lp-wrap">
          <div><Logo /><p style={{ marginTop: 12, maxWidth: '30ch' }}>Secrets management for teams that ship.</p></div>
          <div><h4>Product</h4><a href="#product">Overview</a><a href="#security">Security</a><a href="#cli">CLI &amp; API</a></div>
          <div><h4>Account</h4><Link to="/login">Sign in</Link><Link to="/register">Create workspace</Link></div>
          <div><h4>Resources</h4><a href="#faq">FAQ</a></div>
          <div className="legal">© {new Date().getFullYear()} Vaultly</div>
        </div>
      </footer>
    </div>
  )
}
