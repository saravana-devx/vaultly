import type { Row } from '../types'
import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { useAuth } from '../auth'
import { Avatar, Icon, shortName } from '../components/ui'
import { describeEvent } from '../lib/events'
import { EmptyRow, Rel, StateDot, TableFoot, plural } from '../components/table'

const KINDS: [string, string][] = [
  ['', 'All events'], ['secret', 'Secrets'], ['token', 'Service tokens'], ['auth', 'Sign-ins'], ['member', 'Members'],
  ['project', 'Projects'], ['environment', 'Environments'], ['webhook', 'Webhooks'], ['integration', 'Integrations'],
  ['dynamic', 'Dynamic secrets'], ['org', 'Backups'],
]

export default function Audit() {
  const { orgId } = useAuth()
  const [events, setEvents] = useState<Row[]>([])
  const [next, setNext] = useState('')
  const [err, setErr] = useState('')
  const [kind, setKind] = useState('')
  const [q, setQ] = useState('')
  const [loading, setLoading] = useState(true)
  // The type filter runs on the server so it covers the whole log. Actor, resource and IP are
  // stored encrypted, so the search box can only look through the events loaded so far.
  // Only the newest request may write state: switching the type (or workspace) while a page is
  // still loading must not let the older response overwrite the newer list.
  const seq = useRef(0)
  const load = async (before = '', type = kind) => {
    const mine = ++seq.current
    setLoading(true)
    try {
      const params = new URLSearchParams()
      if (before) params.set('before', before)
      if (type) params.set('kind', type)
      const d = await api(`/orgs/${orgId}/audit${params.size ? `?${params}` : ''}`)
      if (mine !== seq.current) return
      setEvents((cur) => {
        if (!before) return d.events
        const seen = new Set(cur.map((e) => e.id))
        return [...cur, ...d.events.filter((e: Row) => !seen.has(e.id))]
      })
      setNext(d.next || ''); setErr('')
    } catch (x: any) { if (mine === seq.current) setErr(x.message) } finally { if (mine === seq.current) setLoading(false) }
  }
  useEffect(() => { load('', kind) }, [orgId, kind])
  const needle = q.trim().toLowerCase()
  const shown = events.filter((e) => !needle || `${e.actor} ${e.resource} ${e.ip} ${e.project ?? ''}`.toLowerCase().includes(needle))
  return (
    <>
      <div className="head"><div><h1>Audit log</h1><p>Who did what in this workspace, newest first.</p></div>
        <button className="btn" disabled={loading} onClick={() => load()}><Icon name="rotate" size={15} /> Refresh</button></div>
      <div className="filters">
        <div className="search"><Icon name="search" size={15} /><input className="input" placeholder="Search actor, resource, project or IP" value={q} onChange={(e) => setQ(e.target.value)} /></div>
        <select className="input" value={kind} onChange={(e) => { setEvents([]); setNext(''); setKind(e.target.value) }}>
          {KINDS.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>
      </div>
      {err && <div className="alert">{err}</div>}
      <div className="scroll-x"><table className="table audit"><colgroup><col style={{ width: 120 }} /><col style={{ width: 230 }} /><col /><col style={{ width: 170 }} /><col style={{ width: 140 }} /></colgroup><thead><tr><th>When</th><th>Actor</th><th>Event</th><th>Project</th><th>IP address</th></tr></thead><tbody>
        {shown.map((e) => {
          const ev = describeEvent(e.action)
          return (
            <tr key={e.id}>
              <td className="dim" style={{ whiteSpace: 'nowrap' }}><Rel iso={e.at} /></td>
              <td><div className="who"><Avatar name={e.actor.replace('token:', '') || '?'} size={24} /><span className="c-main light" title={e.actor}>{shortName(e.actor)}</span></div></td>
              <td>
                <StateDot tone={ev.tone}>{ev.label}</StateDot>
                {(e.resource || e.metadata?.env) && <div className="c-sub mono" title={e.resource}>{e.resource}{e.metadata?.env && <span> · {e.metadata.env}</span>}</div>}
              </td>
              <td className="dim cut" title={e.project || e.project_id || undefined}>{e.project || (e.project_id ? <i>Deleted project</i> : 'Workspace')}</td>
              <td className="mono dim" style={{ fontSize: 12.5 }}>{e.ip}</td>
            </tr>)
        })}
        {!shown.length && loading && <EmptyRow cols={5} icon="log" title="Loading events…" />}
        {!shown.length && !loading && <EmptyRow cols={5} icon="log" title={events.length ? 'No loaded events match your search' : kind ? 'No events of this type' : 'No events yet'} hint={events.length ? (next ? 'Load more events to search further back.' : 'Try a different search.') : 'Activity in this workspace will appear here.'} />}
      </tbody></table>
      <TableFoot right={next && <button className="btn sm" disabled={loading} onClick={() => load(next)}>{loading ? 'Loading…' : 'Load more'}</button>}>{plural(shown.length, 'event')}{shown.length !== events.length && ` of ${events.length} loaded`}{next && !needle && ' · more available'}</TableFoot></div>
    </>
  )
}
