import { useEffect, useState } from 'react'
import { api } from '../../api.js'
import { Icon, Modal, ago } from '../../components/ui.jsx'

const PROVIDERS = {
  github: {
    name: 'GitHub Actions', blurb: 'Repository or environment secrets',
    fields: [['token', 'Personal access token', 'password', 'Needs repo scope (or Actions secrets: write)'], ['repo', 'Repository', 'text', 'owner/name'], ['environment', 'GitHub environment (optional)', 'text', 'production']],
  },
  vercel: {
    name: 'Vercel', blurb: 'Project environment variables',
    fields: [['token', 'Access token', 'password', ''], ['project_id', 'Project ID or name', 'text', 'prj_…'], ['team_id', 'Team ID (optional)', 'text', 'team_…'], ['target', 'Targets', 'text', 'production, preview']],
  },
  railway: {
    name: 'Railway', blurb: 'Service variables',
    fields: [['token', 'API token', 'password', ''], ['project_id', 'Project ID', 'text', ''], ['environment_id', 'Environment ID', 'text', ''], ['service_id', 'Service ID (optional)', 'text', '']],
  },
}

export default function Integrations({ project, toast }) {
  const pid = project.id
  const [list, setList] = useState(null)
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState('')
  const load = () => api(`/projects/${pid}/integrations`).then(setList)
  useEffect(() => { load() }, [pid])

  const sync = async (i) => {
    setBusy(i.id)
    try { const r = await api(`/projects/${pid}/integrations/${i.id}/sync`, { method: 'POST' }); toast(`Pushed ${r.pushed} secret${r.pushed === 1 ? '' : 's'}${r.skipped?.length ? `, skipped ${r.skipped.length}` : ''}`) } catch (e) { toast(e.message) }
    setBusy(''); load()
  }

  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}>
        <div className="muted" style={{ flex: 1 }}>Push secrets to the platforms that run your code. Credentials are encrypted at rest and never shown again.</div>
        <button className="btn primary" onClick={() => setOpen(true)}><Icon name="plus" size={15} /> Add integration</button>
      </div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Name</th><th>Source</th><th>Destination</th><th>Auto-sync</th><th>Last sync</th><th style={{ width: 160 }} /></tr></thead><tbody>
        {(list || []).map((i) => (
          <tr key={i.id}>
            <td style={{ fontWeight: 500 }}>{i.name}<div className="cm">{PROVIDERS[i.provider]?.name}</div></td>
            <td><span className="chip static">{i.env}</span> <span className="mono muted" style={{ fontSize: 12.5 }}>{i.path}</span></td>
            <td className="mono muted" style={{ fontSize: 12.5 }}>{i.config.repo || i.config.project_id || '—'}{i.config.environment ? ` · ${i.config.environment}` : ''}</td>
            <td><label className="switch"><input type="checkbox" checked={i.auto_sync} onChange={async (e) => { await api(`/projects/${pid}/integrations/${i.id}`, { method: 'PATCH', body: { auto_sync: e.target.checked } }); load() }} /><i /></label></td>
            <td>{i.last_synced_at ? <><span className={`dot-s ${i.last_status}`} /> {ago(i.last_synced_at)}{i.last_error && <div className="cm err" title={i.last_error}>{i.last_error}</div>}</> : <span className="muted">Never</span>}</td>
            <td><div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
              <button className="btn sm" disabled={busy === i.id} onClick={() => sync(i)}>{busy === i.id ? 'Syncing…' : 'Sync now'}</button>
              <button className="iconbtn danger" title="Remove" aria-label="Remove" onClick={async () => { if (confirm(`Remove ${i.name}? Secrets already pushed stay where they are.`)) { await api(`/projects/${pid}/integrations/${i.id}`, { method: 'DELETE' }); load() } }}><Icon name="trash" size={16} /></button>
            </div></td>
          </tr>))}
        {list && !list.length && <tr><td colSpan="6" className="muted" style={{ textAlign: 'center', padding: 44 }}>No integrations yet. Add GitHub Actions, Vercel or Railway.</td></tr>}
      </tbody></table></div>
      <p className="muted" style={{ fontSize: 13, marginTop: 14 }}>Syncs add and update values. Secrets deleted here are not removed from the destination.</p>
      {open && <AddIntegration project={project} onClose={() => setOpen(false)} onDone={() => { setOpen(false); load() }} />}
    </>
  )
}

function AddIntegration({ project, onClose, onDone }) {
  const [provider, setProvider] = useState('github')
  const [f, setF] = useState({ name: '', env: project.environments[0]?.slug, path: '/', auto_sync: true })
  const [cfg, setCfg] = useState({})
  const [err, setErr] = useState('')
  const P = PROVIDERS[provider]
  return (
    <Modal title="Add integration" onClose={onClose}>
      <div className="seg" style={{ marginBottom: 18 }}>{Object.entries(PROVIDERS).map(([k, v]) => <button key={k} className={provider === k ? 'on' : ''} onClick={() => { setProvider(k); setCfg({}) }}>{v.name}</button>)}</div>
      <form onSubmit={async (e) => { e.preventDefault(); try { await api(`/projects/${project.id}/integrations`, { method: 'POST', body: { provider, ...f, config: cfg } }); onDone() } catch (x) { setErr(x.message) } }}>
        {err && <div className="alert">{err}</div>}
        <div className="field"><label>Name</label><input className="input" required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder={`${P.name} – production`} /></div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <div className="field"><label>Environment</label><select className="input" value={f.env} onChange={(e) => setF({ ...f, env: e.target.value })}>{project.environments.map((e) => <option key={e.id} value={e.slug}>{e.name}</option>)}</select></div>
          <div className="field"><label>Path<span className="hint">and below</span></label><input className="input mono" value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} /></div>
        </div>
        {P.fields.map(([k, label, type, ph]) => (
          <div className="field" key={k}><label>{label}{ph && type === 'password' && <span className="hint">{ph}</span>}</label>
            <input className="input" type={type} autoComplete="off" placeholder={type === 'password' ? '' : ph} required={!label.includes('optional')} value={cfg[k] || ''} onChange={(e) => setCfg({ ...cfg, [k]: e.target.value })} /></div>
        ))}
        <label className="check" style={{ marginBottom: 16 }}><input type="checkbox" checked={f.auto_sync} onChange={(e) => setF({ ...f, auto_sync: e.target.checked })} /> Sync automatically when secrets change</label>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Add integration</button></div>
      </form>
    </Modal>
  )
}
