import type { FormEvent } from 'react'
import type { Row } from '../../types'
import { useEffect, useState } from 'react'
import { api } from '../../api'
import { Icon, Menu, Modal, copy } from '../../components/ui'
import { ask, toast } from '../../components/feedback'
import { EmptyRow, Rel, RowMenu, StateDot, TableFoot, plural } from '../../components/table'

const EVENT_GROUPS = [
  ['*', 'Everything'], ['secret.*', 'Secret changes and reads'], ['secret.expired', 'Secret expired'], ['token.*', 'Service token events'],
  ['change.*', 'Change requests'], ['dynamic.*', 'Dynamic credentials'], ['integration.*', 'Integration syncs'], ['webhook.*', 'Webhook changes'],
]

export default function Webhooks({ project }: { project: Row }) {
  const pid = project.id
  const [list, setList] = useState<Row | null>(null)
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState<string | null>(null)
  const [deliveries, setDeliveries] = useState<Row | null>(null)
  const load = () => api(`/projects/${pid}/webhooks`).then(setList)
  useEffect(() => { load() }, [pid])

  const test = async (w: Row) => {
    const r = await api(`/projects/${pid}/webhooks/${w.id}/test`, { method: 'POST', quiet: true })
    toast(r.error ? `Failed: ${r.error}` : `Delivered in ${r.duration_ms} ms (HTTP ${r.status})`); load()
  }
  return (
    <>
      <div className="bar" style={{ paddingTop: 18 }}>
        <div className="muted" style={{ flex: 1 }}>Get an HTTP POST when something changes. Payloads are signed and never include secret values.</div>
        <button className="btn primary" onClick={() => setOpen(true)}><Icon name="plus" size={15} /> Add webhook</button>
      </div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Webhook</th><th>Events</th><th>Status</th><th>Last delivery</th><th style={{ width: 56 }} /></tr></thead><tbody>
        {(list || []).map((w: Row) => (
          <tr key={w.id}>
            <td style={{ opacity: w.active ? 1 : 0.6 }}><div className="c-main">{w.name}</div><div className="c-sub mono clip">{w.url}</div></td>
            <td className="mono dim" style={{ fontSize: 12 }}>{w.events.join(', ')}</td>
            <td><StateDot tone={w.active ? 'ok' : 'idle'}>{w.active ? 'Active' : 'Paused'}</StateDot></td>
            <td>{w.last_delivery_at ? (
              <><StateDot tone={w.last_status >= 200 && w.last_status < 300 ? 'ok' : 'bad'}>{w.last_status || 'Failed'}</StateDot>
                <div className="c-sub"><Rel iso={w.last_delivery_at} />{w.last_error && <span className="err" title={w.last_error}> · {w.last_error}</span>}</div></>
            ) : <span className="dim">No deliveries yet</span>}</td>
            <td><RowMenu items={[
              { label: 'Send test event', icon: 'rotate', run: () => test(w) },
              { label: 'Recent deliveries', icon: 'history', run: () => setDeliveries(w) },
              { label: w.active ? 'Pause' : 'Resume', icon: w.active ? 'eyeoff' : 'eye', run: async () => { await api(`/projects/${pid}/webhooks/${w.id}`, { method: 'PATCH', body: { active: !w.active } }); load() } },
              { label: 'Delete', icon: 'trash', danger: true, run: async () => { if (await ask(`Delete ${w.name}?`, { confirm: 'Delete', danger: true })) { await api(`/projects/${pid}/webhooks/${w.id}`, { method: 'DELETE' }); load() } } },
            ]} /></td>
          </tr>))}
        {list && !list.length && <EmptyRow cols={5} icon="log" title="No webhooks yet" hint="Add an endpoint to be notified when something changes." />}
      </tbody></table><TableFoot>{plural((list || []).length, 'webhook')}</TableFoot></div>
      <p className="muted" style={{ fontSize: 13, marginTop: 14 }}>Verify with <span className="mono">X-Vaultly-Signature</span> = <span className="mono">v1=HMAC_SHA256(secret, timestamp + "." + body)</span>. Failed deliveries are retried twice.</p>
      {open && <AddWebhook pid={pid} onClose={() => setOpen(false)} onDone={(secret) => { setOpen(false); setCreated(secret); load() }} />}
      {created && (
        <Modal title="Webhook created" onClose={() => setCreated(null)}>
          <div className="alert warn">Copy the signing secret now — it won’t be shown again.</div>
          <div className="tokenbox">{created}</div>
          <div className="modal-actions"><button className="btn" onClick={async () => copy(created, 'Copied')}>Copy</button><button className="btn primary" onClick={() => setCreated(null)}>Done</button></div>
        </Modal>
      )}
      {deliveries && <Deliveries pid={pid} hook={deliveries} onClose={() => setDeliveries(null)} />}
    </>
  )
}

function AddWebhook({ pid, onClose, onDone }: { pid: string; onClose: (...args: any[]) => void; onDone: (...args: any[]) => void }) {
  const [f, setF] = useState<Row>({ name: '', url: '' })
  const [events, setEvents] = useState<string[]>(['*'])
  const [err, setErr] = useState('')
  const toggle = (e: any) => setEvents((cur) => (e === '*' ? ['*'] : cur.includes(e) ? cur.filter((x) => x !== e) : [...cur.filter((x) => x !== '*'), e]))
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

function Deliveries({ pid, hook, onClose }: { pid: string; hook: Row; onClose: (...args: any[]) => void }) {
  const [rows, setRows] = useState<Row | null>(null)
  useEffect(() => { api(`/projects/${pid}/webhooks/${hook.id}/deliveries`).then(setRows) }, [])
  return (
    <Modal title={`Deliveries · ${hook.name}`} onClose={onClose} wide>
      <table className="table"><thead><tr><th>Event</th><th>Result</th><th>Tries</th><th>Time</th><th>When</th></tr></thead><tbody>
        {(rows || []).map((d: Row, i: number) => (
          <tr key={i}><td className="mono" style={{ fontSize: 12.5 }}>{d.action}</td>
            <td><StateDot tone={d.error ? 'bad' : 'ok'}>{d.status || 'No response'}</StateDot>{d.error && <div className="c-sub err">{d.error}</div>}</td>
            <td className="dim">{d.attempts}</td><td className="dim">{d.duration_ms} ms</td><td className="dim"><Rel iso={d.at} /></td></tr>))}
        {rows && !rows.length && <tr><td colSpan={5} className="muted" style={{ textAlign: 'center', padding: 32 }}>Nothing sent yet.</td></tr>}
      </tbody></table>
      <div className="modal-actions"><button className="btn" onClick={onClose}>Close</button></div>
    </Modal>
  )
}
