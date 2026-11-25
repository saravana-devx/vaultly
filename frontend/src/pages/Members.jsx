import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Modal, Icon, ago, copy, useToast } from '../components/ui.jsx'

const ENV_LEVELS = [['inherit', 'Same as workspace role'], ['none', 'No access'], ['viewer', 'Viewer (read only)'], ['developer', 'Developer (read & write)']]

function EnvAccess({ slugs, value, onChange }) {
  if (!slugs.length) return <p className="muted" style={{ fontSize: 13 }}>Create a project to set per-environment access.</p>
  return (
    <div className="envgrid">
      {slugs.map((s) => (
        <label key={s}><span className="chip static">{s}</span>
          <select className="input" value={value[s] || 'inherit'} onChange={(e) => onChange({ ...value, [s]: e.target.value })}>{ENV_LEVELS.map(([k, l]) => <option key={k} value={k}>{l}</option>)}</select></label>
      ))}
    </div>
  )
}

export default function Members() {
  const { orgId, org, user, reloadOrgs } = useAuth()
  const [list, setList] = useState([])
  const [invites, setInvites] = useState([])
  const [slugs, setSlugs] = useState([])
  const [params, setParams] = useSearchParams()
  const [inviting, setInviting] = useState(params.get('invite') === '1')
  const [editing, setEditing] = useState(null)
  const [link, setLink] = useState(null)
  const [toast, toastEl] = useToast()
  const admin = org?.role === 'admin'
  const load = () => {
    api(`/orgs/${orgId}/members`).then(setList)
    api(`/orgs/${orgId}/env-slugs`).then(setSlugs)
    if (admin) api(`/orgs/${orgId}/invites`).then(setInvites)
  }
  useEffect(() => { load() }, [orgId, admin])
  const act = async (fn) => { try { await fn(); load() } catch (x) { alert(x.message) } }

  return (
    <>
      <div className="head"><div><h1>Members</h1><p>Admins manage everything. Developers read and write secrets. Viewers are read-only. Limit anyone to specific environments.</p></div>
        {admin && <button className="btn primary" onClick={() => setInviting(true)}><Icon name="plus" size={15} /> Invite member</button>}</div>
      <div className="scroll-x"><table className="table"><thead><tr><th>Name</th><th>Role</th><th>Environment access</th><th>Joined</th><th /></tr></thead><tbody>
        {list.map((m) => {
          const ov = Object.entries(m.env_roles || {})
          return (
            <tr key={m.id}>
              <td><div style={{ display: 'flex', gap: 12, alignItems: 'center' }}><Avatar name={m.name} size={32} /><div><div style={{ fontWeight: 500 }}>{m.name}{m.id === user.id && <span className="muted"> (you)</span>}</div><div className="muted" style={{ fontSize: 13 }}>{m.email}</div></div></div></td>
              <td>{admin ? (
                <select className="input" style={{ width: 130, height: 32 }} value={m.role} aria-label={`Role for ${m.name}`}
                  onChange={(e) => act(() => api(`/orgs/${orgId}/members/${m.id}`, { method: 'PATCH', body: { role: e.target.value } }))}>
                  <option value="admin">Admin</option><option value="developer">Developer</option><option value="viewer">Viewer</option></select>
              ) : <span className={`tag ${m.role}`}>{m.role}</span>}</td>
              <td>{m.role === 'admin' ? <span className="muted">All environments</span> : ov.length ? ov.map(([s, r]) => <span key={s} className={`chip static lvl-${r}`}>{s}: {r === 'none' ? 'no access' : r}</span>) : <span className="muted">Same as role</span>}
                {admin && m.role !== 'admin' && <button className="btn sm ghost" style={{ marginLeft: 6 }} onClick={() => setEditing(m)}>Edit</button>}</td>
              <td className="muted">{ago(m.joined_at)}</td>
              <td style={{ textAlign: 'right' }}>{(admin || m.id === user.id) && (
                <button className="btn sm ghost danger" onClick={() => confirm(m.id === user.id ? 'Leave this workspace?' : `Remove ${m.name}?`) &&
                  act(async () => { await api(`/orgs/${orgId}/members/${m.id}`, { method: 'DELETE' }); if (m.id === user.id) reloadOrgs() })}>
                  {m.id === user.id ? 'Leave' : 'Remove'}</button>)}</td>
            </tr>)
        })}
      </tbody></table></div>

      {admin && invites.length > 0 && (
        <section style={{ marginTop: 40 }}>
          <div className="sect"><h2>Pending invitations</h2></div>
          <table className="table"><thead><tr><th>Email</th><th>Role</th><th>Invited by</th><th>Expires</th><th /></tr></thead><tbody>
            {invites.map((i) => (
              <tr key={i.id}><td>{i.email}</td><td><span className={`tag ${i.role}`}>{i.role}</span></td><td className="muted" style={{ fontSize: 13 }}>{i.invited_by}</td>
                <td className="muted">{i.expired ? <span style={{ color: 'var(--danger)' }}>Expired</span> : new Date(i.expires_at).toLocaleDateString()}</td>
                <td style={{ textAlign: 'right' }}><button className="btn sm ghost danger" onClick={() => act(() => api(`/orgs/${orgId}/invites/${i.id}`, { method: 'DELETE' }))}>Revoke</button></td></tr>))}
          </tbody></table>
        </section>
      )}

      {inviting && <Invite orgId={orgId} slugs={slugs} onClose={() => { setInviting(false); params.delete('invite'); setParams(params, { replace: true }) }} onDone={(r) => { setInviting(false); setLink(r); load() }} />}
      {editing && <EditAccess orgId={orgId} member={editing} slugs={slugs} onClose={() => setEditing(null)} onDone={() => { setEditing(null); load(); toast('Access updated') }} />}
      {link && (
        <Modal title="Invitation sent" onClose={() => setLink(null)}>
          <p style={{ marginBottom: 12 }}>We emailed <b>{link.email}</b>. You can also share the link directly — it works once, for that email address, for 7 days.</p>
          <div className="tokenbox">{link.link}</div>
          <div className="modal-actions"><button className="btn" onClick={async () => toast((await copy(link.link)) ? 'Link copied' : 'Copy failed')}>Copy link</button><button className="btn primary" onClick={() => setLink(null)}>Done</button></div>
        </Modal>
      )}
      {toastEl}
    </>
  )
}

function Invite({ orgId, slugs, onClose, onDone }) {
  const [f, setF] = useState({ email: '', role: 'developer' })
  const [env, setEnv] = useState({})
  const [err, setErr] = useState('')
  return (
    <Modal title="Invite a member" onClose={onClose} wide>
      <form onSubmit={async (e) => { e.preventDefault(); try { onDone(await api(`/orgs/${orgId}/invites`, { method: 'POST', body: { ...f, env_roles: env } })) } catch (x) { setErr(x.message) } }}>
        {err && <div className="alert">{err}</div>}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 180px', gap: 12 }}>
          <div className="field"><label>Email</label><input className="input" type="email" required autoFocus value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} placeholder="teammate@company.com" /></div>
          <div className="field"><label>Role</label><select className="input" value={f.role} onChange={(e) => setF({ ...f, role: e.target.value })}><option value="admin">Admin</option><option value="developer">Developer</option><option value="viewer">Viewer</option></select></div>
        </div>
        {f.role !== 'admin' && <div className="field"><label>Environment access<span className="hint">Admins always have full access</span></label><EnvAccess slugs={slugs} value={env} onChange={setEnv} /></div>}
        <div className="modal-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary">Send invitation</button></div>
      </form>
    </Modal>
  )
}

function EditAccess({ orgId, member, slugs, onClose, onDone }) {
  const [env, setEnv] = useState(member.env_roles || {})
  const [err, setErr] = useState('')
  return (
    <Modal title={`Environment access · ${member.name}`} onClose={onClose} wide>
      {err && <div className="alert">{err}</div>}
      <p className="muted" style={{ marginBottom: 14, fontSize: 13.5 }}>Applies to every project with an environment of that name. “Same as workspace role” means {member.name.split(' ')[0]} keeps their {member.role} access there.</p>
      <EnvAccess slugs={slugs} value={env} onChange={setEnv} />
      <div className="modal-actions" style={{ marginTop: 14 }}><button className="btn" onClick={onClose}>Cancel</button>
        <button className="btn primary" onClick={async () => { try { await api(`/orgs/${orgId}/members/${member.id}/env-roles`, { method: 'PUT', body: { roles: env } }); onDone() } catch (x) { setErr(x.message) } }}>Save</button></div>
    </Modal>
  )
}
