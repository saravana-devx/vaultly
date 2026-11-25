import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Modal, Icon, ago } from '../components/ui.jsx'
import { EnvTag } from '../components/table.jsx'

export default function Projects() {
  const { orgId, org } = useAuth()
  const [params, setParams] = useSearchParams()
  const [list, setList] = useState(null)
  const [open, setOpen] = useState(params.get('new') === '1')
  const [name, setName] = useState('')
  const [q, setQ] = useState('')
  const [err, setErr] = useState('')
  const load = () => api(`/orgs/${orgId}/projects`).then(setList)
  useEffect(() => { load() }, [orgId])
  const close = () => { setOpen(false); params.delete('new'); setParams(params, { replace: true }) }
  const create = async (e) => {
    e.preventDefault(); setErr('')
    try { await api(`/orgs/${orgId}/projects`, { method: 'POST', body: { name } }); close(); setName(''); load(); window.dispatchEvent(new Event('vaultly:projects')) } catch (x) { setErr(x.message) }
  }
  const shown = (list || []).filter((p) => p.name.toLowerCase().includes(q.toLowerCase()))
  return (
    <>
      <div className="head"><div><h1>Projects</h1><p>Each project has its own encryption key and environments.</p></div>
        <div style={{ display: 'flex', gap: 8 }}>
          <div className="search"><Icon name="search" size={15} /><input className="input" placeholder="Search projects" value={q} onChange={(e) => setQ(e.target.value)} /></div>
          {org?.role === 'admin' && <button className="btn primary" style={{ height: 34 }} onClick={() => setOpen(true)}><Icon name="plus" size={15} /> New project</button>}</div></div>
      {list === null ? <div className="spinner" /> : shown.length === 0 ? (
        <div className="empty"><h3>{list.length ? 'No matches' : 'No projects yet'}</h3><p>{list.length ? 'Try a different search.' : org?.role === 'admin' ? 'Create your first project to start storing secrets.' : 'Ask an admin to create one.'}</p></div>
      ) : (
        <div className="ptable">
          <div className="hd"><span>Name</span><span>Environments</span><span>Secrets</span><span>Last activity</span><span /></div>
          {shown.map((p) => (
            <Link to={`/app/projects/${p.id}`} key={p.id}>
              <span className="nm"><Avatar name={p.name} square size={28} /><span>{p.name}</span></span>
              <span className="chips">{p.environments.slice(0, 3).map((e) => <EnvTag key={e.id} slug={e.slug} />)}{p.environments.length > 3 && <i>+{p.environments.length - 3}</i>}</span>
              <span className="num">{p.secret_count}</span>
              <span className="muted">{p.last_activity ? ago(p.last_activity) : ago(p.created_at)}</span>
              <Icon name="right" size={15} />
            </Link>))}
        </div>
      )}
      {open && (
        <Modal title="New project" onClose={close}>
          <form onSubmit={create}>
            {err && <div className="alert">{err}</div>}
            <div className="field"><label>Project name</label><input className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="payments-api" required /></div>
            <p className="muted" style={{ fontSize: 13, marginBottom: 18 }}>Development, Staging and Production are created for you.</p>
            <div className="modal-actions"><button type="button" className="btn" onClick={close}>Cancel</button><button className="btn primary">Create project</button></div>
          </form>
        </Modal>
      )}
    </>
  )
}
