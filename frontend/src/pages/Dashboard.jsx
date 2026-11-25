import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Icon, ago, shortName } from '../components/ui.jsx'

const VERB = {
  'secret.create': 'created', 'secret.update': 'updated', 'secret.read': 'revealed', 'secret.delete': 'deleted', 'secret.rotate': 'rotated',
  'secret.rollback': 'restored', 'secret.fetch': 'fetched secrets at', 'secret.export': 'exported', 'secret.import': 'imported into', 'secret.expired': 'expired:',
  'token.create': 'created token', 'token.revoke': 'revoked token', 'token.denied': 'was blocked by IP allowlist:',
  'auth.login': 'signed in', 'auth.register': 'joined',
  'project.create': 'created project', 'project.delete': 'deleted project',
  'member.add': 'added', 'member.remove': 'removed', 'member.role_change': 'changed the role of', 'member.invite': 'invited', 'member.join': 'joined via invite', 'member.env_roles': 'changed environment access for',
  'environment.create': 'added environment', 'environment.delete': 'removed environment',
  'dynamic.lease': 'generated credentials for', 'dynamic.revoke': 'revoked credentials', 'dynamic.create': 'created dynamic secret',
  'integration.sync': 'synced', 'integration.create': 'added integration', 'webhook.create': 'added webhook',
  'org.backup': 'created a backup of', 'org.restore': 'restored a backup into',
}
const leaf = (r = '') => r.split('/').filter(Boolean).pop() || '/'
const isUUID = (s = '') => /^[0-9a-f]{8}-[0-9a-f]{4}-/.test(s)

function Spark({ data }) {
  const max = Math.max(1, ...data)
  const pts = data.map((v, i) => `${(i / Math.max(1, data.length - 1)) * 100},${26 - (v / max) * 24}`).join(' ')
  return <svg className="spark" viewBox="0 0 100 28" preserveAspectRatio="none" aria-hidden="true"><polyline points={pts} fill="none" stroke="currentColor" strokeWidth="1.6" vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" /></svg>
}

function Kpi({ label, value, sub, warn, to, children }) {
  const body = (
    <>
      <div className="kpi-l">{label}</div>
      <div className="kpi-v">{value ?? '–'}</div>
      <div className={`kpi-s ${warn ? 'warn' : ''}`}>{sub}</div>
      {children}
    </>
  )
  return to ? <Link className="kpi" to={to}>{body}</Link> : <div className="kpi">{body}</div>
}

function Chart({ data }) {
  const max = Math.max(1, ...data.map((d) => d.count))
  const top = max <= 4 ? 4 : Math.ceil(max / 4) * 4
  const fmt = (d) => new Date(d + 'T00:00:00').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  return (
    <div className="chart2">
      <div className="yaxis">{[top, top / 2, 0].map((v) => <span key={v}>{v}</span>)}</div>
      <div className="plot">
        <i className="gl" style={{ top: 0 }} /><i className="gl" style={{ top: '50%' }} /><i className="gl" style={{ bottom: 0 }} />
        <div className="bars">
          {data.map((d, i) => (
            <div className="col" key={d.day}>
              <span className="tip"><b>{d.count}</b> events · {fmt(d.day)}</span>
              <i className={i === data.length - 1 ? 'now' : ''} style={{ height: `${(d.count / top) * 100}%` }} />
            </div>
          ))}
        </div>
        <div className="xaxis"><span>{fmt(data[0].day)}</span><span>{fmt(data[Math.floor(data.length / 2)].day)}</span><span>Today</span></div>
      </div>
    </div>
  )
}

export default function Dashboard() {
  const { org, orgId, user } = useAuth()
  const [stats, setStats] = useState(null)
  const [projects, setProjects] = useState(null)
  const [range, setRange] = useState(14)
  const [allAttn, setAllAttn] = useState(false)
  const admin = org?.role === 'admin'

  useEffect(() => {
    api(`/orgs/${orgId}/stats`).then(setStats).catch(() => {})
    api(`/orgs/${orgId}/projects`).then(setProjects).catch(() => setProjects([]))
  }, [orgId])

  const attention = useMemo(() => {
    const urgency = (a) => (/expired/i.test(a.reason) ? -1 : Number(a.reason.match(/\d+/)?.[0] ?? 999))
    return [...(stats?.attention || [])].sort((a, b) => urgency(a) - urgency(b))
  }, [stats])
  const series = stats ? stats.activity.slice(-range) : null
  const total = series?.reduce((a, d) => a + d.count, 0)
  const maxSecrets = Math.max(1, ...(projects || []).map((p) => p.secret_count))
  const expiringTokens = attention.filter((a) => a.kind === 'token').length
  const onboarding = stats && (stats.projects === 0 || stats.secrets === 0 || stats.tokens === 0)
  const first = projects?.[0] ? `/app/projects/${projects[0].id}` : '/app/projects'
  const steps = onboarding && [
    ['Create a project', 'Each project gets its own encryption key.', stats.projects > 0, '/app/projects?new=1'],
    ['Add your first secrets', 'One by one, or import a .env file.', stats.secrets > 0, first],
    ['Create a service token', 'Lets CI or your app fetch secrets without a login.', stats.tokens > 0, first],
  ]
  const shownAttn = allAttn ? attention : attention.slice(0, 4)

  return (
    <>
      <div className="head">
        <div><h1>Overview</h1><p>{org?.name}</p></div>
      </div>

      <div className="kpis">
        <Kpi label="Secrets" value={stats?.secrets} sub={stats ? `across ${stats.projects} project${stats.projects === 1 ? '' : 's'}` : ' '} to="/app/projects" />
        <Kpi label="Service tokens" value={stats?.tokens} sub={stats ? (expiringTokens ? `${expiringTokens} need attention` : 'All healthy') : ' '} warn={expiringTokens > 0} />
        <Kpi label="Members" value={stats?.members} sub={stats ? (stats.pending_invites ? `${stats.pending_invites} invite${stats.pending_invites === 1 ? '' : 's'} pending` : 'No pending invites') : ' '} to="/app/members" />
        <Kpi label="Events · 24h" value={stats?.events_24h} sub={stats ? `${stats.activity.reduce((a, d) => a + d.count, 0).toLocaleString()} in 14 days` : ' '} to={admin ? '/app/audit' : undefined}>
          {stats && <Spark data={stats.activity.map((d) => d.count)} />}
        </Kpi>
      </div>

      {steps && (
        <section className="panel" style={{ marginBottom: 16 }}>
          <div className="panel-h"><h2>Get started</h2><span className="muted">{steps.filter((s) => s[2]).length} of 3 done</span></div>
          <div className="onb">{steps.map(([t, m, done, to]) => (
            <div key={t} className={done ? 'done' : ''}><span className="dot"><Icon name="check" size={12} stroke={3} /></span><div><div className="t">{t}</div><div className="m">{m}</div></div>
              {!done && <Link className="btn sm" to={to}>Start</Link>}</div>))}</div>
        </section>
      )}

      <div className="dgrid">
        <div className="dcol">
          <section className="panel">
            <div className="panel-h"><div><h2>Activity</h2><span className="muted">{total != null ? `${total.toLocaleString()} events` : ' '}</span></div>
              <div className="seg sm">{[7, 14].map((r) => <button key={r} className={range === r ? 'on' : ''} onClick={() => setRange(r)}>{r}d</button>)}</div></div>
            <div className="panel-b">{series ? <Chart data={series} /> : <div style={{ height: 190 }} />}</div>
          </section>

          <section className="panel">
            <div className="panel-h"><h2>Projects</h2><Link to="/app/projects" className="link">View all</Link></div>
            {projects && projects.length === 0 ? <div className="empty-in"><h3>No projects yet</h3><p>Projects hold your secrets, grouped by environment.</p></div> : (
              <div className="prow-wrap">
                <div className="prow hd"><span>Name</span><span>Environments</span><span>Secrets</span><span>Last activity</span></div>
                {(projects || []).slice(0, 6).map((p) => (
                  <Link to={`/app/projects/${p.id}`} key={p.id} className="prow">
                    <span className="nm"><Avatar name={p.name} square size={24} /><span>{p.name}</span></span>
                    <span className="chips">{p.environments.slice(0, 3).map((e) => <i key={e.id}>{e.slug}</i>)}{p.environments.length > 3 && <i>+{p.environments.length - 3}</i>}</span>
                    <span className="sc"><b>{p.secret_count}</b><u><i style={{ width: `${(p.secret_count / maxSecrets) * 100}%` }} /></u></span>
                    <span className="muted">{p.last_activity ? ago(p.last_activity) : '—'}</span>
                  </Link>))}
              </div>
            )}
          </section>
        </div>

        <div className="dcol">
          {admin && stats?.attention && (
            <section className="panel">
              <div className="panel-h"><h2>Needs attention{attention.length > 0 && <span className="count">{attention.length}</span>}</h2></div>
              {attention.length === 0 ? <div className="allgood"><Icon name="check" size={16} /> Nothing needs attention.</div> : (
                <div className="arows">
                  {shownAttn.map((a, i) => (
                    <Link key={i} to={`/app/projects/${a.project_id}`} className="arow">
                      <span className={`aic ${a.kind}`}><Icon name={a.kind === 'secret' ? 'key2' : 'alert'} size={14} /></span>
                      <span className="at"><b className={a.kind === 'secret' ? 'mono' : ''}>{a.token}</b><small>{a.project}</small></span>
                      <span className={`ar ${/expired/i.test(a.reason) ? 'bad' : ''}`}>{a.reason.replace('Secret expires', 'Expires').replace('Secret expired', 'Expired')}</span>
                    </Link>))}
                  {attention.length > 4 && <button className="more" onClick={() => setAllAttn(!allAttn)}>{allAttn ? 'Show fewer' : `Show ${attention.length - 4} more`}</button>}
                </div>
              )}
            </section>
          )}
          {admin && stats?.recent && (
            <section className="panel">
              <div className="panel-h"><h2>Recent activity</h2><Link to="/app/audit" className="link">View log</Link></div>
              <div className="frows">{stats.recent.slice(0, 6).map((e, i) => (
                <div key={i} className="frow"><Avatar name={e.actor.replace('token:', '')} size={22} />
                  <div className="ft"><b>{shortName(e.actor)}</b> {VERB[e.action] || e.action}{' '}
                    {e.resource && !['auth.login', 'auth.register'].includes(e.action) && !isUUID(e.resource) && <code>{leaf(e.resource)}</code>}
                    {e.project && <span className="muted"> in {e.project}</span>}</div>
                  <time>{ago(e.at)}</time></div>))}</div>
            </section>
          )}
          {!admin && (
            <section className="panel"><div className="panel-h"><h2>Your access</h2></div>
              <div className="panel-b muted" style={{ fontSize: 13.5 }}>You’re a <b style={{ color: 'var(--ink)' }}>{org?.role}</b> in {org?.name}. Signed in as {user.email}.</div></section>
          )}
        </div>
      </div>
    </>
  )
}
