import type { FormEvent } from 'react'
import type { Row } from '../types'
import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { useAuth } from '../auth'
import { Avatar, Icon, ago, shortName } from '../components/ui'
import { EnvTag } from '../components/table'

const VERB: Record<string, string> = {
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

// What kind of event this was, shown as a small status pill like a payments table's "Success / Pending".
function kindOf(action = '') {
  if (action === 'token.denied') return ['Blocked', 'bad']
  if (/^secret\.(read|fetch|export)$/.test(action) || action === 'dynamic.lease') return ['Access', 'info']
  if (/^secret\./.test(action) || /^integration\./.test(action)) return ['Change', 'ok']
  if (/^auth\./.test(action)) return ['Sign-in', 'plain']
  return ['Admin', 'plain']
}

function Stat({ icon, label, caption, value, pill, foot, to }: { icon?: string; label: string; caption?: string; value: any; pill?: any; foot?: string; to?: string }) {
  const body = (
    <>
      <div className="dstat-top"><span><i className="tile"><Icon name={icon} size={16} /></i>{label}</span>{caption && <small>{caption}</small>}</div>
      <div className="dstat-v"><b>{value ?? '–'}</b>{pill && <span className={`sp ${pill[1]}`}>{pill[0]}</span>}</div>
      <div className="dstat-f">{foot || ' '}</div>
    </>
  )
  return to ? <Link className="dstat" to={to}>{body}</Link> : <div className="dstat">{body}</div>
}

function Chart({ data }: { data: Row }) {
  const max = Math.max(1, ...data.map((d: Row) => d.count))
  const top = max <= 4 ? 4 : Math.ceil(max / 4) * 4
  const fmt = (d: string) => new Date(d + 'T00:00:00').toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  return (
    <div className="chart2">
      <div className="yaxis">{[top, top / 2, 0].map((v) => <span key={v}>{v}</span>)}</div>
      <div className="plot">
        <i className="gl" style={{ top: 0 }} /><i className="gl" style={{ top: '50%' }} /><i className="gl" style={{ bottom: 0 }} />
        <div className="bars">
          {data.map((d: Row, i: number) => (
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
  const [stats, setStats] = useState<Row | null>(null)
  const [projects, setProjects] = useState<Row | null>(null)
  const [range, setRange] = useState(14)
  const [allAttn, setAllAttn] = useState(false)
  const admin = org?.role === 'admin'

  useEffect(() => {
    api(`/orgs/${orgId}/stats`).then(setStats).catch(() => {})
    api(`/orgs/${orgId}/projects`).then(setProjects).catch(() => setProjects([]))
  }, [orgId])

  const attention = useMemo(() => {
    const urgency = (a: Row) => (/expired/i.test(a.reason) ? -1 : Number(a.reason.match(/\d+/)?.[0] ?? 999))
    return [...(stats?.attention || [])].sort((a, b) => urgency(a) - urgency(b))
  }, [stats])
  const series = stats ? stats.activity.slice(-range) : null
  const total = series?.reduce((a: any, d: any) => a + d.count, 0)
  const maxSecrets = Math.max(1, ...(projects || []).map((p: Row) => p.secret_count))
  const expiringTokens = attention.filter((a) => a.kind === 'token').length
  const onboarding = stats && (stats.projects === 0 || stats.secrets === 0 || stats.tokens === 0)
  const first = projects?.[0] ? `/app/projects/${projects[0].id}` : '/app/projects'
  const steps = onboarding && ([
    ['Create a project', 'Each project gets its own encryption key.', stats.projects > 0, '/app/projects?new=1'],
    ['Add your first secrets', 'One by one, or import a .env file.', stats.secrets > 0, first],
    ['Create a service token', 'Lets CI or your app fetch secrets without a login.', stats.tokens > 0, first],
  ] as [string, string, boolean, string][])
  const shownAttn = allAttn ? attention : attention.slice(0, 4)

  const envCount = (projects || []).reduce((n: any, p: any) => n + p.environments.length, 0)
  const perProject = stats && stats.projects ? Math.round(stats.secrets / stats.projects) : 0
  const hello = new Date().getHours() < 12 ? 'Good morning' : new Date().getHours() < 18 ? 'Good afternoon' : 'Good evening'

  return (
    <>
      <div className="head">
        <div><h1>{hello}, {shortName(user?.name)}</h1><p>Here is what is happening in {org?.name}.</p></div>
      </div>

      <section className="hero">
        <div>
          <span className="hero-l">Secrets under management</span>
          <div className="hero-v"><b>{stats?.secrets ?? '–'}</b>{stats && <span className="sp ok"><Icon name="shield" size={13} /> Encrypted at rest</span>}</div>
          <div className="hero-s">{stats ? `${stats.projects} project${stats.projects === 1 ? '' : 's'} · ${envCount} environment${envCount === 1 ? '' : 's'} · ${stats.events_24h} events in the last 24 hours` : ' '}</div>
        </div>
        <div className="hero-actions">
          {admin && <Link className="btn primary" to="/app/projects?new=1"><Icon name="plus" size={15} /> New project</Link>}
          <Link className="btn glass" to={first}><Icon name="key2" size={15} /> Add secret</Link>
          {admin && <Link className="btn glass" to="/app/members?invite=1"><Icon name="users" size={15} /> Invite</Link>}
        </div>
      </section>

      {steps && (
        <section className="panel" style={{ marginBottom: 16 }}>
          <div className="panel-h"><h2>Get started</h2><span className="muted">{steps.filter((s) => s[2]).length} of 3 done</span></div>
          <div className="onb">{steps.map(([t, m, done, to]) => (
            <div key={t} className={done ? 'done' : ''}><span className="dot"><Icon name="check" size={12} stroke={3} /></span><div><div className="t">{t}</div><div className="m">{m}</div></div>
              {!done && <Link className="btn sm" to={to}>Start</Link>}</div>))}</div>
        </section>
      )}

      <section className="dcard">
        <div className="dcard-h">
          <div className="dtitle"><i className="tile"><Icon name="pulse" size={16} /></i>Activity <span className="muted">{total != null ? `${total.toLocaleString()} events` : ''}</span></div>
          <div className="seg sm">{[7, 14].map((r) => <button key={r} className={range === r ? 'on' : ''} onClick={() => setRange(r)}>{r}d</button>)}</div>
        </div>
        <div className="dchart">
          <div className="dchart-main">{series ? <Chart data={series} /> : <div style={{ height: 210 }} />}</div>
          <div className="dchart-side">
            <div className="dsm">
              <div className="dsm-top"><i className="tile dark"><Icon name="log" size={16} /></i>Events · 24h</div>
              <div className="dsm-v">{stats?.events_24h ?? '–'}</div>
              <div className="dsm-f">{stats ? `${stats.activity.reduce((a: any, d: any) => a + d.count, 0).toLocaleString()} in the last 14 days` : ' '}</div>
            </div>
            <div className="dsm">
              <div className="dsm-top"><i className={`tile ${attention.length ? 'warn' : ''}`}><Icon name={attention.length ? 'alert' : 'check'} size={16} /></i>Needs attention</div>
              <div className="dsm-v">{stats?.attention ? attention.length : '–'}</div>
              <div className="dsm-f">{stats?.attention ? (attention.length ? 'expiring tokens and secrets' : 'everything is healthy') : 'visible to admins'}</div>
            </div>
          </div>
        </div>
      </section>

      <div className="dstats">
        <Stat icon="key2" label="Service tokens" caption="Machines" value={stats?.tokens} pill={stats ? (expiringTokens ? [`${expiringTokens} need attention`, 'warn'] : ['All healthy', 'ok']) : null} foot="Each token is limited to one environment and path." />
        <Stat icon="users" label="Members" caption="People" value={stats?.members} to="/app/members" pill={stats ? (stats.pending_invites ? [`${stats.pending_invites} pending`, 'info'] : ['No pending invites', 'plain']) : null} foot={`You are ${org?.role === 'admin' ? 'an' : 'a'} ${org?.role} in this workspace.`} />
        <Stat icon="layers" label="Projects" caption="Workspace" value={stats?.projects} to="/app/projects" pill={stats ? [`${envCount} environments`, 'plain'] : null} foot={stats ? `About ${perProject} secrets per project.` : ' '} />
      </div>

      <div className="dtwo">
        {admin && stats?.recent ? (
          <section className="dcard" style={{ marginBottom: 0 }}>
            <div className="dcard-h"><div className="dtitle"><i className="tile"><Icon name="log" size={16} /></i>Recent activity</div><Link to="/app/audit" className="link">View log</Link></div>
            <div className="atab">
              <div className="ar-h"><span>Event</span><span>Project</span><span>Type</span><span>When</span></div>
              {stats.recent.slice(0, 6).map((e: Row, i: number) => {
                const [kind, tone] = kindOf(e.action)
                return (
                  <div key={i} className="ar">
                    <div className="ev"><Avatar name={e.actor.replace('token:', '')} size={30} />
                      <div className="t"><b>{shortName(e.actor)}</b> {VERB[e.action] || e.action}{' '}
                        {e.resource && !['auth.login', 'auth.register'].includes(e.action) && !isUUID(e.resource) && <code>{leaf(e.resource)}</code>}</div></div>
                    <span className="proj">{e.project || '—'}</span>
                    <span><span className={`sp ${tone}`}>{kind}</span></span>
                    <time>{ago(e.at)}</time>
                  </div>)
              })}
            </div>
          </section>
        ) : (
          <section className="dcard" style={{ marginBottom: 0 }}><div className="dcard-h"><div className="dtitle"><i className="tile"><Icon name="shield" size={16} /></i>Your access</div></div>
            <div className="panel-b muted" style={{ fontSize: 13.5, paddingTop: 0 }}>You’re {org?.role === 'admin' ? 'an' : 'a'} <b style={{ color: 'var(--ink)' }}>{org?.role}</b> in {org?.name}. Signed in as {user?.email}.</div></section>
        )}

        {admin && stats?.attention && (
          <section className="dcard" style={{ marginBottom: 0 }}>
            <div className="dcard-h"><div className="dtitle"><i className={`tile ${attention.length ? 'warn' : ''}`}><Icon name="alert" size={16} /></i>Needs attention{attention.length > 0 && <span className="count">{attention.length}</span>}</div></div>
            {attention.length === 0 ? <div className="allgood"><Icon name="check" size={16} /> Nothing needs attention.</div> : (
              <div className="arows">
                {shownAttn.map((a, i) => (
                  <Link key={i} to={`/app/projects/${a.project_id}`} className="arow">
                    <span className={`aic ${a.kind}`}><Icon name={a.kind === 'secret' ? 'key2' : 'alert'} size={14} /></span>
                    <span className="at"><b className={a.kind === 'secret' ? 'mono' : ''}>{a.token}</b><small>{a.project}</small></span>
                    <span className={`ar ${/expired/i.test(a.reason) ? 'bad' : ''}`}>{a.reason.replace('Secret expires', 'Expires').replace('Secret expired', 'Expired').replace(/\b1 days\b/, '1 day')}</span>
                  </Link>))}
                {attention.length > 4 && <button className="more" onClick={() => setAllAttn(!allAttn)}>{allAttn ? 'Show fewer' : `Show ${attention.length - 4} more`}</button>}
              </div>
            )}
          </section>
        )}
      </div>

      <section className="dcard">
        <div className="dcard-h"><div className="dtitle"><i className="tile"><Icon name="layers" size={16} /></i>Projects</div><Link to="/app/projects" className="link">View all</Link></div>
        {projects && projects.length === 0 ? <div className="empty-in"><h3>No projects yet</h3><p>Projects hold your secrets, grouped by environment.</p></div> : (
          <div className="prow-wrap">
            <div className="prow hd"><span>Name</span><span>Environments</span><span>Secrets</span><span>Last activity</span></div>
            {(projects || []).slice(0, 6).map((p: Row) => (
              <Link to={`/app/projects/${p.id}`} key={p.id} className="prow">
                <span className="nm"><Avatar name={p.name} square size={28} /><span>{p.name}</span></span>
                <span className="chips">{p.environments.slice(0, 3).map((e: any) => <EnvTag key={e.id} slug={e.slug} />)}{p.environments.length > 3 && <i>+{p.environments.length - 3}</i>}</span>
                <span className="sc"><b>{p.secret_count}</b><u><i style={{ width: `${(p.secret_count / maxSecrets) * 100}%` }} /></u></span>
                <span className="muted">{p.last_activity ? ago(p.last_activity) : '—'}</span>
              </Link>))}
          </div>
        )}
      </section>
    </>
  )
}
