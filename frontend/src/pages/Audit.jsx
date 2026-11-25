import { useEffect, useState } from 'react'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Icon, ago, shortName } from '../components/ui.jsx'

export default function Audit() {
  const { orgId } = useAuth()
  const [events, setEvents] = useState([])
  const [next, setNext] = useState('')
  const [err, setErr] = useState('')
  const [kind, setKind] = useState('')
  const [q, setQ] = useState('')
  const load = async (before = '') => {
    try {
      const d = await api(`/orgs/${orgId}/audit${before ? `?before=${encodeURIComponent(before)}` : ''}`)
      setEvents((e) => (before ? [...e, ...d.events] : d.events)); setNext(d.next); setErr('')
    } catch (x) { setErr(x.message) }
  }
  useEffect(() => { load() }, [orgId])
  return (
    <>
      <div className="head"><div><h1>Audit log</h1><p>Events are published to Kafka and written by a consumer worker.</p></div>
        <button className="btn" onClick={() => load()}><Icon name="rotate" size={15} /> Refresh</button></div>
      <div className="filters">
        <div className="search"><Icon name="search" size={15} /><input className="input" placeholder="Search actor or resource" value={q} onChange={(e) => setQ(e.target.value)} /></div>
        <select className="input" value={kind} onChange={(e) => setKind(e.target.value)}>
          <option value="">All events</option><option value="secret">Secrets</option><option value="token">Tokens</option><option value="auth">Sign-ins</option><option value="member">Members</option><option value="project">Projects</option></select>
      </div>
      {err && <div className="alert">{err}</div>}
      <div className="scroll-x"><table className="table"><thead><tr><th>Time</th><th>Actor</th><th>Event</th><th>Resource</th><th>Project</th><th>IP</th></tr></thead><tbody>
        {events.filter((e) => (!kind || e.action.startsWith(kind + '.')) && (e.actor + e.resource).toLowerCase().includes(q.toLowerCase())).map((e) => (
          <tr key={e.id}>
            <td className="muted" title={new Date(e.at).toLocaleString()} style={{ whiteSpace: 'nowrap' }}>{ago(e.at)}</td>
            <td><div className="who"><Avatar name={e.actor.replace('token:', '')} size={22} />{shortName(e.actor)}</div></td>
            <td><span className={`badge ${e.action.split('.')[1]}`}>{e.action}</span></td>
            <td className="mono" style={{ fontSize: 12.5 }}>{e.resource}{e.metadata?.env && <span className="muted"> · {e.metadata.env}</span>}</td>
            <td className="muted">{e.project || '—'}</td><td className="muted mono" style={{ fontSize: 12 }}>{e.ip}</td>
          </tr>))}
        {events.length === 0 && <tr><td colSpan="6" className="muted" style={{ textAlign: 'center', padding: 40 }}>No events yet.</td></tr>}
      </tbody></table></div>
      {next && <div style={{ textAlign: 'center', marginTop: 20 }}><button className="btn" onClick={() => load(next)}>Load more</button></div>}
    </>
  )
}
