import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Cmd, Note, Table, type DocPage } from "./parts";

const D = ({ to, children }: { to: string; children: ReactNode }) => (
  <Link to={`/docs/${to}`}>{children}</Link>
);

export const PAGES: DocPage[] = [
  {
    slug: "introduction",
    title: "Introduction",
    group: "Getting started",
    summary: "What Vaultly is and what it gives you.",
    keywords: "overview about what is vaultly",
    body: () => (
      <>
        <p>
          Vaultly keeps your team's secrets in one place: API keys, database
          passwords and <code>.env</code> files. They are stored encrypted,
          handed to apps and CI through tokens that can reach only what they
          need, and every access is recorded.
        </p>
        <h2>What you can do</h2>
        <ul className="doc-list">
          <li>
            <b>Store secrets per environment.</b> A project has development,
            staging and production (or your own), with folders and a full
            version history.
          </li>
          <li>
            <b>Give each app its own access.</b> Service tokens are limited to
            one environment and folder, with an optional expiry and IP
            allowlist.
          </li>
          <li>
            <b>
              Issue a different database login to every developer or request.
            </b>{" "}
            <D to="dynamic-secrets">Dynamic secrets</D> create short-lived
            PostgreSQL users that disappear on their own.
          </li>
          <li>
            <b>Control who sees what.</b> Roles for the workspace, plus an
            override for each environment.
          </li>
          <li>
            <b>See what happened.</b> An <D to="audit-log">audit log</D> of
            sign-ins, reads, writes and token use, and{" "}
            <D to="integrations">webhooks</D> to send events elsewhere.
          </li>
          <li>
            <b>Run it yourself.</b> One command starts everything with Docker.
            See <D to="self-hosting">Self-hosting</D>.
          </li>
        </ul>
        <h2>How it fits together</h2>
        <p>
          You use the web app, the <D to="cli">CLI</D> or the{" "}
          <D to="api">API</D>. They all talk to the Vaultly backend, which
          stores only ciphertext in PostgreSQL. The keys that protect that
          ciphertext live in OpenBao, a separate key service. Redis counts
          requests for rate limits, and audit events travel through Kafka. The{" "}
          <D to="security">Security</D> page explains the encryption in detail.
        </p>
        <h2>Where to go next</h2>
        <p>
          New here? Start with the <D to="quickstart">Quickstart</D>. Setting up
          a team? Read <D to="workspaces-access">Workspaces and access</D>.
          Connecting an app? Read <D to="tokens">Tokens</D> and the{" "}
          <D to="cli">CLI</D>.
        </p>
      </>
    ),
  },
  {
    slug: "quickstart",
    title: "Quickstart",
    group: "Getting started",
    summary: "From zero to an app running with secrets in about five minutes.",
    keywords: "start begin first steps setup token pull",
    body: () => (
      <>
        <ol className="steps-doc">
          <li>
            <b>Create a workspace.</b> <Link to="/register">Sign up</Link>, or{" "}
            <D to="self-hosting">run it yourself</D>. You get a workspace and
            become its admin.
          </li>
          <li>
            <b>Create a project.</b> Each project gets its own encryption key
            and three environments: development, staging and production.
          </li>
          <li>
            <b>Add secrets.</b> Add them one by one, or choose <em>Import</em>{" "}
            and paste a <code>.env</code> file.
          </li>
          <li>
            <b>Create a service token.</b> In the project, open{" "}
            <em>Service tokens</em> and pick an environment and a folder. Copy
            the token now. It is shown once.
          </li>
          <li>
            <b>Use it from your app or CI.</b>
          </li>
        </ol>
        <Cmd>{`export VAULTLY_URL=https://your-vaultly-host
export VAULTLY_TOKEN=vst_xxxxxxxxxxxx

vaultly pull > .env              # write a .env file
vaultly run -- node server.js    # or inject the secrets into one process`}</Cmd>
        <Note kind="tip">
          On an empty workspace, the overview has a <em>Load sample data</em>{" "}
          button. It creates a sample project with a few made-up secrets so you
          can look around first.
        </Note>
        <h2>Next</h2>
        <p>
          Learn how <D to="secrets">secrets and environments</D> work, or
          restrict access with <D to="workspaces-access">roles</D>.
        </p>
      </>
    ),
  },
  {
    slug: "workspaces-access",
    title: "Workspaces and access",
    group: "Concepts",
    summary: "Roles, per-environment access and invitations.",
    keywords: "roles admin developer viewer members invite permissions team",
    body: () => (
      <>
        <p>
          A workspace is your team. Everything belongs to one workspace, and
          each member has a role in it.
        </p>
        <h2>Roles</h2>
        <Table
          head={["Role", "Can do"]}
          rows={[
            [
              <b key="a">Admin</b>,
              "Everything: create and delete projects, manage members and tokens, read the audit log, back up and restore.",
            ],
            [
              <b key="d">Developer</b>,
              "Read and write secrets in the environments they can access.",
            ],
            [<b key="v">Viewer</b>, "Read secrets only."],
          ]}
        />
        <p>
          The last admin cannot be demoted or removed, so a workspace can never
          be left without one.
        </p>
        <h2>Access per environment</h2>
        <p>
          An admin can override a member's role for a single environment, for
          example <em>developer</em> in development and <em>no access</em> to
          production. The choices are <code>inherit</code> (use the workspace
          role), <code>none</code>, <code>viewer</code> and{" "}
          <code>developer</code>. Overrides match the environment name, so one
          setting covers every project that has a <code>prod</code> environment.
        </p>
        <Note>
          A person with no access to an environment cannot see its secrets, and
          search never returns them.
        </Note>
        <h2>Inviting people</h2>
        <p>
          In <em>Members</em>, choose <em>Invite member</em>, enter an email
          address and a role, and optionally set per-environment access. Vaultly
          emails a link and also shows it to you so you can share it directly.
          The link works once, for that email address, for 7 days. You can
          revoke a pending invitation at any time.
        </p>
        <p>
          Someone who already has an account signs in and accepts. Someone new
          creates an account from the same link.
        </p>
        <h2>Two-factor sign-in</h2>
        <p>
          In <em>Settings</em>, anyone can turn on two-factor sign-in. Scan the
          QR code with an authenticator app (1Password, Google Authenticator,
          Authy…) and confirm with a code. From then on, signing in asks for a
          6-digit code after the password.
        </p>
        <ul className="doc-list">
          <li>
            <b>Recovery codes.</b> Turning it on shows 10 one-time codes. Each
            one signs you in once if you lose your phone. You can create a new
            set at any time, which cancels the old one.
          </li>
          <li>
            <b>Codes can't be reused.</b> A code that was accepted once is
            refused afterwards, even inside its 30-second window. After 5 wrong
            codes, you must enter your password again.
          </li>
          <li>
            <b>Turning it off</b> needs your password and a current code, so a
            stolen session alone can't remove it. Personal tokens can't change
            these settings.
          </li>
        </ul>
      </>
    ),
  },
  {
    slug: "secrets",
    title: "Projects and secrets",
    group: "Concepts",
    summary: "Environments, folders, versions, references and expiry.",
    keywords:
      "environments folders versions rollback rotate expiry references tags import export env file",
    body: () => (
      <>
        <h2>Projects and environments</h2>
        <p>
          A project is one application or service. It starts with development,
          staging and production, and admins can add more. Each environment
          holds the same keys with different values. Every project has its own
          encryption key.
        </p>
        <h2>Folders</h2>
        <p>
          Secrets live in folders such as <code>/stripe</code> or{" "}
          <code>/database</code>, so a service receives only what it needs. A
          token scoped to <code>/stripe</code> can never read{" "}
          <code>/database</code>.
        </p>
        <h2>Versions and rollback</h2>
        <p>
          Every change creates a new version. Restoring an old version creates
          another new one, so history is never rewritten. The <em>History</em>{" "}
          view shows who changed a secret and when, without showing old values.
        </p>
        <h2>Rotation and expiry</h2>
        <p>
          <b>Rotate</b> replaces a secret with a fresh random value. You can
          also give a secret an <b>expiry date</b>: after it, the secret is no
          longer served, and Vaultly records a <code>secret.expired</code>{" "}
          event. Expiring secrets and tokens appear under{" "}
          <em>Needs attention</em> on the overview.
        </p>
        <h2>References</h2>
        <p>One secret can point at another, so a value is stored once.</p>
        <Table
          head={["Write", "Means"]}
          rows={[
            [
              <code key="1">{"${DB_HOST}"}</code>,
              "A secret in the same folder.",
            ],
            [
              <code key="2">{"${/database/PASSWORD}"}</code>,
              "A secret in another folder.",
            ],
            [
              <code key="3">{"${prod:/database/PASSWORD}"}</code>,
              "A secret in another environment.",
            ],
          ]}
        />
        <p>
          References are resolved when secrets are fetched or exported. A
          service token can only follow references inside its own environment
          and folder. Circular references are handled safely.
        </p>
        <h2>Tags and search</h2>
        <p>
          Add tags to group secrets, and use the search box to look across
          folders and environments. Search only returns what you are allowed to
          read.
        </p>
        <h2>Import and export</h2>
        <p>
          <em>Import</em> takes the text of a <code>.env</code> file and creates
          or updates the secrets in it. <em>Export</em> downloads the current
          folder as <code>.env</code>, shell exports or JSON. Exports are
          audited and rate limited.
        </p>
        <h2>Comparing environments</h2>
        <p>
          The <em>Compare</em> tab lines up environments key by key and shows
          which keys are <b>missing</b> somewhere and which have{" "}
          <b>different values</b>, the drift that usually breaks a deploy.
          Values are compared on the server and never sent: environments with
          the same letter have the same value. It only includes environments you
          can read.
        </p>
        <h2>Protected environments and approvals</h2>
        <p>
          An admin can mark an environment as <b>protected</b> in the project's{" "}
          <em>Settings</em>. After that, when a developer saves or deletes a
          secret there, Vaultly opens a <b>change request</b> instead of
          changing it. Apps keep getting the old value until someone else
          approves.
        </p>
        <ul className="doc-list">
          <li>
            Requests are listed under <em>Changes</em>. A reviewer sees the
            current and proposed values side by side and can approve, with an
            optional note, or reject.
          </li>
          <li>
            You can't approve your own change. Approving needs write access to
            that environment. The author can cancel a request while it is open.
          </li>
          <li>
            If the secret changed after the request was opened, the request is
            marked <b>stale</b> and can't be applied, so a newer value is never
            overwritten by accident.
          </li>
          <li>
            An applied change gets a normal new version under the author's name,
            and the usual audit events and webhooks fire.
          </li>
          <li>
            Admins write directly. Rotate, restore and import are turned off for
            developers on a protected environment, since each value has to be
            reviewed.
          </li>
        </ul>
      </>
    ),
  },
  {
    slug: "tokens",
    title: "Tokens",
    group: "Concepts",
    summary: "Service tokens for machines and personal tokens for scripts.",
    keywords:
      "service token personal token vst vpt ip allowlist cidr api key ci",
    body: () => (
      <>
        <p>
          Apps, CI jobs and scripts authenticate with tokens instead of a
          password. A token is shown once when you create it, and Vaultly keeps
          only a hash of it, so a lost token cannot be recovered. Create a new
          one instead.
        </p>
        <h2>Service tokens</h2>
        <p>
          For machines. Create them inside a project (<em>Service tokens</em>).
          They start with <code>vst_</code>.
        </p>
        <ul className="doc-list">
          <li>
            <b>Scoped.</b> Limited to one environment and one folder, and
            everything below it.
          </li>
          <li>
            <b>Read-only by default.</b> Allow writes only when a job needs to
            save a secret.
          </li>
          <li>
            <b>Expiry.</b> Set a number of days, and Vaultly warns before it
            runs out.
          </li>
          <li>
            <b>IP allowlist.</b> Optionally accept requests only from given
            addresses or ranges. A request from anywhere else gets{" "}
            <code>403</code> and an audit event, <code>token.denied</code>.
          </li>
        </ul>
        <h2>Personal tokens</h2>
        <p>
          For your own scripts. Create them in <em>Settings</em>. They start
          with <code>vpt_</code>, act as you, and have your roles, including
          your per-environment access. A personal token cannot create or delete
          tokens.
        </p>
        <h2>Which one to use</h2>
        <Table
          head={["Use case", "Token"]}
          rows={[
            [
              "A deployed app, CI job or container",
              <>Service token, one per app and environment</>,
            ],
            ["A script you run yourself against the API", "Personal token"],
            [
              "Giving each developer their own database login",
              <>
                <D to="dynamic-secrets">Dynamic secrets</D>
              </>,
            ],
          ]}
        />
        <Note kind="tip">
          Create one token per app and environment. If one leaks, you revoke
          just that one, and the audit log shows exactly which token was used.
        </Note>
      </>
    ),
  },
  {
    slug: "dynamic-secrets",
    title: "Dynamic secrets",
    group: "Guides",
    summary:
      "A different, short-lived database login for each developer or request.",
    keywords:
      "dynamic lease ttl postgres database credentials temporary username password",
    body: () => (
      <>
        <p>
          Instead of sharing one database password, Vaultly can create a new
          database user on demand, give it to the caller, and delete it when its
          time is up. Every developer, job or request can have its own login,
          and nothing long-lived is left to leak.
        </p>
        <Note kind="warn">
          Only <b>PostgreSQL</b> is supported today.
        </Note>
        <h2>How it works</h2>
        <ol className="steps-doc">
          <li>
            An admin adds a dynamic secret to a project environment, with an
            admin connection URL, the <code>GRANT</code> statements to apply,
            and a default and maximum time to live.
          </li>
          <li>
            Someone asks for a <b>lease</b>. Vaultly creates a PostgreSQL user
            with a random name and password and returns a ready-to-use
            connection URL. The password is shown once.
          </li>
          <li>
            When the time runs out, or an admin revokes the lease, Vaultly drops
            the user.
          </li>
        </ol>
        <p>
          Each lease is recorded in the <D to="audit-log">audit log</D>. Admins
          can list live leases and revoke any of them from the project's{" "}
          <em>Dynamic secrets</em> tab. Deleting the project or environment
          revokes its leases first.
        </p>
        <h2>Set one up</h2>
        <p>
          In the project, open <em>Dynamic secrets</em> and add one. Use a
          database account that can create roles (<code>CREATEROLE</code>). The
          admin URL is stored encrypted.
        </p>
        <Cmd>{`Name:         readonly-db
Environment:  development
Admin URL:    postgres://admin:***@db.internal:5432/app
Grants:       GRANT pg_read_all_data TO {{name}}
Default TTL:  60 min        Max TTL: 1440 min`}</Cmd>
        <h2>Ask for credentials</h2>
        <p>
          <b>From the dashboard</b>, a developer with access to that environment
          chooses <em>Generate</em> (30 per minute per user).{" "}
          <b>From an app or CI</b>, use a service token:
        </p>
        <Cmd>{`curl -X POST -H "Authorization: Bearer $VAULTLY_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"ttl_seconds":300}' \\
  "$VAULTLY_URL/api/v1/dynamic/readonly-db/lease"

# { "lease_id": "...", "username": "v_ab12cd", "password": "...",
#   "connection_url": "postgres://v_ab12cd:...@db.internal:5432/app",
#   "expires_at": "..." }`}</Cmd>
        <p>
          The machine endpoint is limited to 60 requests a minute per token. A
          lease lasts at least 60 seconds and never longer than the maximum time
          to live you set.
        </p>
        <h2>Good to know</h2>
        <ul className="doc-list">
          <li>
            Each lease creates a real database user, so very high request rates
            put load on that database. Keep time to live short but not tiny.
          </li>
          <li>
            Passwords for static secrets, such as a third-party API key, cannot
            be created per request. Store those as normal secrets and use{" "}
            <em>Rotate</em>.
          </li>
        </ul>
      </>
    ),
  },
  {
    slug: "integrations",
    title: "Integrations and webhooks",
    group: "Guides",
    summary:
      "Sync secrets to GitHub, Vercel and Railway, and send events to your own URL.",
    keywords:
      "github actions vercel railway sync webhook signature hmac events",
    body: () => (
      <>
        <h2>Sync to other platforms</h2>
        <p>
          An integration copies the secrets of one environment and folder to
          another service, either when you press <em>Sync</em> or automatically
          whenever a secret changes (<em>auto-sync</em>).
        </p>
        <Table
          head={["Platform", "What it needs"]}
          rows={[
            [
              <b key="g">GitHub Actions</b>,
              "A personal access token with repo scope (or Actions secrets: write), the repository as owner/name, and optionally a GitHub environment.",
            ],
            [
              <b key="v">Vercel</b>,
              "An access token and the project ID or name. Optionally a team ID and the targets, such as production and preview.",
            ],
            [
              <b key="r">Railway</b>,
              "An API token, the project ID and environment ID, and optionally a service ID.",
            ],
          ]}
        />
        <Note>
          Integrations push additions and updates only. Deleting a secret in
          Vaultly does not delete it at the destination. The platform tokens you
          enter are stored encrypted.
        </Note>
        <h2>Webhooks</h2>
        <p>
          A webhook posts a signed JSON message to a URL you choose whenever
          matching events happen in a project. Choose the events with patterns:{" "}
          <code>*</code> for everything, <code>secret.*</code> for all secret
          events, or an exact name such as <code>secret.update</code>. The full
          list of names is in the <D to="audit-log">audit log</D> page.
        </p>
        <p>
          The message describes what happened. It{" "}
          <b>never contains secret values</b>.
        </p>
        <Cmd>{`POST https://your-app.example.com/hooks/vaultly
X-Vaultly-Event:     secret.update
X-Vaultly-Delivery:  7f1c...            (unique id, safe for de-duplication)
X-Vaultly-Timestamp: 1767225600
X-Vaultly-Signature: v1=3b9f...

{ "id": "7f1c...", "event": "secret.update", "created_at": "...",
  "project": "payments-api", "actor": "maya@acme.dev", "resource": "/stripe/API_KEY" }`}</Cmd>
        <h2>Verify the signature</h2>
        <p>
          When you create a webhook, Vaultly shows its signing secret once. The
          signature is an HMAC-SHA256 of the timestamp, a dot and the raw body.
          Reject messages whose signature does not match, and ones whose
          timestamp is too old.
        </p>
        <Cmd>{`import crypto from 'node:crypto'

function verify(secret, timestamp, rawBody, header) {
  const expected = 'v1=' + crypto.createHmac('sha256', secret)
    .update(timestamp + '.' + rawBody).digest('hex')
  return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(header))
}`}</Cmd>
        <h2>Delivery and limits</h2>
        <ul className="doc-list">
          <li>
            If your server fails or times out, Vaultly retries twice more, after
            about 1 and 5 seconds.
          </li>
          <li>
            The <em>Deliveries</em> view shows each attempt and its result.{" "}
            <em>Send test</em> sends a sample event.
          </li>
          <li>
            Webhooks to private or local addresses are blocked, so a webhook
            cannot be used to reach your internal network.
          </li>
        </ul>
      </>
    ),
  },
  {
    slug: "audit-log",
    title: "Audit log",
    group: "Guides",
    summary: "A record of who did what, and when.",
    keywords: "audit events history log actions",
    body: () => (
      <>
        <p>
          Vaultly records sign-ins, secret reads and changes, token use,
          membership changes and more. Admins see the log under{" "}
          <em>Audit log</em>, newest first, and can filter it by type. The
          overview shows the latest events.
        </p>
        <p>
          Each entry has the time, who did it (a person or a token), what
          happened, the resource, the project and the IP address. Actor,
          resource and IP are stored encrypted. Events for a project that has
          since been deleted are shown as <em>Deleted project</em>.
        </p>
        <h2>Event types</h2>
        <Table
          head={["Group", "Events"]}
          rows={[
            [
              <b key="a">Sign-in</b>,
              <code key="a2">
                auth.login, auth.register, auth.mfa_enabled, auth.mfa_disabled,
                auth.mfa_recovery_regenerated
              </code>,
            ],
            [
              <b key="s">Secrets</b>,
              <code key="s2">
                secret.create, update, delete, read, rotate, rollback, import,
                export, fetch, expired, compare
              </code>,
            ],
            [
              <b key="c">Change requests</b>,
              <code key="c2">
                change.request, view, approve, reject, cancel;
                environment.protect, environment.unprotect
              </code>,
            ],
            [
              <b key="t">Tokens</b>,
              <code key="t2">token.create, token.revoke, token.denied</code>,
            ],
            [
              <b key="m">Members</b>,
              <code key="m2">
                member.add, remove, role_change, invite, invite_revoke, join,
                env_roles
              </code>,
            ],
            [
              <b key="p">Projects</b>,
              <code key="p2">
                project.create, project.delete, environment.create,
                environment.delete
              </code>,
            ],
            [
              <b key="w">Webhooks and integrations</b>,
              <code key="w2">
                webhook.create, update, delete, test; integration.create,
                delete, sync
              </code>,
            ],
            [
              <b key="d">Dynamic secrets</b>,
              <code key="d2">dynamic.create, delete, lease, revoke</code>,
            ],
            [
              <b key="o">Backups</b>,
              <code key="o2">org.backup, org.restore</code>,
            ],
          ]}
        />
        <Note>
          Revealing or exporting a value is itself an audit event, so the log
          shows not only who changed a secret but who looked at it.
        </Note>
      </>
    ),
  },
  {
    slug: "cli",
    title: "CLI",
    group: "Reference",
    summary:
      "Log in once, link a folder, and run apps with their secrets, even offline.",
    keywords:
      "command line vaultly login setup pull run set whoami offline cache",
    body: () => (
      <>
        <p>
          One small binary that talks to the machine API with a service token.
        </p>
        <Cmd>{`cd backend && go build -o vaultly ./cmd/vaultly
# or, with the Docker setup:  docker compose exec backend vaultly pull`}</Cmd>
        <h2>Log in and link a folder</h2>
        <Cmd>{`vaultly login --url https://your-vaultly-host   # paste the token; input is hidden
vaultly setup --path /app                         # in your project folder
vaultly whoami                                     # check the scope and where settings come from`}</Cmd>
        <p>
          <code>login</code> checks the token and stores it in{" "}
          <code>~/.config/vaultly/credentials.json</code>, readable only by you.
          Use <code>--profile</code> to keep several tokens. <code>setup</code>{" "}
          writes a <code>.vaultly.json</code> that names the profile and folder.
          It holds no secrets, so you can commit it, and every teammate logs in
          with their own token.
        </p>
        <p>
          In CI, skip both and set <code>VAULTLY_URL</code> and{" "}
          <code>VAULTLY_TOKEN</code>. They override any stored profile.
        </p>
        <h2>Commands</h2>
        <Table
          head={["Command", "What it does"]}
          rows={[
            [
              <code key="1">vaultly pull [--format dotenv|shell|json]</code>,
              "Print the secrets of the token's environment, folders included.",
            ],
            [
              <code key="2">vaultly run -- cmd</code>,
              <>
                Start <code>cmd</code> with the secrets as environment
                variables.
              </>,
            ],
            [
              <code key="3">vaultly set KEY VALUE</code>,
              "Write one secret. The token must allow writes.",
            ],
            [
              <code key="4">vaultly whoami</code>,
              "Show the token's name, environment, folder and access.",
            ],
            [
              <code key="5">vaultly logout</code>,
              "Forget a profile and clear the offline cache.",
            ],
          ]}
        />
        <p>
          All of them take <code>--path /p</code> and{" "}
          <code>--profile name</code>. The folder defaults to the one in{" "}
          <code>.vaultly.json</code>, then to the token's own, and a token can
          never read outside its scope.
        </p>
        <h2>When the server is down</h2>
        <p>
          Each successful <code>pull</code> or <code>run</code> keeps an
          encrypted copy of what it fetched. If the server can't be reached, or
          answers with a server error, the CLI uses that copy and says so, so a
          deploy or a laptop on a train still starts.
        </p>
        <ul className="doc-list">
          <li>
            The copy is encrypted with a key derived from the token. Copied
            anywhere without the token, it can't be read.
          </li>
          <li>
            It is <b>never</b> used when the server says the token is invalid,
            revoked or out of scope. A revoked token stops working at once, and
            its copy is deleted.
          </li>
          <li>
            Copies older than 24 hours are ignored. Change this with{" "}
            <code>--max-age 2h</code>, or turn it off with{" "}
            <code>--max-age 0</code> or <code>--no-cache</code>.
          </li>
        </ul>
        <h2>Examples</h2>
        <Cmd>{`vaultly pull > .env                       # .env file
vaultly pull --format shell | source /dev/stdin
vaultly run -- npm start                   # no .env file needed`}</Cmd>
        <Note kind="tip">
          Prefer <code>vaultly run</code> in CI and containers. The secrets go
          straight into that process and no plain-text file is written. Add{" "}
          <code>--no-cache</code> where nothing should be written to disk at
          all.
        </Note>
      </>
    ),
  },
  {
    slug: "api",
    title: "REST API",
    group: "Reference",
    summary:
      "The endpoints for apps and scripts, with auth, limits and errors.",
    keywords:
      "api endpoints curl rest authentication rate limit status codes json",
    body: () => (
      <>
        <p>
          All endpoints are under <code>/api</code> and return JSON in one
          shape. The web app uses the same API.
        </p>
        <Cmd>{`{ "success": true,  "message": "Secrets exported", "data": { ... } }
{ "success": false, "message": "Secret not found" }`}</Cmd>
        <p>
          The exceptions are plain-text outputs (<code>format=dotenv</code> or{" "}
          <code>shell</code>) and file downloads.
        </p>
        <h2>Authentication</h2>
        <Table
          head={["Who", "Header"]}
          rows={[
            [
              "A machine (service token)",
              <code key="1">Authorization: Bearer vst_…</code>,
            ],
            [
              "A script acting as you (personal token)",
              <code key="2">Authorization: Bearer vpt_…</code>,
            ],
            [
              "The web app",
              "A session: a short-lived access token and a rotating refresh cookie.",
            ],
          ]}
        />
        <p>
          Service tokens work only on the machine endpoints under{" "}
          <code>/api/v1</code>.
        </p>
        <h2>Machine endpoints</h2>
        <Cmd>{`# read secrets
curl -H "Authorization: Bearer $VAULTLY_TOKEN" \\
  "$VAULTLY_URL/api/v1/secrets?recursive=true&format=json"

# write a secret (token must allow writes)
curl -X PUT -H "Authorization: Bearer $VAULTLY_TOKEN" -H "Content-Type: application/json" \\
  -d '{"path":"/","key":"API_KEY","value":"s3cret","comment":"payments"}' \\
  "$VAULTLY_URL/api/v1/secrets"

# short-lived database credentials
curl -X POST -H "Authorization: Bearer $VAULTLY_TOKEN" -H "Content-Type: application/json" \\
  -d '{"ttl_seconds":300}' "$VAULTLY_URL/api/v1/dynamic/readonly-db/lease"`}</Cmd>
        <p>
          Reading accepts <code>path</code>, <code>recursive</code>,{" "}
          <code>resolve</code> (expand references, default true) and{" "}
          <code>format</code> (<code>json</code>, <code>dotenv</code>,{" "}
          <code>shell</code>). Expired secrets are left out.
        </p>
        <h2>Common user endpoints</h2>
        <p>
          These need a personal token or a session. <code>:pid</code> is a
          project ID and <code>:org</code> a workspace ID.
        </p>
        <Table
          head={["Endpoint", "Purpose"]}
          rows={[
            [
              <code key="1">GET /orgs</code>,
              "Your workspaces and your role in each.",
            ],
            [
              <code key="2">GET|POST /orgs/:org/projects</code>,
              "List or create projects (admin).",
            ],
            [
              <code key="3">GET /projects/:pid/secrets?env=&path=/</code>,
              "List one folder. Values are not included.",
            ],
            [
              <code key="4">PUT /projects/:pid/secrets</code>,
              "Create or update a secret. This creates a new version.",
            ],
            [
              <code key="5">GET /projects/:pid/secrets/:sid</code>,
              "Reveal one value. Audited.",
            ],
            [
              <code key="6">POST /projects/:pid/secrets/:sid/rotate</code>,
              "Replace the value with a random one.",
            ],
            [
              <code key="7">POST /projects/:pid/secrets/:sid/rollback</code>,
              "Restore an earlier version.",
            ],
            [
              <code key="8">GET /projects/:pid/secrets/export</code>,
              "Export as dotenv, shell or JSON.",
            ],
            [
              <code key="9">POST /projects/:pid/tokens</code>,
              "Create a service token (admin). Shown once.",
            ],
            [
              <code key="10">GET /orgs/:org/audit</code>,
              "The audit log (admin), paged. Filter with kind=, project_id= and before=.",
            ],
          ]}
        />
        <h2>Rate limits</h2>
        <p>
          Every response carries <code>X-RateLimit-Limit</code>,{" "}
          <code>X-RateLimit-Remaining</code> and <code>X-RateLimit-Reset</code>.
          By default a token or user may make 600 requests a minute, and some
          routes are stricter (sign-in, revealing and exporting secrets,
          backups). When you hit a limit you get <code>429</code> and a{" "}
          <code>Retry-After</code> header.
        </p>
        <h2>Status codes</h2>
        <Table
          head={["Code", "Meaning"]}
          rows={[
            [
              <code key="1">400</code>,
              "Invalid input. The response lists the fields that failed.",
            ],
            [
              <code key="2">401</code>,
              "Not signed in, or the token is wrong or expired.",
            ],
            [
              <code key="3">403</code>,
              "Your role or token scope is too low, or the IP is not allowed.",
            ],
            [
              <code key="4">404</code>,
              "Not found, or not in your workspace. Existence is not revealed.",
            ],
            [
              <code key="5">409</code>,
              "Conflict, such as a duplicate or the last admin.",
            ],
            [<code key="6">429</code>, "Rate limited. Wait for Retry-After."],
          ]}
        />
      </>
    ),
  },
  {
    slug: "security",
    title: "Security and encryption",
    group: "Operations",
    summary:
      "What is encrypted, who holds the keys, and what is not protected.",
    keywords:
      "encryption aes openbao keys envelope kms master key threat model passwords sessions",
    body: () => (
      <>
        <h2>What is protected</h2>
        <ul className="doc-list">
          <li>
            <b>Secret values</b> are encrypted with AES-256-GCM, using a
            separate key for each project. A copy of the database alone shows
            only ciphertext.
          </li>
          <li>
            <b>People and sensitive text</b> are encrypted too: emails and
            names, workspace names, secret key names and comments, token
            prefixes, IP allowlists, webhook URLs, and the audit actor and IP
            address.
          </li>
          <li>
            <b>Left readable on purpose:</b> project, environment and folder
            names, tags and token names, so they can be browsed and sorted.
            Someone with a database dump could see how a team is organised, not
            what it stores.
          </li>
          <li>
            <b>Passwords</b> use bcrypt. <b>Tokens</b> are stored as hashes.
            Sessions use a 15-minute access token and a single-use rotating
            refresh cookie.
          </li>
        </ul>
        <h2>Who encrypts what</h2>
        <p>
          Vaultly does its own encryption: the backend encrypts every secret
          value with AES-256-GCM, using a key that belongs to one project. That
          key is generated by <b>OpenBao</b>, a separate key service, which
          keeps a root key that wraps every project key. OpenBao never sees a
          secret value, and its root key never leaves it.
        </p>
        <Cmd>{`secret value  --AES-256-GCM, project key-->  ciphertext in PostgreSQL
project key   --wrapped by the OpenBao root key-->  stored next to the project`}</Cmd>
        <p>
          This is envelope encryption, the same pattern hosted secret managers
          use with a cloud key service. Here the key service is self-hosted.
        </p>
        <p>
          OpenBao is asked only to unwrap a project key, never once per secret,
          and unwrapped keys are cached for 60 seconds. Normal reads do not wait
          on it, and a short outage does not stop the app. Rotating the root key
          re-wraps the stored project keys without re-encrypting any secret.
        </p>
        <h2>Running without OpenBao</h2>
        <p>
          For a simple self-hosted install you can skip it. The root key then
          comes from <code>MASTER_KEY</code> in the environment. That is simpler
          but protects the root key less well, so use OpenBao in production.
        </p>
        <Cmd>{`KEY_PROVIDER=local
MASTER_KEY=$(openssl rand -base64 32)`}</Cmd>
        <Note kind="warn">
          Back up the root key: the OpenBao data volumes, or{" "}
          <code>MASTER_KEY</code>. If it is lost, every secret becomes
          unreadable and cannot be recovered.
        </Note>
        <h2>Limits to know about</h2>
        <ul className="doc-list">
          <li>
            The running server holds the keys while it works, so a compromised
            server can read what it serves.
          </li>
          <li>
            The development setup unseals OpenBao with a single key stored on a
            Docker volume. In production use auto-unseal through a cloud key
            service, or split the key between people.
          </li>
          <li>
            Not built: single sign-on, end-to-end encryption, and multi-region
            failover.
          </li>
        </ul>
      </>
    ),
  },
  {
    slug: "self-hosting",
    title: "Self-hosting",
    group: "Operations",
    summary: "Run Vaultly yourself, configure it and keep it backed up.",
    keywords:
      "docker install deploy server configuration env variables backup restore production https troubleshooting",
    body: () => (
      <>
        <p>
          The hosted workspace and your own server run the same code. You need
          Docker with Compose v2 and nothing else.
        </p>
        <h2>Run it</h2>
        <Cmd>{`git clone https://github.com/saravana-devx/vaultly.git
git clone https://github.com/saravana-devx/Distributed-rate-limiter.git   # next to it
cd vaultly
cp .env.example .env      # set JWT_SECRET (openssl rand -hex 32)
make up                   # docker compose up -d --build`}</Cmd>
        <Table
          head={["Address", "What"]}
          rows={[
            [<code key="1">http://localhost:3000</code>, "The app."],
            [
              <code key="2">http://localhost:8080</code>,
              "The API (health check at /healthz).",
            ],
            [
              <code key="3">http://localhost:8025</code>,
              "Mailpit, which catches the emails Vaultly sends, such as invitations.",
            ],
            [
              <code key="4">http://localhost:8200</code>,
              "The OpenBao key service.",
            ],
          ]}
        />
        <p>
          <code>make down</code> stops everything and keeps your data.{" "}
          <code>make logs s=backend</code> follows one service, and{" "}
          <code>make help</code> lists every command. Never run{" "}
          <code>docker compose down -v</code> unless you want to delete all
          data.
        </p>
        <h2>What runs</h2>
        <p>
          Ten containers: the web app and the backend; PostgreSQL for data;
          Redis for rate-limit counters; Kafka for audit events; OpenBao and a
          helper that sets it up and unseals it; the separate rate-limiter
          service and its own database; and Mailpit.
        </p>
        <h2>Configuration</h2>
        <p>
          Settings live in <code>.env</code>. Every value has a default except
          the secrets.
        </p>
        <Table
          head={["Setting", "Meaning"]}
          rows={[
            [
              <code key="1">JWT_SECRET</code>,
              <>
                Signs sessions. Required, 32 or more characters.{" "}
                <code>openssl rand -hex 32</code>
              </>,
            ],
            [
              <code key="2">KEY_PROVIDER</code>,
              <>
                <code>openbao</code> (default) or <code>local</code>. See{" "}
                <D to="security">Security</D>.
              </>,
            ],
            [
              <code key="3">MASTER_KEY</code>,
              <>
                Root key for <code>local</code> mode. Optional with OpenBao.
              </>,
            ],
            [
              <code key="4">APP_URL</code>,
              "The public address of the app. Used in invitation links.",
            ],
            [
              <code key="5">CORS_ORIGINS</code>,
              "Browser origins allowed to call the API.",
            ],
            [
              <code key="6">
                SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS, SMTP_FROM
              </code>,
              "Outgoing email. Defaults to Mailpit in development.",
            ],
            [
              <code key="7">
                POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB
              </code>,
              "The main database.",
            ],
            [
              <code key="8">APP_PORT, API_PORT, …</code>,
              "Ports published on your machine.",
            ],
            [
              <code key="9">WEBHOOK_ALLOW_PRIVATE</code>,
              <>
                Lets webhooks call private addresses. <b>Development only</b>;
                leave it unset in production.
              </>,
            ],
          ]}
        />
        <h2>On a server</h2>
        <p>
          Use <code>docker-compose.prod.yml</code> (<code>make prod-up</code>).
          It puts Caddy in front, which gets an HTTPS certificate automatically,
          and only ports 80 and 443 are public. Put your domain and the
          generated secrets in <code>.env</code> (the repository's{" "}
          <code>deploy/env.prod.example</code> shows which). Use auto-unseal for
          OpenBao rather than the development setup.
        </p>
        <h2>Backups</h2>
        <ul className="doc-list">
          <li>
            <b>Whole server:</b> <code>make backup</code> saves a database dump
            and an OpenBao snapshot. Set <code>BACKUP_PASSPHRASE</code> to
            encrypt it. Both parts are needed to restore.
          </li>
          <li>
            <b>One workspace:</b> admins can download an encrypted backup from{" "}
            <em>Settings</em> and restore it later. It is sealed with your
            passphrase, and restoring needs the same root key. Tokens, webhooks
            and integration credentials are deliberately left out.
          </li>
        </ul>
        <h2>Troubleshooting</h2>
        <Table
          head={["Problem", "Try"]}
          rows={[
            [
              "A port is already in use",
              "Stop the program using it, or change the port in .env.",
            ],
            [
              "Containers are not healthy yet",
              <>
                Wait a minute, then run <code key="1">docker compose ps</code>.
                The first build is slow.
              </>,
            ],
            [
              "One service keeps failing",
              <>
                Read its logs: <code key="2">docker compose logs backend</code>.
              </>,
            ],
            [
              "Every secret fails to decrypt",
              "The root key does not match the data: the OpenBao volumes or MASTER_KEY changed.",
            ],
          ]}
        />
      </>
    ),
  },
];
