import type { FormEvent } from 'react'
import type { Row } from '../types'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../api'
import { Link } from 'react-router-dom'
import { Avatar, Expiry, Menu, Modal, Icon, copy, shortName } from '../components/ui'
import { ask, toast } from '../components/feedback'
import { EnvTag, StateDot, Rel, RowMenu, TableFoot, EmptyRow, plural } from '../components/table'
import Integrations from './project/Integrations'
import Webhooks from './project/Webhooks'
import Dynamic from './project/Dynamic'
import Compare from './project/Compare'
import Changes from './project/Changes'

export default function Project() {
  const { pid } = useParams()
  const nav = useNavigate()
  const [project, setProject] = useState<Row | null>(null)
  const [env, setEnv] = useState('')
  const [tab, setTab] = useState('secrets')
  const [pending, setPending] = useState(0)

  const load = useCallback(() => api(`/projects/${pid}`).then((p) => { setProject(p); setEnv((e) => (p.environments.some((x: Row) => x.slug === e) ? e : p.environments[0]?.slug || '')) }), [pid])
  useEffect(() => { setProject(null); setTab('secrets'); setEnv(''); load().catch(() => nav('/dashboard/projects')) }, [load, nav])
  useEffect(() => { api(`/projects/${pid}/changes?status=pending`).then((l) => setPending(l.length)).catch(() => {}) }, [pid, tab])
  if (!project) return <div className="spinner" style={{ margin: 40 }} />
  const admin = project.role === 'admin'
  const canWrite = project.role !== 'viewer'

  return (
    <>
      <div className="crumb"><Link to="/dashboard/projects">Projects</Link><Icon name="right" size={12} /><b>{project.name}</b></div>
      <div className="ptitle">
        <Avatar name={project.name} square size={40} />
        <div><h1>{project.name}</h1><div className="sub">{project.environments.length} environments · <span style={{ textTransform: 'capitalize' }}>{project.role}</span> access</div></div>
      </div>
      <div className="tabs">
        <button className={`tab ${tab === 'secrets' ? 'active' : ''}`} onClick={() => setTab('secrets')}>Secrets</button>
        {project.environments.length > 1 && <button className={`tab ${tab === 'compare' ? 'active' : ''}`} onClick={() => setTab('compare')}>Compare</button>}
        {(pending > 0 || project.environments.some((e: Row) => e.protected)) && <button className={`tab ${tab === 'changes' ? 'active' : ''}`} onClick={() => setTab('changes')}>Changes{pending > 0 && <span className="tab-count">{pending}</span>}</button>}
        {admin && <button className={`tab ${tab === 'tokens' ? 'active' : ''}`} onClick={() => setTab('tokens')}>Service tokens</button>}
        {admin && <button className={`tab ${tab === 'dynamic' ? 'active' : ''}`} onClick={() => setTab('dynamic')}>Dynamic secrets</button>}
        {admin && <button className={`tab ${tab === 'integrations' ? 'active' : ''}`} onClick={() => setTab('integrations')}>Integrations</button>}
        {admin && <button className={`tab ${tab === 'webhooks' ? 'active' : ''}`} onClick={() => setTab('webhooks')}>Webhooks</button>}
        {admin && <button className={`tab ${tab === 'settings' ? 'active' : ''}`} onClick={() => setTab('settings')}>Settings</button>}
      </div>
      {tab === 'secrets' && <Secrets project={project} env={env} setEnv={setEnv} canWrite={canWrite} />}
      {tab === 'compare' && <Compare project={project} />}
      {tab === 'changes' && <Changes project={project} onCount={setPending} />}
      {tab === 'tokens' && <Tokens project={project} />}
      {tab === 'dynamic' && <Dynamic project={project} />}
      {tab === 'integrations' && <Integrations project={project} />}
      {tab === 'webhooks' && <Webhooks project={project} />}
      {tab === 'settings' && <Settings project={project} reload={load} nav={nav} />}
    </>
  )
}

const AUTO_HIDE_MS = 15000

function EnvPicker({ project, env, setEnv }: { project: Row; env: string; setEnv: (...args: any[]) => void }) {
  return <div className="seg" role="tablist">{project.environments.map((e: any) => <button key={e.id} role="tab" aria-selected={env === e.slug} className={env === e.slug ? 'on' : ''} onClick={() => setEnv(e.slug)} title={e.protected ? 'Protected: changes need approval' : undefined}>{e.protected && <span className="lock"><Icon name="shield" size={12} /></span>}{e.name}</button>)}</div>
}

function Secrets({ project, env, setEnv, canWrite }: { project: Row; env: string; setEnv: (...args: any[]) => void; canWrite: boolean }) {
  const pid = project.id
  const gated = project.role !== 'admin' && !!project.environments.find((e: Row) => e.slug === env)?.protected
  const [path, setPath] = useState('/')
  const [data, setData] = useState<Row | null>(null)
  const [revealed, setRevealed] = useState<Record<string, any>>({})
  const [edit, setEdit] = useState<Row | null>(null)
  const [hist, setHist] = useState<Row | null>(null)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')
  const [tag, setTag] = useState('')
  const [allTags, setAllTags] = useState<string[]>([])
  const [resolved, setResolved] = useState<Record<string, any>>({})
  const [q, setQ] = useState('')
  const [found, setFound] = useState<Row[] | null>(null)

  const load = useCallback(() => {
    if (!env) return
    api(`/projects/${pid}/secrets?env=${env}&path=${encodeURIComponent(path)}${tag ? `&tag=${encodeURIComponent(tag)}` : ''}`).then((d) => {
      setData(d); setError('')
      if (!tag) setAllTags([...new Set<string>(d.secrets.flatMap((x: any) => x.tags || []))].sort())
    }).catch((e) => setError(e.message))
  }, [pid, env, path, tag])
  useEffect(() => { setRevealed({}); setResolved({}); load() }, [load])
  useEffect(() => { setTag('') }, [env, path])
  useEffect(() => {
    if (q.trim().length < 2) { setFound(null); return }
    const t = setTimeout(() => api(`/projects/${pid}/secrets/search?q=${encodeURIComponent(q.trim())}`).then((d) => setFound(d.results)).catch(() => setFound([])), 250)
    return () => clearTimeout(t)
  }, [q, pid])

  // A revealed value hides itself again after a short time, so it is never left on screen.
  const hideTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>())
  useEffect(() => {
    const timers = hideTimers.current
    const drop = (id: string) => (r: Record<string, any>) => { const n = { ...r }; delete n[id]; return n }
    Object.keys(revealed).forEach((id) => {
      if (timers.has(id)) return
      timers.set(id, setTimeout(() => { timers.delete(id); setRevealed(drop(id)); setResolved(drop(id)) }, AUTO_HIDE_MS))
    })
    timers.forEach((t, id) => { if (!(id in revealed)) { clearTimeout(t); timers.delete(id) } })
  }, [revealed])
  useEffect(() => () => hideTimers.current.forEach(clearTimeout), [])

  const reveal = async (s: Row) => {
    if (revealed[s.id] !== undefined) return setRevealed((r) => { const n = { ...r }; delete n[s.id]; return n })
    const d = await api(`/projects/${pid}/secrets/${s.id}`)
    setRevealed((r) => ({ ...r, [s.id]: d.value }))
    setResolved((r) => ({ ...r, [s.id]: d.resolved }))
  }
  const copyVal = async (s: Row) => { const d = await api(`/projects/${pid}/secrets/${s.id}`); copy(d.value, 'Copied to clipboard') }
  const del = async (s: Row) => { if (await ask(gated ? `Request deletion of ${s.key}? It is removed once someone else approves.` : `Delete ${s.key}?`, { confirm: gated ? 'Request deletion' : 'Delete', danger: true })) { await api(`/projects/${pid}/secrets/${s.id}`, { method: 'DELETE' }); load() } }
  const rotate = async (s: Row) => {
    if (!await ask(`Rotate ${s.key}? A new random value will replace the current one.`, { confirm: 'Rotate' })) return
    const d = await api(`/projects/${pid}/secrets/${s.id}/rotate`, { method: 'POST', body: {} })
    setRevealed((r) => ({ ...r, [s.id]: d.value })); load()
  }
  const exportEnv = async () => {
    const t = await api(`/projects/${pid}/secrets/export?env=${env}&path=${encodeURIComponent(path)}&format=dotenv`)
    const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([t], { type: 'text/plain' })); a.download = `${project.name}.${env}.env`; a.click()
  }

  const parts = path.split('/').filter(Boolean)
  const rows = data?.secrets || []
  const folders = tag ? [] : data?.folders || []
  const searching = q.trim().length >= 2
  const jump = (r: Row) => { setQ(''); setEnv(r.env); setPath(r.path) }
  const editSecret = async (s: Row) => { const d = await api(`/projects/${pid}/secrets/${s.id}`); setEdit({ ...s, value: d.value }) }
  return (
    <>
      <div className="bar">
        <EnvPicker project={project} env={env} setEnv={setEnv} />
        <div className="crumbs"><button onClick={() => setPath('/')}>/</button>
          {parts.map((p, i) => <span key={i}><button onClick={() => setPath('/' + parts.slice(0, i + 1).join('/'))}>{p}</button>{i < parts.length - 1 && '/'}</span>)}</div>
        <div className="search"><Icon name="search" size={15} /><input className="input" placeholder="Search all secrets" value={q} onChange={(e) => setQ(e.target.value)} /></div>
        <button className="btn" onClick={exportEnv}><Icon name="download" size={15} /> Export</button>
        {canWrite && !gated && <button className="btn" onClick={() => setImporting(true)}><Icon name="upload" size={15} /> Import</button>}
        {canWrite && <button className="btn primary" onClick={() => setEdit({ key: '', value: '', comment: '' })}><Icon name="plus" size={15} /> Add secret</button>}
      </div>
      {error && <div className="alert">{error}</div>}
      {!searching && allTags.length > 0 && (
        <div className="tagbar"><span className="muted">Tags</span>{allTags.map((t) => <button key={t} className={`chip ${tag === t ? 'on' : ''}`} onClick={() => setTag(tag === t ? '' : t)}>#{t}</button>)}</div>
      )}
      {searching && (
        <div className="scroll-x"><table className="table"><thead><tr><th style={{ width: '34%' }}>Key</th><th>Location</th><th>Tags</th><th style={{ width: 150 }}>Updated</th></tr></thead><tbody>
          {(found || []).map((r: Row) => (
            <tr key={r.id} className="clickable" onClick={() => jump(r)}>
              <td><div className="c-main key">{r.key}</div>{r.comment && <div className="c-sub">{r.comment}</div>}</td>
              <td><div className="state-line"><EnvTag slug={r.env} /><span className="mono dim" style={{ fontSize: 12.5 }}>{r.path}</span></div></td>
              <td>{(r.tags || []).map((t: any) => <span key={t} className="taglink">#{t}</span>)}</td>
              <td className="dim"><Rel iso={r.updated_at} /></td>
            </tr>))}
          {found && !found.length && <EmptyRow cols={4} icon="search" title="No matches" hint={`Nothing matches “${q}” in any environment you can access.`} />}
        </tbody></table>{found && <TableFoot>{plural(found.length, 'result')}</TableFoot>}</div>
      )}
      {!searching && data && (
        <div className="scroll-x"><table className="table"><thead><tr><th style={{ width: '34%' }}>Key</th><th>Value</th><th style={{ width: 200 }}>Updated</th><th style={{ width: 56 }} /></tr></thead><tbody>
          {path !== '/' && <tr className="folder clickable" onClick={() => setPath('/' + parts.slice(0, -1).join('/'))}><td colSpan={4}><div className="fname"><Icon name="folder" size={16} />..</div></td></tr>}
          {folders.map((f: string) => (
            <tr key={f} className="folder clickable" onClick={() => setPath(`${path === '/' ? '' : path}/${f}`)}><td colSpan={3}><div className="fname"><Icon name="folder" size={16} />{f}</div></td><td className="chev"><Icon name="right" size={15} /></td></tr>
          ))}
          {rows.map((s: Row) => {
            const shown = revealed[s.id] !== undefined
            return (
              <tr key={s.id}>
                <td>
                  <div className="c-main key">{s.key}{expiryBadge(s.expires_at)}</div>
                  {s.comment && <div className="c-sub">{s.comment}</div>}
                  {(s.tags || []).length > 0 && <div className="c-tags">{s.tags.map((t: any) => <button key={t} className="taglink" onClick={() => setTag(t)}>#{t}</button>)}</div>}
                </td>
                <td>
                  <div className="vcell">
                    <span className={`v ${shown ? 'shown' : ''}`}>{shown ? revealed[s.id] : '••••••••••••'}</span>
                    <span className="vb">
                      <button className="iconbtn" title={shown ? 'Hide value' : `Reveal value (hides after ${AUTO_HIDE_MS / 1000} s)`} aria-label={shown ? 'Hide value' : 'Reveal value'} onClick={() => reveal(s)}><Icon name={shown ? 'eyeoff' : 'eye'} size={15} /></button>
                      <button className="iconbtn" title="Copy value" aria-label="Copy value" onClick={() => copyVal(s)}><Icon name="copy" size={15} /></button>
                    </span>
                  </div>
                  {shown && resolved[s.id] && <div className="c-sub mono" title="Value after ${references} are resolved">→ {resolved[s.id]}</div>}
                </td>
                <td>
                  <div className="c-main light"><Rel iso={s.updated_at} /> <span className="dim">· v{s.version}</span></div>
                  <div className="c-sub who sm"><Avatar name={shortName(s.updated_by) || '?'} size={16} />{shortName(s.updated_by)}</div>
                </td>
                <td>
                  <RowMenu items={[
                    canWrite && { label: 'Edit', icon: 'pencil', run: () => editSecret(s) },
                    { label: 'Version history', icon: 'history', run: () => setHist(s) },
                    canWrite && !gated && { label: 'Rotate value', icon: 'rotate', run: () => rotate(s) },
                    canWrite && { label: gated ? 'Request deletion' : 'Delete', icon: 'trash', danger: true, run: () => del(s) },
                  ]} />
                </td>
              </tr>)
          })}
          {!folders.length && !rows.length && <EmptyRow cols={4} icon="key2" title={tag ? `No secrets tagged #${tag}` : 'No secrets here yet'} hint={tag ? 'Clear the tag filter to see everything in this folder.' : 'Add one, or import a .env file.'} />}
        </tbody></table>
        <TableFoot>{plural(rows.length, 'secret')}{folders.length > 0 && ` · ${plural(folders.length, 'folder')}`}</TableFoot></div>
      )}
      {edit && <SecretModal pid={pid} env={env} path={path} initial={edit} gated={gated} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load() }} />}
      {hist && <History pid={pid} secret={hist} canWrite={canWrite && !gated} onClose={() => setHist(null)} onChanged={load} />}
      {importing && <ImportModal pid={pid} env={env} path={path} onClose={() => setImporting(false)} onDone={() => { setImporting(false); load() }} />}
    </>
  )
}

function expiryBadge(iso: string) {
  if (!iso) return null
  const ms = new Date(iso).getTime() - Date.now()
  if (ms <= 0) return <span className="exp bad" title={new Date(iso).toLocaleString()}>Expired</span>
  const days = Math.ceil(ms / 86400000)
  return <span className={`exp ${days <= 7 ? 'warn' : ''}`} title={new Date(iso).toLocaleString()}>{days <= 1 ? 'Expires today' : `Expires in ${days}d`}</span>
}

const toDateInput = (iso: string) => (iso ? new Date(iso).toISOString().slice(0, 10) : '')

function SecretModal({ pid, env, path, initial, gated, onClose, onSaved }: { pid: string; env: string; path: string; initial: Row; gated?: boolean; onClose: (...args: any[]) => void; onSaved: (...args: any[]) => void }) {
  const isEdit = !!initial.id
  const [f, setF] = useState<Row>({ key: initial.key, value: initial.value, comment: initial.comment || '', path: initial.path || path,
    tags: (initial.tags || []).join(', '), expires: toDateInput(initial.expires_at) })
  const [err, setErr] = useState('')
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    const body: Row = { env, key: f.key, value: f.value, comment: f.comment, path: f.path, tags: f.tags.split(",").map((t: string) => t.trim()).filter(Boolean) }
    if (f.expires) body.expires_at = new Date(`${f.expires}T23:59:59`).toISOString()
    else if (initial.expires_at) body.clear_expiry = true
    try { await api(`/projects/${pid}/secrets`, { method: 'PUT', body }); onSaved() } catch (x) { setErr(x.message) }
  }
  return (
    <Modal title={isEdit ? `Edit ${initial.key}` : 'Add secret'} onClose={onClose}>
      <form onSubmit={submit}>
        {err && <div className="alert">{err}</div>}
        {gated && <div className="alert warn"><b>{env}</b> is protected. Saving opens a change request, and someone else has to approve it before apps see it.</div>}
        <div className="field"><label>Key</label><input className="input mono" disabled={isEdit} autoFocus={!isEdit} required value={f.key} onChange={(e) => setF({ ...f, key: e.target.value.toUpperCase().replace(/[^A-Z0-9_.-]/g, '_') })} placeholder="DATABASE_URL" /></div>
        <div className="field"><label>Value<span className="hint">{'Reference others with ${KEY}, ${/path/KEY} or ${env:/path/KEY}'}</span></label><textarea className="input" required value={f.value} onChange={(e) => setF({ ...f, value: e.target.value })} spellCheck={false} /></div>
        <div className="field"><label>Path</label><input className="input mono" disabled={isEdit} value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} /></div>
        {!gated && <div className="field"><label>Tags<span className="hint">Comma separated</span></label><input className="input" value={f.tags} onChange={(e) => setF({ ...f, tags: e.target.value })} placeholder="database, stripe" /></div>}
        {!gated && <div className="field"><label>Expires<span className="hint">Optional — expired secrets stop being served to apps</span></label>
          <div style={{ display: 'flex', gap: 8 }}><input className="input" type="date" min={new Date().toISOString().slice(0, 10)} value={f.expires} onChange={(e) => setF({ ...f, expires: e.target.value })} />
            {f.expires && <button type="button" className="btn" onClick={() => setF({ ...f, expires: '' })}>Clear</button>}</div></div>}
        <div className="field"><label>Comment<span className="hint">Optional</span></label><input className="input" value={f.comment} onChange={(e) => setF({ ...f, comment: e.target.value })} /></div>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">{gated ? 'Request change' : 'Save'}</button></div>
      </form>
    </Modal>
  )
}

function ImportModal({ pid, env, path, onClose, onDone }: { pid: string; env: string; path: string; onClose: (...args: any[]) => void; onDone: (...args: any[]) => void }) {
  const [content, setContent] = useState('')
  const [err, setErr] = useState('')
  return (
    <Modal title={`Import .env into ${env} ${path}`} onClose={onClose}>
      <form onSubmit={async (e) => { e.preventDefault(); try { const r = await api(`/projects/${pid}/secrets/import`, { method: 'POST', body: { env, path, content }, quiet: true }); toast(`${r.created} created, ${r.updated} updated`); onDone() } catch (x) { setErr(x.message) } }}>
        {err && <div className="alert">{err}</div>}
        <div className="field"><textarea className="input" style={{ minHeight: 220 }} autoFocus required value={content} onChange={(e) => setContent(e.target.value)} placeholder={'DATABASE_URL=postgres://...\nAPI_KEY="abc123"'} spellCheck={false} /></div>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Import</button></div>
      </form>
    </Modal>
  )
}

function History({ pid, secret, canWrite, onClose, onChanged }: { pid: string; secret: Row; canWrite: boolean; onClose: (...args: any[]) => void; onChanged: (...args: any[]) => void }) {
  const [rows, setRows] = useState<Row[]>([])
  const load = () => api(`/projects/${pid}/secrets/${secret.id}/versions`).then(setRows)
  useEffect(() => { load() }, [])
  return (
    <Modal title={`History · ${secret.key}`} onClose={onClose} wide>
      <table className="table"><thead><tr><th>Ver</th><th>Action</th><th>By</th><th>When</th><th /></tr></thead><tbody>
        {rows.map((r, i) => (
          <tr key={r.version}><td>v{r.version}</td><td className="c-main light" style={{ textTransform: "capitalize" }}>{r.action}</td><td className="muted" style={{ fontSize: 13 }}>{r.actor}</td><td className="dim"><Rel iso={r.at} /></td>
            <td>{canWrite && i > 0 && <button className="btn sm" onClick={async () => { if (await ask(`Restore v${r.version} as a new version?`, { confirm: 'Restore' })) { await api(`/projects/${pid}/secrets/${secret.id}/rollback`, { method: 'POST', body: { version: r.version } }); load(); onChanged() } }}>Restore</button>}</td></tr>))}
      </tbody></table>
      <div className="modal-actions"><button className="btn" onClick={onClose}>Close</button></div>
    </Modal>
  )
}

function Tokens({ project }: { project: Row }) {
  const pid = project.id
  const [list, setList] = useState<Row[]>([])
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<string | null>(null)
  const [f, setF] = useState<Row>({ name: '', env: project.environments[0]?.slug, path: '/', can_write: false, expire_days: 90, ips: '' })
  const [err, setErr] = useState('')
  const load = () => api(`/projects/${pid}/tokens`).then(setList)
  useEffect(() => { load() }, [])
  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}><div className="muted" style={{ flex: 1 }}>Machine credentials for CI/CD and runtime. Each token is scoped to one environment and path.</div>
        <button className="btn primary" onClick={() => { setErr(''); setOpen(true) }}><Icon name="plus" size={15} /> New token</button></div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Name</th><th>Scope</th><th>Access</th><th>Allowed IPs</th><th>Expires</th><th>Last used</th><th style={{ width: 56 }} /></tr></thead><tbody>
        {list.map((t) => (
          <tr key={t.id}>
            <td><div className="c-main">{t.name}</div><div className="c-sub mono">{t.prefix}…</div></td>
            <td><div className="state-line"><EnvTag slug={t.env} /><span className="mono dim" style={{ fontSize: 12.5 }}>{t.path}</span></div></td>
            <td><StateDot tone={t.can_write ? 'warn' : 'ok'}>{t.can_write ? 'Read & write' : 'Read only'}</StateDot></td>
            <td className="dim" title={(t.allowed_cidrs || []).join(', ')}>{(t.allowed_cidrs || []).length ? plural(t.allowed_cidrs.length, 'range') : 'Any IP'}</td>
            <td><Expiry iso={t.expires_at} /></td>
            <td className="dim"><Rel iso={t.last_used_at} /></td>
            <td><RowMenu items={[{ label: 'Revoke token', icon: 'trash', danger: true, run: async () => { if (await ask(`Revoke ${t.name}?`, { confirm: 'Revoke', danger: true })) { await api(`/projects/${pid}/tokens/${t.id}`, { method: 'DELETE' }); load() } } }]} /></td>
          </tr>))}
        {!list.length && <EmptyRow cols={7} icon="key2" title="No service tokens" hint="Create one so CI or an app can fetch secrets." />}
      </tbody></table><TableFoot>{plural(list.length, 'token')}</TableFoot></div>
      {open && (
        <Modal title="New service token" onClose={() => setOpen(false)}>
          <form onSubmit={async (e) => { e.preventDefault(); try { const d = await api(`/projects/${pid}/tokens`, { method: 'POST', body: { name: f.name, env: f.env, path: f.path, can_write: f.can_write, expire_days: +f.expire_days, allowed_cidrs: f.ips.split(/[\s,]+/).filter(Boolean) } }); setOpen(false); setCreated(d.token); load() } catch (x) { setErr(x.message) } }}>
            {err && <div className="alert">{err}</div>}
            <div className="field"><label>Name</label><input className="input" required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="github-actions" /></div>
            <div className="field"><label>Environment</label><select className="input" value={f.env} onChange={(e) => setF({ ...f, env: e.target.value })}>{project.environments.map((e: any) => <option key={e.id} value={e.slug}>{e.name}</option>)}</select></div>
            <div className="field"><label>Path prefix</label><input className="input mono" value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} /></div>
            <div className="field"><label>Allowed IPs<span className="hint">Optional — addresses or CIDR ranges, separated by commas</span></label><input className="input mono" value={f.ips} onChange={(e) => setF({ ...f, ips: e.target.value })} placeholder="203.0.113.7, 10.0.0.0/8" /></div>
            <div className="field"><label>Expires in (days, 0 = never)</label><input className="input" type="number" min="0" value={f.expire_days} onChange={(e) => setF({ ...f, expire_days: e.target.value })} /></div>
            <label className="check" style={{ marginBottom: 14 }}><input type="checkbox" checked={f.can_write} onChange={(e) => setF({ ...f, can_write: e.target.checked })} /> Allow writing secrets</label>
            <div className="modal-actions"><button type="button" className="btn" onClick={() => setOpen(false)}>Cancel</button><button className="btn primary">Create token</button></div>
          </form>
        </Modal>
      )}
      {created && (
        <Modal title="Token created" onClose={() => setCreated(null)}>
          <div className="alert warn">Copy this token now — it won't be shown again.</div>
          <div className="tokenbox">{created}</div>
          <p className="muted mono" style={{ fontSize: 12.5 }}>export VAULTLY_TOKEN={created.slice(0, 12)}…<br />vaultly run -- ./your-app</p>
          <div className="modal-actions"><button className="btn" onClick={async () => copy(created, 'Copied')}>Copy</button><button className="btn primary" onClick={() => setCreated(null)}>Done</button></div>
        </Modal>
      )}
    </>
  )
}

function Settings({ project, reload, nav }: { project: Row; reload: (...args: any[]) => void; nav: (...args: any[]) => void }) {
  const [name, setName] = useState('')
  const [err, setErr] = useState('')
  return (
    <>
      <div className="sect" style={{ marginTop: 28 }}><h2>Environments</h2></div>
      {err && <div className="alert">{err}</div>}
      <table className="table"><thead><tr><th>Name</th><th>Slug</th><th>Approvals<span className="hint" style={{ fontWeight: 400 }}> · non-admin changes need a second person</span></th><th /></tr></thead><tbody>
        {project.environments.map((e: Row) => (
          <tr key={e.id}><td style={{ fontWeight: 500 }}>{e.name}</td><td className="mono muted" style={{ fontSize: 13 }}>{e.slug}</td>
            <td><span className="sw-row"><label className="switch"><input type="checkbox" aria-label={`Require approval for ${e.name}`} checked={!!e.protected} onChange={async (ev) => { try { await api(`/projects/${project.id}/environments/${e.id}`, { method: 'PATCH', body: { protected: ev.target.checked } }); reload() } catch (x) { setErr(x.message) } }} /><i /></label>{e.protected ? 'Required' : 'Off'}</span></td>
            <td style={{ textAlign: 'right' }}><button className="btn sm ghost danger" onClick={async () => { if (await ask(`Delete ${e.name} and all its secrets?`, { confirm: 'Delete', danger: true })) { try { await api(`/projects/${project.id}/environments/${e.id}`, { method: 'DELETE' }); reload() } catch (x) { setErr(x.message) } } }}>Delete</button></td></tr>))}
      </tbody></table>
      <form className="bar" style={{ paddingTop: 16 }} onSubmit={async (e) => { e.preventDefault(); try { await api(`/projects/${project.id}/environments`, { method: 'POST', body: { name } }); setName(''); setErr(''); reload() } catch (x) { setErr(x.message) } }}>
        <input className="input" style={{ maxWidth: 260, height: 34 }} placeholder="New environment, e.g. QA" value={name} onChange={(e) => setName(e.target.value)} required /><button className="btn">Add environment</button></form>
      <div className="danger-zone">
        <div><b>Delete this project</b><span>Permanently removes all secrets, versions and tokens. This can’t be undone.</span></div>
        <button className="btn danger" onClick={async () => { if (await ask(`Delete "${project.name}" and ALL its secrets? This cannot be undone.`, { confirm: 'Delete', danger: true })) { await api(`/projects/${project.id}`, { method: 'DELETE' }); window.dispatchEvent(new Event('vaultly:projects')); nav('/dashboard/projects') } }}>Delete project</button>
      </div>
    </>
  )
}
