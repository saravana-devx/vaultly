import { useEffect, useState } from 'react'
import { api } from '../../api.js'
import { Icon, Menu, Modal, ago, copy } from '../../components/ui.jsx'

const fmtTTL = (s) => (s % 3600 === 0 ? `${s / 3600}h` : s % 60 === 0 ? `${s / 60}m` : `${s}s`)
const left = (iso) => {
  const s = Math.floor((new Date(iso) - Date.now()) / 1000)
  if (s <= 0) return 'expired'
  return s >= 3600 ? `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m left` : s >= 60 ? `${Math.floor(s / 60)}m left` : `${s}s left`
}

export default function Dynamic({ project, toast }) {
  const pid = project.id
  const [list, setList] = useState(null)
  const [open, setOpen] = useState(false)
  const [lease, setLease] = useState(null)
  const [leases, setLeases] = useState(null)
  const load = () => api(`/projects/${pid}/dynamic`).then(setList)
  useEffect(() => { load() }, [pid])

  const generate = async (d) => {
    try { setLease({ cfg: d, ...(await api(`/projects/${pid}/dynamic/${d.id}/lease`, { method: 'POST', body: {} })) }); load() } catch (e) { toast(e.message) }
  }
  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}>
        <div className="muted" style={{ flex: 1 }}>Generate database credentials on demand. Each one is unique, expires automatically, and is deleted from the database when it does.</div>
        <button className="btn primary" onClick={() => setOpen(true)}><Icon name="plus" size={15} /> New dynamic secret</button>
      </div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Name</th><th>Environment</th><th>Target</th><th>Lifetime</th><th>Active</th><th style={{ width: 190 }} /></tr></thead><tbody>
        {(list || []).map((d) => (
          <tr key={d.id}>
            <td style={{ fontWeight: 500 }}>{d.name}<div className="cm">PostgreSQL</div></td>
            <td><span className="chip static">{d.env}</span></td>
            <td className="mono muted" style={{ fontSize: 12.5 }}>{d.target.admin_user}@{d.target.host}/{d.target.database}</td>
            <td className="muted">{fmtTTL(d.default_ttl_s)} <span style={{ opacity: .7 }}>(max {fmtTTL(d.max_ttl_s)})</span></td>
            <td>{d.active_leases}</td>
            <td><div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end' }}>
              <button className="btn sm" onClick={() => generate(d)}>Generate credentials</button>
              <Menu items={[
                { label: 'Leases', icon: 'history', run: () => setLeases(d) },
                { label: 'Delete', icon: 'trash', danger: true, run: async () => { if (confirm(`Delete ${d.name}? All active credentials are revoked.`)) { await api(`/projects/${pid}/dynamic/${d.id}`, { method: 'DELETE' }); load() } } },
              ]} />
            </div></td>
          </tr>))}
        {list && !list.length && <tr><td colSpan="6" className="muted" style={{ textAlign: 'center', padding: 44 }}>No dynamic secrets yet.</td></tr>}
      </tbody></table></div>
      {list?.length > 0 && <p className="muted" style={{ fontSize: 13, marginTop: 14 }}>From CI or an app, use a service token: <span className="mono">POST /api/v1/dynamic/{list[0].name}/lease</span></p>}
      {open && <AddDynamic project={project} onClose={() => setOpen(false)} onDone={() => { setOpen(false); load() }} />}
      {lease && (
        <Modal title="Credentials generated" onClose={() => setLease(null)}>
          <div className="alert warn">This password won’t be shown again. It expires {new Date(lease.expires_at).toLocaleString()} ({left(lease.expires_at)}).</div>
          {[['Username', lease.username], ['Password', lease.password], ['Connection URL', lease.connection_url]].map(([k, v]) => (
            <div className="field" key={k}><label>{k}</label>
              <div style={{ display: 'flex', gap: 8 }}><input className="input mono" readOnly value={v} onFocus={(e) => e.target.select()} /><button type="button" className="btn" onClick={async () => toast((await copy(v)) ? `${k} copied` : 'Copy failed')}>Copy</button></div></div>))}
          <div className="modal-actions"><button className="btn primary" onClick={() => setLease(null)}>Done</button></div>
        </Modal>
      )}
      {leases && <Leases pid={pid} cfg={leases} onClose={() => { setLeases(null); load() }} toast={toast} />}
    </>
  )
}

function AddDynamic({ project, onClose, onDone }) {
  const [f, setF] = useState({ name: '', env: project.environments[0]?.slug, admin_url: '', grants: 'GRANT pg_read_all_data TO {{name}}', ttl: 60, max: 1440 })
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  return (
    <Modal title="New dynamic secret" onClose={onClose}>
      <form onSubmit={async (e) => {
        e.preventDefault(); setBusy(true); setErr('')
        try { await api(`/projects/${project.id}/dynamic`, { method: 'POST', body: { name: f.name, env: f.env, admin_url: f.admin_url, grants: f.grants.split('\n').map((x) => x.trim()).filter(Boolean), default_ttl_s: f.ttl * 60, max_ttl_s: f.max * 60 } }); onDone() } catch (x) { setErr(x.message); setBusy(false) }
      }}>
        {err && <div className="alert">{err}</div>}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <div className="field"><label>Name</label><input className="input" required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="readonly-db" /></div>
          <div className="field"><label>Environment</label><select className="input" value={f.env} onChange={(e) => setF({ ...f, env: e.target.value })}>{project.environments.map((e) => <option key={e.id} value={e.slug}>{e.name}</option>)}</select></div>
        </div>
        <div className="field"><label>Admin connection URL<span className="hint">Needs CREATEROLE — stored encrypted</span></label><input className="input mono" type="password" required autoComplete="off" value={f.admin_url} onChange={(e) => setF({ ...f, admin_url: e.target.value })} placeholder="postgres://admin:password@db.internal:5432/app" /></div>
        <div className="field"><label>Grants<span className="hint">One SQL statement per line. {'{{name}}'} is the new role.</span></label><textarea className="input" style={{ minHeight: 80 }} value={f.grants} onChange={(e) => setF({ ...f, grants: e.target.value })} spellCheck={false} /></div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <div className="field"><label>Default lifetime<span className="hint">minutes</span></label><input className="input" type="number" min="1" value={f.ttl} onChange={(e) => setF({ ...f, ttl: +e.target.value })} /></div>
          <div className="field"><label>Maximum lifetime<span className="hint">minutes</span></label><input className="input" type="number" min="1" value={f.max} onChange={(e) => setF({ ...f, max: +e.target.value })} /></div>
        </div>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{busy ? 'Checking connection…' : 'Create'}</button></div>
      </form>
    </Modal>
  )
}

function Leases({ pid, cfg, onClose, toast }) {
  const [rows, setRows] = useState(null)
  const load = () => api(`/projects/${pid}/dynamic/${cfg.id}/leases`).then(setRows)
  useEffect(() => { load() }, [])
  return (
    <Modal title={`Leases · ${cfg.name}`} onClose={onClose} wide>
      <table className="table"><thead><tr><th>Role</th><th>State</th><th>Created by</th><th>Expires</th><th /></tr></thead><tbody>
        {(rows || []).map((l) => (
          <tr key={l.id}><td className="mono" style={{ fontSize: 12.5 }}>{l.username}</td>
            <td><span className={`tag ${l.state === 'active' ? 'admin' : ''}`}>{l.state}</span>{l.revoke_error && <div className="cm err" title={l.revoke_error}>revoke failed</div>}</td>
            <td className="muted" style={{ fontSize: 13 }}>{l.created_by}</td><td className="muted">{l.state === 'active' ? left(l.expires_at) : ago(l.revoked_at || l.expires_at)}</td>
            <td style={{ textAlign: 'right' }}>{l.state !== 'revoked' && <button className="btn sm ghost danger" onClick={async () => { try { await api(`/projects/${pid}/dynamic/${cfg.id}/leases/${l.id}`, { method: 'DELETE' }); toast('Revoked'); load() } catch (e) { toast(e.message) } }}>Revoke now</button>}</td></tr>))}
        {rows && !rows.length && <tr><td colSpan="5" className="muted" style={{ textAlign: 'center', padding: 32 }}>No credentials generated yet.</td></tr>}
      </tbody></table>
      <div className="modal-actions"><button className="btn" onClick={onClose}>Close</button></div>
    </Modal>
  )
}
