import { useEffect, useState } from 'react'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Icon, shortName } from '../components/ui.jsx'
import { describeEvent } from '../lib/events.js'
import { EmptyRow, Rel, StateDot, TableFoot, plural } from '../components/table.jsx'

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
  const shown = events.filter((e) => (!kind || e.action.startsWith(kind + '.')) && (e.actor + e.resource).toLowerCase().includes(q.toLowerCase()))
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
      <div className="scroll-x"><table className="table"><thead><tr><th style={{ width: 130 }}>When</th><th style={{ width: 210 }}>Actor</th><th>Event</th><th style={{ width: 170 }}>Project</th><th style={{ width: 140 }}>IP address</th></tr></thead><tbody>
        {shown.map((e) => {
          const ev = describeEvent(e.action)
          return (
            <tr key={e.id}>
              <td className="dim" style={{ whiteSpace: 'nowrap' }}><Rel iso={e.at} /></td>
              <td><div className="who"><Avatar name={e.actor.replace('token:', '') || '?'} size={24} /><span className="c-main light">{shortName(e.actor)}</span></div></td>
              <td>
                <StateDot tone={ev.tone}>{ev.label}</StateDot>
                {(e.resource || e.metadata?.env) && <div className="c-sub mono">{e.resource}{e.metadata?.env && <span> · {e.metadata.env}</span>}</div>}
              </td>
              <td className="dim">{e.project || '—'}</td>
              <td className="mono dim" style={{ fontSize: 12.5 }}>{e.ip}</td>
            </tr>)
        })}
        {!shown.length && <EmptyRow cols={5} icon="log" title={events.length ? 'No events match your filters' : 'No events yet'} hint={events.length ? 'Try a different search or event type.' : 'Activity in this workspace will appear here.'} />}
      </tbody></table>
      <TableFoot right={next && <button className="btn sm" onClick={() => load(next)}>Load more</button>}>{plural(shown.length, 'event')}{shown.length !== events.length && ` of ${events.length} loaded`}</TableFoot></div>
    </>
  )
}
