import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Logo, ThemeToggle } from "../components/ui";

function Preview() {
  const rows: [string, string, boolean, string, string][] = [
    [
      "DATABASE_URL",
      "postgres://app:s3cr3t@10.0.4.12/app",
      true,
      "v4",
      "2 hr ago",
    ],
    ["STRIPE_SECRET_KEY", "", false, "v2", "3 d ago"],
    ["SENDGRID_API_KEY", "", false, "v1", "3 d ago"],
    ["JWT_SIGNING_KEY", "", false, "v7", "12 d ago"],
    ["REDIS_URL", "", false, "v1", "12 d ago"],
  ];
  return (
    <div className="pv" aria-hidden="true">
      <div className="pv-side">
        <div className="ws">
          <span className="sq">A</span>Acme
        </div>
        <a>Overview</a>
        <a className="on">Projects</a>
        <a>Members</a>
        <a>Audit log</a>
      </div>
      <div className="pv-main">
        <div className="pv-top">
          <b>payments-api</b>
          <span className="btn primary sm">Add secret</span>
        </div>
        <div className="pv-tabs">
          <span>Development</span>
          <span>Staging</span>
          <span className="on">Production</span>
        </div>
        <table>
          <thead>
            <tr>
              <th>Key</th>
              <th>Value</th>
              <th>Version</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(([k, v, show, ver, t]) => (
              <tr key={k}>
                <td>{k}</td>
                <td>
                  {show ? (
                    <span className="rev">{v}</span>
                  ) : (
                    <span className="dots">••••••••••••••</span>
                  )}
                </td>
                <td className="muted">{ver}</td>
                <td className="muted">{t}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default function Landing({ signedIn }: { signedIn?: boolean }) {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const on = () => setScrolled(window.scrollY > 8);
    on();
    window.addEventListener("scroll", on, { passive: true });
    return () => window.removeEventListener("scroll", on);
  }, []);
  const start = signedIn ? "/dashboard" : "/register";
  return (
    <div className="lp">
      <header className={`lp-nav ${scrolled ? "scrolled" : ""}`}>
        <div className="lp-wrap">
          <Link to="/">
            <Logo />
          </Link>
          <nav className="lp-links">
            <a href="#product">Product</a>
            <a href="#security">Security</a>
            <a href="#cli">CLI &amp; API</a>
            <a href="#hosting">Cloud &amp; self-host</a>
            <a href="#faq">FAQ</a>
            <Link to="/docs">Docs</Link>
          </nav>
          <div className="right">
            <ThemeToggle />
            {signedIn ? (
              <Link className="btn primary sm" to="/dashboard">
                Open dashboard
              </Link>
            ) : (
              <>
                <Link className="btn ghost sm" to="/login">
                  Sign in
                </Link>
                <Link className="btn primary sm" to="/register">
                  Get started
                </Link>
              </>
            )}
          </div>
        </div>
      </header>

      <section className="lp-hero">
        <div className="lp-wrap">
          <div className="pill">
            <b>Free</b> Hosted workspace, or run it yourself
          </div>
          <h1>
            Stop leaking secrets.{" "}
            <span className="grad">Start shipping securely.</span>
          </h1>
          <p className="lede">
            Vaultly is the home for your API keys, database passwords and
            environment variables. Encrypted, versioned, and audited across
            every environment. Use the hosted workspace, or run the same code on
            your own server.
          </p>
          <div className="lp-cta">
            <Link className="btn primary lg" to={start}>
              {signedIn ? "Open dashboard" : "Create free workspace"}
            </Link>
            <a className="btn lg" href="#hosting">
              Self-host it
            </a>
          </div>
          <div className="run" aria-label="Example commands">
            <pre>
              <span className="t-dim">$ </span>
              <span className="t-g">vaultly</span>
              {` pull --format dotenv
`}
              <span className="t-dim">{`# fetched 4 secrets from production /`}</span>
              {`
DATABASE_URL=`}
              <span className="t-y">postgres://app:••••••••@db:5432/app</span>
              {`
STRIPE_KEY=`}
              <span className="t-y">sk_live_••••••••••••</span>
              {`
JWT_SECRET=`}
              <span className="t-y">••••••••••••••••••••</span>
              {`

`}
              <span className="t-dim">$ </span>
              <span className="t-g">vaultly</span>
              {` run -- node server.js
`}
              <span className="t-b">▸</span>
              {` injected 4 secrets · listening on :3000`}
            </pre>
          </div>
        </div>
      </section>

      <section className="lp-stage">
        <div className="lp-wrap">
          <Preview />
        </div>
      </section>

      <section className="lp-strip">
        <div className="lp-wrap">
          <p>Any language, any script, through the CLI or REST API.</p>
          <ul>
            <li>Node.js</li>
            <li>Go</li>
            <li>Python</li>
            <li>Docker</li>
            <li>Bash</li>
          </ul>
        </div>
      </section>

      <div className="lp-wrap" id="product">
        <div className="row">
          <div className="copy">
            <span className="tag">01 — ORGANIZE</span>
            <h2>Environments and folders that match how you ship.</h2>
            <p>
              Every project starts with development, staging and production. Add
              your own, then group secrets into paths like{" "}
              <span className="mono">/stripe</span> or{" "}
              <span className="mono">/database</span> so each service only sees
              what it needs.
            </p>
            <ul>
              <li>Paste in an existing .env file to import it</li>
              <li>Export any environment back to .env in one click</li>
              <li>
                Comments on each secret so nobody has to ask what it’s for
              </li>
            </ul>
          </div>
          <div className="vis">
            <div className="vis-h">
              <span>payments-api</span>
              <span>3 environments</span>
            </div>
            <div className="tree">
              <div>
                <span>development</span>
                <span className="n">12 secrets</span>
              </div>
              <div>
                <span>staging</span>
                <span className="n">12 secrets</span>
              </div>
              <div>
                <span>production</span>
                <span className="n">14 secrets</span>
              </div>
              <div className="d1">
                <span>/stripe</span>
                <span className="n">3</span>
              </div>
              <div className="d1">
                <span>/database</span>
                <span className="n">4</span>
              </div>
              <div className="d2">
                <span>/database/replica</span>
                <span className="n">2</span>
              </div>
              <div className="d1">
                <span>/smtp</span>
                <span className="n">2</span>
              </div>
            </div>
          </div>
        </div>

        <div className="row flip">
          <div className="copy">
            <span className="tag">02 — HISTORY</span>
            <h2>Every change is a version you can go back to.</h2>
            <p>
              Someone pasted the wrong key into production? Open the history,
              see who changed it and when, and restore the previous value.
              Restoring creates a new version, so the record is never rewritten.
            </p>
            <ul>
              <li>Rotate any secret to a freshly generated random value</li>
              <li>
                Values are only decrypted when someone chooses to reveal them
              </li>
            </ul>
          </div>
          <div className="vis">
            <div className="vis-h">
              <span>STRIPE_SECRET_KEY</span>
              <span>4 versions</span>
            </div>
            <div className="tl">
              <div>
                <b>v4</b>
                <span>
                  Rotated<small>maya@acme.io</small>
                </span>
                <small>2 hr ago</small>
              </div>
              <div>
                <b>v3</b>
                <span>
                  Updated<small>dev@acme.io</small>
                </span>
                <span className="lnk">Restore</span>
              </div>
              <div>
                <b>v2</b>
                <span>
                  Updated<small>maya@acme.io</small>
                </span>
                <span className="lnk">Restore</span>
              </div>
              <div>
                <b>v1</b>
                <span>
                  Created<small>maya@acme.io</small>
                </span>
                <span className="lnk">Restore</span>
              </div>
            </div>
          </div>
        </div>

        <div className="row" id="cli">
          <div className="copy">
            <span className="tag">03 — ACCESS</span>
            <h2>One token for your pipeline. Nothing more than it needs.</h2>
            <p>
              Service tokens are limited to a single environment and path,
              read-only unless you say otherwise, and can expire. Use the CLI to
              inject secrets into a process, or call the API at runtime.
            </p>
            <ul>
              <li>Admin, developer and viewer roles for people</li>
              <li>Tokens are stored hashed and shown once</li>
              <li>Revoke a token and it stops working immediately</li>
            </ul>
          </div>
          <div className="code">
            <span className="c"># inject secrets into any process</span>
            {"\n"}
            <span className="g">$</span> vaultly run --path /stripe -- node
            server.js{"\n\n"}
            <span className="c"># or write a .env file</span>
            {"\n"}
            <span className="g">$</span> vaultly pull {">"} .env{"\n\n"}
            <span className="c"># or call the API</span>
            {"\n"}
            <span className="g">$</span> curl -H{" "}
            <span className="y">"Authorization: Bearer $VAULTLY_TOKEN"</span> \
            {"\n"} https://vault.acme.io/api/v1/secrets
          </div>
        </div>

        <div className="row flip">
          <div className="copy">
            <span className="tag">04 — AUDIT</span>
            <h2>A record of who touched what, and when.</h2>
            <p>
              Logins, reads, writes, exports and token use all produce an event.
              Events are published to Kafka and written by a separate consumer,
              so auditing never slows a request down.
            </p>
            <ul>
              <li>Filter by project, browse by time</li>
              <li>
                If Kafka is unreachable, events fall back to the database
                instead of being dropped
              </li>
            </ul>
          </div>
          <div className="vis log">
            <div className="vis-h">
              <span>audit log</span>
              <span>live</span>
            </div>
            <div>
              <time>14:02:11</time>
              <span>
                maya@acme.io <b>secret.update</b> /stripe/KEY
              </span>
              <em>production</em>
            </div>
            <div>
              <time>14:01:48</time>
              <span>
                token:github-actions <b>secret.fetch</b> /
              </span>
              <em>staging</em>
            </div>
            <div>
              <time>13:58:20</time>
              <span>
                dev@acme.io <b>secret.read</b> /database/URL
              </span>
              <em>development</em>
            </div>
            <div>
              <time>13:40:02</time>
              <span>
                maya@acme.io <b>token.create</b> deploy-bot
              </span>
              <em>production</em>
            </div>
            <div>
              <time>13:12:37</time>
              <span>
                dev@acme.io <b>auth.login</b>
              </span>
              <em>—</em>
            </div>
          </div>
        </div>
      </div>

      <section className="host" id="hosting">
        <div className="lp-wrap">
          <span className="tag">CLOUD OR SELF-HOSTED</span>
          <h2>Same product. You choose where it runs.</h2>
          <p className="sub">
            Start in a minute on the hosted workspace, or keep everything on
            your own infrastructure. The code is the same, so you can move
            between them.
          </p>
          <div className="host-grid">
            <div className="host-col">
              <h3>Hosted workspace</h3>
              <p>
                Sign up and you get your own workspace with projects,
                environments, tokens and an audit log. Nothing to install.
              </p>
              <ul>
                <li>Free demo, no card needed</li>
                <li>Your own isolated workspace and encryption keys</li>
                <li>Same CLI and API as self-hosted</li>
              </ul>
              <Link className="btn primary" to={start}>
                {signedIn ? "Open dashboard" : "Create free workspace"}
              </Link>
            </div>
            <div className="host-col">
              <h3>Self-hosted</h3>
              <p>
                Run the whole stack with Docker. Your data, your keys and your
                network, with HTTPS from Caddy in production.
              </p>
              <pre>{`git clone https://github.com/saravana-devx/vaultly.git
cd vaultly
make up`}</pre>
              <ul>
                <li>PostgreSQL, Redis, Kafka and OpenBao included</li>
                <li>Encrypted, passphrase-protected backups</li>
              </ul>
              <a
                className="btn"
                href="https://github.com/saravana-devx/vaultly"
                target="_blank"
                rel="noreferrer"
              >
                View on GitHub
              </a>
            </div>
          </div>
        </div>
      </section>

      <section className="sec" id="security">
        <div className="lp-wrap">
          <h2>If someone gets your database, they get ciphertext.</h2>
          <p className="sub">
            Secrets use envelope encryption. The key that unlocks everything
            never sits next to the data it protects.
          </p>
          <div className="sec-grid">
            <div className="keys">
              <div className="k">
                <span className="n">01</span>
                <b>Root key</b>
                <span>
                  Held by OpenBao, a separate key service, and never written to
                  the database. It only wraps and unwraps project keys.
                </span>
              </div>
              <div className="k">
                <span className="n">02</span>
                <b>Project key</b>
                <span>
                  A random 256-bit key per project, stored only in wrapped
                  (encrypted) form.
                </span>
              </div>
              <div className="k">
                <span className="n">03</span>
                <b>Secret value</b>
                <span>
                  Sealed with AES-256-GCM and bound to its project, so
                  ciphertext can’t be moved between projects.
                </span>
              </div>
            </div>
            <div className="facts">
              <div>
                <b>Passwords</b>
                <span>bcrypt, cost 12</span>
              </div>
              <div>
                <b>Sessions</b>
                <span>
                  15-minute access token, rotating refresh token in an httpOnly
                  cookie
                </span>
              </div>
              <div>
                <b>Service tokens</b>
                <span>SHA-256 hashed at rest, shown once</span>
              </div>
              <div>
                <b>Sign-in</b>
                <span>Rate limited per IP</span>
              </div>
              <div>
                <b>Personal data</b>
                <span>Emails and names are encrypted too</span>
              </div>
              <div>
                <b>Reads</b>
                <span>Every reveal and export is audited</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <div className="lp-wrap">
        <section className="faq" id="faq">
          <h2>Questions, answered.</h2>
          <div>
            <details>
              <summary>Where does the root key live?</summary>
              <p>
                In OpenBao, a separate key service. Vaultly asks it to wrap and
                unwrap each project’s key, and the root key never leaves it. For
                simple setups a local key from the environment is also
                supported.
              </p>
            </details>
            <details>
              <summary>Can I use it for real production secrets?</summary>
              <p>
                The hosted workspace is a free demo, so please don’t put real
                production secrets in it yet. If you need that, self-host it:
                you keep the data, the keys and the server.
              </p>
            </details>
            <details>
              <summary>How do I self-host it?</summary>
              <p>
                Clone the repository and run{" "}
                <span className="mono">make up</span> (or{" "}
                <span className="mono">docker compose up -d</span>). The runbook
                in the repository covers a single-server setup with automatic
                HTTPS and backups.
              </p>
            </details>
            <details>
              <summary>Why is Kafka involved?</summary>
              <p>
                Audit events are published to a topic and persisted by a
                consumer group, so they stay ordered per workspace and writing
                them never sits in the request path. If the broker is down,
                events are written straight to Postgres.
              </p>
            </details>
            <details>
              <summary>Can I bring my existing .env files?</summary>
              <p>
                Yes. Paste the contents into the import dialog and pick an
                environment and path. Quoted values, comments and export
                prefixes are handled.
              </p>
            </details>
            <details>
              <summary>Does it support SSO?</summary>
              <p>
                Not yet. Sign-in is email and password, with role-based access
                inside each workspace.
              </p>
            </details>
          </div>
        </section>
      </div>

      <section className="lp-final">
        <div className="lp-wrap">
          <div>
            <h2>Get the secrets out of Slack.</h2>
            <p>
              Start in a minute on the hosted workspace, or run the same product
              on your own server with one command.
            </p>
          </div>
          <div className="lp-final-act">
            <Link className="btn primary lg" to={start}>
              {signedIn ? "Open dashboard" : "Create free workspace"}
            </Link>
            <a className="btn lg" href="#hosting">
              Self-host it
            </a>
          </div>
        </div>
      </section>

      <footer className="lp-foot">
        <div className="lp-wrap lp-foot-top">
          <div className="lp-foot-brand">
            <Logo />
            <p>
              Secrets management for teams that ship. Hosted or self-hosted.
            </p>
            <span className="lp-foot-status">
              <i /> Encrypted at rest · every access audited
            </span>
          </div>
          <nav aria-label="Product">
            <h4>Product</h4>
            <a href="#product">How it works</a>
            <a href="#security">Security</a>
            <a href="#cli">CLI &amp; API</a>
            <a href="#hosting">Cloud &amp; self-host</a>
          </nav>
          <nav aria-label="Developers">
            <h4>Developers</h4>
            <a
              href="https://github.com/saravana-devx/vaultly"
              target="_blank"
              rel="noreferrer"
            >
              GitHub
            </a>
            <Link to="/docs">Documentation</Link>
            <Link to="/docs/api">API reference</Link>
            <a href="#faq">FAQ</a>
          </nav>
          <nav aria-label="Account">
            <h4>Account</h4>
            <Link to="/login">Sign in</Link>
            <Link to="/register">Create workspace</Link>
          </nav>
        </div>
        <div className="lp-wrap lp-foot-bottom">
          <span>© {new Date().getFullYear()} Vaultly</span>
          <span>Built with Go · React · PostgreSQL · Kafka · OpenBao</span>
        </div>
        <div className="lp-mark" aria-hidden="true">
          <span>Vaultly</span>
        </div>
      </footer>
    </div>
  );
}
