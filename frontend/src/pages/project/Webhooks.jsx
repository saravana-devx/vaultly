import { useEffect, useState } from 'react'
import { api } from '../../api.js'
import { Icon, Menu, Modal, ago, copy } from '../../components/ui.jsx'

const EVENT_GROUPS = [
  ['*', 'Everything'], ['secret.*', 'Secret changes and reads'], ['secret.expired', 'Secret expired'], ['token.*', 'Service token events'],
  ['dynamic.*', 'Dynamic credentials'], ['integration.*', 'Integration syncs'], ['webhook.*', 'Webhook changes'],
]

export default function Webhooks({ project, toast }) {
  const pid = project.id
  const [list, setList] = useState(null)
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState(null)
  const [deliveries, setDeliveries] = useState(null)
  const load = () => api(`/projects/${pid}/webhooks`).then(setList)
  useEffect(() => { load() }, [pid])

  const test = async (w) => {
    const r = await api(`/projects/${pid}/webhooks/${w.id}/test`, { method: 'POST' })
    toast(r.error ? `Failed: ${r.error}` : `Delivered in ${r.duration_ms} ms (HTTP ${r.status})`); load()
  }
  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}>
        <div className="muted" style={{ flex: 1 }}>Get an HTTP POST when something changes. Payloads are signed and never include secret values.</div>
        <button className="btn primary" onClick={() => setOpen(true)}><Icon name="plus" size={15} /> Add webhook</button>
      </div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Name</th><th>Endpoint</th><th>Events</th><th>Status</th><th>Last delivery</th><th style={{ width: 90 }} /></tr></thead><tbody>
        {(list || []).map((w) => (
          <tr key={w.id} style={{ opacity: w.active ? 1 : 0.55 }}>
            <td style={{ fontWeight: 500 }}>{w.name}</td>
            <td className="mono muted" style={{ fontSize: 12.5, maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{w.url}</td>
            <td>{w.events.map((e) => <span className="chip static" key={e}>{e}</span>)}</td>
            <td>{w.active ? <span className="tag admin">Active</span> : <span className="tag">Paused</span>}</td>
            <td>{w.last_delivery_at ? <><span className={`dot-s ${w.last_status >= 200 && w.last_status < 300 ? 'ok' : 'error'}`} /> {w.last_status || 'Failed'} · {ago(w.last_delivery_at)}{w.last_error && <div className="cm err" title={w.last_error}>{w.last_error}</div>}</> : <span className="muted">No deliveries</span>}</td>
            <td style={{ textAlign: 'right' }}><Menu items={[
              { label: 'Send test event', icon: 'rotate', run: () => test(w) },
              { label: 'Recent deliveries', icon: 'history', run: () => setDeliveries(w) },
              { label: w.active ? 'Pause' : 'Resume', icon: w.active ? 'eyeoff' : 'eye', run: async () => { await api(`/projects/${pid}/webhooks/${w.id}`, { method: 'PATCH', body: { active: !w.active } }); load() } },
              { label: 'Delete', icon: 'trash', danger: true, run: async () => { if (confirm(`Delete ${w.name}?`)) { await api(`/projects/${pid}/webhooks/${w.id}`, { method: 'DELETE' }); load() } } },
            ]} /></td>
          </tr>))}
        {list && !list.length && <tr><td colSpan="6" className="muted" style={{ textAlign: 'center', padding: 44 }}>No webhooks yet.</td></tr>}
      </tbody></table></div>
      <p className="muted" style={{ fontSize: 13, marginTop: 14 }}>Verify with <span className="mono">X-Vaultly-Signature</span> = <span className="mono">v1=HMAC_SHA256(secret, timestamp + "." + body)</span>. Failed deliveries are retried twice.</p>
      {open && <AddWebhook pid={pid} onClose={() => setOpen(false)} onDone={(secret) => { setOpen(false); setCreated(secret); load() }} />}
      {created && (
        <Modal title="Webhook created" onClose={() => setCreated(null)}>
          <div className="alert warn">Copy the signing secret now — it won’t be shown again.</div>
          <div className="tokenbox">{created}</div>
          <div className="modal-actions"><button className="btn" onClick={async () => toast((await copy(created)) ? 'Copied' : 'Copy failed')}>Copy</button><button className="btn primary" onClick={() => setCreated(null)}>Done</button></div>
        </Modal>
      )}
      {deliveries && <Deliveries pid={pid} hook={deliveries} onClose={() => setDeliveries(null)} />}
    </>
  )
}

function AddWebhook({ pid, onClose, onDone }) {
  const [f, setF] = useState({ name: '', url: '' })
  const [events, setEvents] = useState(['*'])
  const [err, setErr] = useState('')
  const toggle = (e) => setEvents((cur) => (e === '*' ? ['*'] : cur.includes(e) ? cur.filter((x) => x !== e) : [...cur.filter((x) => x !== '*'), e]))
  return (
    <Modal title="Add webhook" onClose={onClose}>
      <form onSubmit={async (e) => { e.preventDefault(); try { const r = await api(`/projects/${pid}/webhooks`, { method: 'POST', body: { ...f, events: events.length ? events : ['*'] } }); onDone(r.secret) } catch (x) { setErr(x.message) } }}>
        {err && <div className="alert">{err}</div>}
        <div className="field"><label>Name</label><input className="input" required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Slack relay" /></div>
        <div className="field"><label>Endpoint URL</label><input className="input mono" required value={f.url} onChange={(e) => setF({ ...f, url: e.target.value })} placeholder="https://example.com/hooks/vaultly" /></div>
        <div className="field"><label>Events</label>
          <div className="checks">{EVENT_GROUPS.map(([e, label]) => <label className="check" key={e}><input type="checkbox" checked={events.includes(e)} onChange={() => toggle(e)} /> <span><span className="mono" style={{ fontSize: 12.5 }}>{e}</span> <span className="muted">{label}</span></span></label>)}</div></div>
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Create webhook</button></div>
      </form>
    </Modal>
  )
}

function Deliveries({ pid, hook, onClose }) {
  const [rows, setRows] = useState(null)
  useEffect(() => { api(`/projects/${pid}/webhooks/${hook.id}/deliveries`).then(setRows) }, [])
  return (
    <Modal title={`Deliveries · ${hook.name}`} onClose={onClose} wide>
      <table className="table"><thead><tr><th>Event</th><th>Result</th><th>Tries</th><th>Time</th><th>When</th></tr></thead><tbody>
        {(rows || []).map((d, i) => (
          <tr key={i}><td className="mono" style={{ fontSize: 12.5 }}>{d.action}</td>
            <td><span className={`dot-s ${d.error ? 'error' : 'ok'}`} /> {d.status || 'No response'}{d.error && <div className="cm err">{d.error}</div>}</td>
            <td className="muted">{d.attempts}</td><td className="muted">{d.duration_ms} ms</td><td className="muted">{ago(d.at)}</td></tr>))}
        {rows && !rows.length && <tr><td colSpan="5" className="muted" style={{ textAlign: 'center', padding: 32 }}>Nothing sent yet.</td></tr>}
      </tbody></table>
      <div className="modal-actions"><button className="btn" onClick={onClose}>Close</button></div>
    </Modal>
  )
}
