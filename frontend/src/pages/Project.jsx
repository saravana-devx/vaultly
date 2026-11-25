import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { api } from '../api.js'
import { Link } from 'react-router-dom'
import { Avatar, Menu, Modal, Icon, useToast, copy, ago, shortName } from '../components/ui.jsx'
import Integrations from './project/Integrations.jsx'
import Webhooks from './project/Webhooks.jsx'
import Dynamic from './project/Dynamic.jsx'

export default function Project() {
  const { pid } = useParams()
  const nav = useNavigate()
  const [project, setProject] = useState(null)
  const [env, setEnv] = useState('')
  const [tab, setTab] = useState('secrets')
  const [toast, toastEl] = useToast()

  const load = useCallback(() => api(`/projects/${pid}`).then((p) => { setProject(p); setEnv((e) => (p.environments.some((x) => x.slug === e) ? e : p.environments[0]?.slug || '')) }), [pid])
  useEffect(() => { setProject(null); setTab('secrets'); setEnv(''); load().catch(() => nav('/app/projects')) }, [load, nav])
  if (!project) return <div className="spinner" style={{ margin: 40 }} />
  const admin = project.role === 'admin'
  const canWrite = project.role !== 'viewer'

  return (
    <>
      <div className="crumb"><Link to="/app/projects">Projects</Link><Icon name="right" size={12} /><b>{project.name}</b></div>
      <div className="ptitle">
        <Avatar name={project.name} square size={40} />
        <div><h1>{project.name}</h1><div className="sub">{project.environments.length} environments · <span style={{ textTransform: 'capitalize' }}>{project.role}</span> access</div></div>
      </div>
      <div className="tabs">
        <button className={`tab ${tab === 'secrets' ? 'active' : ''}`} onClick={() => setTab('secrets')}>Secrets</button>
        {admin && <button className={`tab ${tab === 'tokens' ? 'active' : ''}`} onClick={() => setTab('tokens')}>Service tokens</button>}
        {admin && <button className={`tab ${tab === 'dynamic' ? 'active' : ''}`} onClick={() => setTab('dynamic')}>Dynamic secrets</button>}
        {admin && <button className={`tab ${tab === 'integrations' ? 'active' : ''}`} onClick={() => setTab('integrations')}>Integrations</button>}
        {admin && <button className={`tab ${tab === 'webhooks' ? 'active' : ''}`} onClick={() => setTab('webhooks')}>Webhooks</button>}
        {admin && <button className={`tab ${tab === 'settings' ? 'active' : ''}`} onClick={() => setTab('settings')}>Settings</button>}
      </div>
      {tab === 'secrets' && <Secrets project={project} env={env} setEnv={setEnv} canWrite={canWrite} toast={toast} />}
      {tab === 'tokens' && <Tokens project={project} toast={toast} />}
      {tab === 'dynamic' && <Dynamic project={project} toast={toast} />}
      {tab === 'integrations' && <Integrations project={project} toast={toast} />}
      {tab === 'webhooks' && <Webhooks project={project} toast={toast} />}
      {tab === 'settings' && <Settings project={project} reload={load} nav={nav} />}
      {toastEl}
    </>
  )
}

function EnvPicker({ project, env, setEnv }) {
  return <div className="seg" role="tablist">{project.environments.map((e) => <button key={e.id} role="tab" aria-selected={env === e.slug} className={env === e.slug ? 'on' : ''} onClick={() => setEnv(e.slug)}>{e.name}</button>)}</div>
}

function Secrets({ project, env, setEnv, canWrite, toast }) {
  const pid = project.id
  const [path, setPath] = useState('/')
  const [data, setData] = useState(null)
  const [revealed, setRevealed] = useState({})
  const [edit, setEdit] = useState(null)
  const [hist, setHist] = useState(null)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')
  const [tag, setTag] = useState('')
  const [allTags, setAllTags] = useState([])
  const [resolved, setResolved] = useState({})
  const [q, setQ] = useState('')
  const [found, setFound] = useState(null)

  const load = useCallback(() => {
    if (!env) return
    api(`/projects/${pid}/secrets?env=${env}&path=${encodeURIComponent(path)}${tag ? `&tag=${encodeURIComponent(tag)}` : ''}`).then((d) => {
      setData(d); setError('')
      if (!tag) setAllTags([...new Set(d.secrets.flatMap((x) => x.tags || []))].sort())
    }).catch((e) => setError(e.message))
  }, [pid, env, path, tag])
  useEffect(() => { setRevealed({}); setResolved({}); load() }, [load])
  useEffect(() => { setTag('') }, [env, path])
  useEffect(() => {
    if (q.trim().length < 2) { setFound(null); return }
    const t = setTimeout(() => api(`/projects/${pid}/secrets/search?q=${encodeURIComponent(q.trim())}`).then((d) => setFound(d.results)).catch(() => setFound([])), 250)
    return () => clearTimeout(t)
  }, [q, pid])

  const reveal = async (s) => {
    if (revealed[s.id] !== undefined) return setRevealed((r) => { const n = { ...r }; delete n[s.id]; return n })
    const d = await api(`/projects/${pid}/secrets/${s.id}`)
    setRevealed((r) => ({ ...r, [s.id]: d.value }))
    setResolved((r) => ({ ...r, [s.id]: d.resolved }))
  }
  const copyVal = async (s) => { const d = await api(`/projects/${pid}/secrets/${s.id}`); toast((await copy(d.value)) ? 'Copied to clipboard' : 'Copy failed') }
  const del = async (s) => { if (confirm(`Delete ${s.key}?`)) { await api(`/projects/${pid}/secrets/${s.id}`, { method: 'DELETE' }); load() } }
  const rotate = async (s) => {
    if (!confirm(`Rotate ${s.key}? A new random value will replace the current one.`)) return
    const d = await api(`/projects/${pid}/secrets/${s.id}/rotate`, { method: 'POST', body: {} })
    setRevealed((r) => ({ ...r, [s.id]: d.value })); load(); toast('Rotated — new value shown')
  }
  const exportEnv = async () => {
    const t = await api(`/projects/${pid}/secrets/export?env=${env}&path=${encodeURIComponent(path)}&format=dotenv`)
    const a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([t], { type: 'text/plain' })); a.download = `${project.name}.${env}.env`; a.click()
  }

  const parts = path.split('/').filter(Boolean)
  const rows = data?.secrets || []
  const folders = tag ? [] : data?.folders || []
  const searching = q.trim().length >= 2
  const jump = (r) => { setQ(''); setEnv(r.env); setPath(r.path) }
  const editSecret = async (s) => { const d = await api(`/projects/${pid}/secrets/${s.id}`); setEdit({ ...s, value: d.value }) }
  return (
    <>
      <div className="bar">
        <EnvPicker project={project} env={env} setEnv={setEnv} />
        <div className="crumbs"><button onClick={() => setPath('/')}>/</button>
          {parts.map((p, i) => <span key={i}><button onClick={() => setPath('/' + parts.slice(0, i + 1).join('/'))}>{p}</button>{i < parts.length - 1 && '/'}</span>)}</div>
        <div className="search"><Icon name="search" size={15} /><input className="input" placeholder="Search all secrets" value={q} onChange={(e) => setQ(e.target.value)} /></div>
        <button className="btn" onClick={exportEnv}><Icon name="download" size={15} /> Export</button>
        {canWrite && <button className="btn" onClick={() => setImporting(true)}><Icon name="upload" size={15} /> Import</button>}
        {canWrite && <button className="btn primary" onClick={() => setEdit({ key: '', value: '', comment: '' })}><Icon name="plus" size={15} /> Add secret</button>}
      </div>
      {error && <div className="alert">{error}</div>}
      {!searching && allTags.length > 0 && (
        <div className="tagbar"><span className="muted">Tags</span>{allTags.map((t) => <button key={t} className={`chip ${tag === t ? 'on' : ''}`} onClick={() => setTag(tag === t ? '' : t)}>#{t}</button>)}</div>
      )}
      {searching && (
        <div className="scroll-x"><table className="table"><thead><tr><th style={{ width: '34%' }}>Key</th><th>Location</th><th>Tags</th><th style={{ width: 140 }}>Updated</th></tr></thead><tbody>
          {(found || []).map((r) => (
            <tr key={r.id} className="folder" onClick={() => jump(r)}>
              <td><div className="key">{r.key}</div>{r.comment && <div className="cm">{r.comment}</div>}</td>
              <td><span className="chip static">{r.env}</span> <span className="mono muted" style={{ fontSize: 12.5 }}>{r.path}</span></td>
              <td>{(r.tags || []).map((t) => <span className="chip static" key={t}>#{t}</span>)}</td>
              <td className="muted">{ago(r.updated_at)}</td>
            </tr>))}
          {found && !found.length && <tr><td colSpan="4" className="muted" style={{ textAlign: 'center', padding: 48 }}>Nothing matches “{q}” in any environment you can access.</td></tr>}
        </tbody></table></div>
      )}
      {!searching && data && (
        <div className="scroll-x"><table className="table"><thead><tr><th style={{ width: '34%' }}>Key</th><th>Value</th><th style={{ width: 170 }}>Last updated</th><th style={{ width: 110 }} /></tr></thead><tbody>
          {path !== '/' && <tr className="folder" onClick={() => setPath('/' + parts.slice(0, -1).join('/'))}><td colSpan="4"><div className="fname"><Icon name="folder" size={16} />..</div></td></tr>}
          {folders.map((f) => (
            <tr key={f} className="folder" onClick={() => setPath(`${path === '/' ? '' : path}/${f}`)}><td colSpan="3"><div className="fname"><Icon name="folder" size={16} />{f}</div></td><td style={{ textAlign: 'right', color: 'var(--muted)' }}><Icon name="right" size={15} /></td></tr>
          ))}
          {rows.map((s) => {
            const shown = revealed[s.id] !== undefined
            return (
              <tr key={s.id}>
                <td><div className="key">{s.key}{expiryBadge(s.expires_at)}</div>{s.comment && <div className="cm">{s.comment}</div>}
                  {(s.tags || []).length > 0 && <div className="tagrow">{s.tags.map((t) => <button key={t} className="chip" onClick={() => setTag(t)}>#{t}</button>)}</div>}</td>
                <td className={`val ${shown ? 'shown' : ''}`}>{shown ? <>{revealed[s.id]}{resolved[s.id] && <div className="cm" title="Value after ${references} are resolved">→ {resolved[s.id]}</div>}</> : <span className="mask">••••••••••••</span>}</td>
                <td><div className="upd"><span className="ver">v{s.version}</span> <span style={{ marginLeft: 6 }}>{ago(s.updated_at)}</span><small>by {shortName(s.updated_by)}</small></div></td>
                <td><div className="acts">
                  <button className="iconbtn" title={shown ? 'Hide value' : 'Reveal value'} aria-label="Reveal" onClick={() => reveal(s)}><Icon name={shown ? 'eyeoff' : 'eye'} size={16} /></button>
                  <button className="iconbtn" title="Copy value" aria-label="Copy" onClick={() => copyVal(s)}><Icon name="copy" size={16} /></button>
                  <Menu items={[
                    canWrite && { label: 'Edit', icon: 'pencil', run: () => editSecret(s) },
                    { label: 'Version history', icon: 'history', run: () => setHist(s) },
                    canWrite && { label: 'Rotate value', icon: 'rotate', run: () => rotate(s) },
                    canWrite && { label: 'Delete', icon: 'trash', danger: true, run: () => del(s) },
                  ]} />
                </div></td>
              </tr>)
          })}
          {!folders.length && !rows.length && <tr><td colSpan="4" className="muted" style={{ textAlign: 'center', padding: 56 }}>{tag ? `No secrets tagged #${tag} here.` : 'No secrets here yet. Add one or import a .env file.'}</td></tr>}
        </tbody></table></div>
      )}
      {edit && <SecretModal pid={pid} env={env} path={path} initial={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load() }} />}
      {hist && <History pid={pid} secret={hist} canWrite={canWrite} onClose={() => setHist(null)} onChanged={load} />}
      {importing && <ImportModal pid={pid} env={env} path={path} onClose={() => setImporting(false)} onDone={() => { setImporting(false); load() }} toast={toast} />}
    </>
  )
}

function expiryBadge(iso) {
  if (!iso) return null
  const ms = new Date(iso) - Date.now()
  if (ms <= 0) return <span className="exp bad" title={new Date(iso).toLocaleString()}>Expired</span>
  const days = Math.ceil(ms / 86400000)
  return <span className={`exp ${days <= 7 ? 'warn' : ''}`} title={new Date(iso).toLocaleString()}>{days <= 1 ? 'Expires today' : `Expires in ${days}d`}</span>
}

const toDateInput = (iso) => (iso ? new Date(iso).toISOString().slice(0, 10) : '')

function SecretModal({ pid, env, path, initial, onClose, onSaved }) {
  const isEdit = !!initial.id
  const [f, setF] = useState({ key: initial.key, value: initial.value, comment: initial.comment || '', path: initial.path || path,
    tags: (initial.tags || []).join(', '), expires: toDateInput(initial.expires_at) })
  const [err, setErr] = useState('')
  const submit = async (e) => {
    e.preventDefault()
    const body = { env, key: f.key, value: f.value, comment: f.comment, path: f.path, tags: f.tags.split(',').map((t) => t.trim()).filter(Boolean) }
    if (f.expires) body.expires_at = new Date(`${f.expires}T23:59:59`).toISOString()
    else if (initial.expires_at) body.clear_expiry = true
    try { await api(`/projects/${pid}/secrets`, { method: 'PUT', body }); onSaved() } catch (x) { setErr(x.message) }
  }
  return (
    <Modal title={isEdit ? `Edit ${initial.key}` : 'Add secret'} onClose={onClose}>
      <form onSubmit={submit}>
        {err && <div className="alert">{err}</div>}
        <div className="field"><label>Key</label><input className="input mono" disabled={isEdit} autoFocus={!isEdit} required value={f.key} onChange={(e) => setF({ ...f, key: e.target.value.toUpperCase().replace(/[^A-Z0-9_.-]/g, '_') })} placeholder="DATABASE_URL" /></div>
        <div className="field"><label>Value<span className="hint">{'Reference others with ${KEY}, ${/path/KEY} or ${env:/path/KEY}'}</span></label><textarea className="input" required value={f.value} onChange={(e) => setF({ ...f, value: e.target.value })} spellCheck={false} /></div>
        <div className="field"><label>Path</label><input className="input mono" disabled={isEdit} value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} /></div>
        <div className="field"><label>Tags<span className="hint">Comma separated</span></label><input className="input" value={f.tags} onChange={(e) => setF({ ...f, tags: e.target.value })} placeholder="database, stripe" /></div>
        <div className="field"><label>Expires<span className="hint">Optional — expired secrets stop being served to apps</span></label>
          <div style={{ display: 'flex', gap: 8 }}><input className="input" type="date" min={new Date().toISOString().slice(0, 10)} value={f.expires} onChange={(e) => setF({ ...f, expires: e.target.value })} />
            {f.expires && <button type="button" className="btn" onClick={() => setF({ ...f, expires: '' })}>Clear</button>}</div></div>
        <div className="field"><label>Comment<span className="hint">Optional</span></label><input className="input" value={f.comment} onChange={(e) => setF({ ...f, comment: e.target.value })} /></div>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Save</button></div>
      </form>
    </Modal>
  )
}

function ImportModal({ pid, env, path, onClose, onDone, toast }) {
  const [content, setContent] = useState('')
  const [err, setErr] = useState('')
  return (
    <Modal title={`Import .env into ${env} ${path}`} onClose={onClose}>
      <form onSubmit={async (e) => { e.preventDefault(); try { const r = await api(`/projects/${pid}/secrets/import`, { method: 'POST', body: { env, path, content } }); toast(`${r.created} created, ${r.updated} updated`); onDone() } catch (x) { setErr(x.message) } }}>
        {err && <div className="alert">{err}</div>}
        <div className="field"><textarea className="input" style={{ minHeight: 220 }} autoFocus required value={content} onChange={(e) => setContent(e.target.value)} placeholder={'DATABASE_URL=postgres://...\nAPI_KEY="abc123"'} spellCheck={false} /></div>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Import</button></div>
      </form>
    </Modal>
  )
}

function History({ pid, secret, canWrite, onClose, onChanged }) {
  const [rows, setRows] = useState([])
  const load = () => api(`/projects/${pid}/secrets/${secret.id}/versions`).then(setRows)
  useEffect(() => { load() }, [])
  return (
    <Modal title={`History · ${secret.key}`} onClose={onClose} wide>
      <table className="table"><thead><tr><th>Ver</th><th>Action</th><th>By</th><th>When</th><th /></tr></thead><tbody>
        {rows.map((r, i) => (
          <tr key={r.version}><td>v{r.version}</td><td><span className="badge">{r.action}</span></td><td className="muted" style={{ fontSize: 13 }}>{r.actor}</td><td className="muted">{ago(r.at)}</td>
            <td>{canWrite && i > 0 && <button className="btn sm" onClick={async () => { if (confirm(`Restore v${r.version} as a new version?`)) { await api(`/projects/${pid}/secrets/${secret.id}/rollback`, { method: 'POST', body: { version: r.version } }); load(); onChanged() } }}>Restore</button>}</td></tr>))}
      </tbody></table>
      <div className="modal-actions"><button className="btn" onClick={onClose}>Close</button></div>
    </Modal>
  )
}

function Tokens({ project, toast }) {
  const pid = project.id
  const [list, setList] = useState([])
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState(null)
  const [f, setF] = useState({ name: '', env: project.environments[0]?.slug, path: '/', can_write: false, expire_days: 90, ips: '' })
  const [err, setErr] = useState('')
  const load = () => api(`/projects/${pid}/tokens`).then(setList)
  useEffect(() => { load() }, [])
  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}><div className="muted" style={{ flex: 1 }}>Machine credentials for CI/CD and runtime. Each token is scoped to one environment and path.</div>
        <button className="btn primary" onClick={() => { setErr(''); setOpen(true) }}><Icon name="plus" size={15} /> New token</button></div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Name</th><th>Token</th><th>Scope</th><th>Access</th><th>Allowed IPs</th><th>Expires</th><th>Last used</th><th /></tr></thead><tbody>
        {list.map((t) => (
          <tr key={t.id}><td style={{ fontWeight: 500 }}>{t.name}</td><td className="mono muted">{t.prefix}…</td><td className="mono" style={{ fontSize: 13 }}>{t.env}:{t.path}</td>
            <td><span className="tag">{t.can_write ? 'read/write' : 'read-only'}</span></td>
            <td className="muted" title={(t.allowed_cidrs || []).join(', ')}>{(t.allowed_cidrs || []).length ? `${t.allowed_cidrs.length} range${t.allowed_cidrs.length > 1 ? 's' : ''}` : 'Any'}</td>
            <td className="muted">{t.expires_at ? new Date(t.expires_at).toLocaleDateString() : 'never'}</td><td className="muted">{ago(t.last_used_at)}</td>
            <td><div style={{ textAlign: 'right' }}><button className="btn sm ghost danger" onClick={async () => { if (confirm(`Revoke ${t.name}?`)) { await api(`/projects/${pid}/tokens/${t.id}`, { method: 'DELETE' }); load() } }}>Revoke</button></div></td></tr>))}
        {!list.length && <tr><td colSpan="8" className="muted" style={{ textAlign: 'center', padding: 36 }}>No service tokens.</td></tr>}
      </tbody></table></div>
      {open && (
        <Modal title="New service token" onClose={() => setOpen(false)}>
          <form onSubmit={async (e) => { e.preventDefault(); try { const d = await api(`/projects/${pid}/tokens`, { method: 'POST', body: { name: f.name, env: f.env, path: f.path, can_write: f.can_write, expire_days: +f.expire_days, allowed_cidrs: f.ips.split(/[\s,]+/).filter(Boolean) } }); setOpen(false); setCreated(d.token); load() } catch (x) { setErr(x.message) } }}>
            {err && <div className="alert">{err}</div>}
            <div className="field"><label>Name</label><input className="input" required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="github-actions" /></div>
            <div className="field"><label>Environment</label><select className="input" value={f.env} onChange={(e) => setF({ ...f, env: e.target.value })}>{project.environments.map((e) => <option key={e.id} value={e.slug}>{e.name}</option>)}</select></div>
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
          <div className="modal-actions"><button className="btn" onClick={async () => toast((await copy(created)) ? 'Copied' : 'Copy failed')}>Copy</button><button className="btn primary" onClick={() => setCreated(null)}>Done</button></div>
        </Modal>
      )}
    </>
  )
}

function Settings({ project, reload, nav }) {
  const [name, setName] = useState('')
  const [err, setErr] = useState('')
  return (
    <>
      <div className="sect" style={{ marginTop: 28 }}><h2>Environments</h2></div>
      {err && <div className="alert">{err}</div>}
      <table className="table"><thead><tr><th>Name</th><th>Slug</th><th /></tr></thead><tbody>
        {project.environments.map((e) => (
          <tr key={e.id}><td style={{ fontWeight: 500 }}>{e.name}</td><td className="mono muted" style={{ fontSize: 13 }}>{e.slug}</td>
            <td style={{ textAlign: 'right' }}><button className="btn sm ghost danger" onClick={async () => { if (confirm(`Delete ${e.name} and all its secrets?`)) { try { await api(`/projects/${project.id}/environments/${e.id}`, { method: 'DELETE' }); reload() } catch (x) { setErr(x.message) } } }}>Delete</button></td></tr>))}
      </tbody></table>
      <form className="bar" style={{ paddingTop: 16 }} onSubmit={async (e) => { e.preventDefault(); try { await api(`/projects/${project.id}/environments`, { method: 'POST', body: { name } }); setName(''); setErr(''); reload() } catch (x) { setErr(x.message) } }}>
        <input className="input" style={{ maxWidth: 260, height: 34 }} placeholder="New environment, e.g. QA" value={name} onChange={(e) => setName(e.target.value)} required /><button className="btn">Add environment</button></form>
      <div className="danger-zone">
        <div><b>Delete this project</b><span>Permanently removes all secrets, versions and tokens. This can’t be undone.</span></div>
        <button className="btn danger" onClick={async () => { if (confirm(`Delete "${project.name}" and ALL its secrets? This cannot be undone.`)) { await api(`/projects/${project.id}`, { method: 'DELETE' }); window.dispatchEvent(new Event('vaultly:projects')); nav('/app/projects') } }}>Delete project</button>
      </div>
    </>
  )
}
