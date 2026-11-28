import type { Row } from '../types'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { useAuth } from '../auth'
import { Avatar, Expiry, Modal, Icon, copy } from '../components/ui'
import { ask, toast } from '../components/feedback'
import { EmptyRow, EnvTag, Rel, RowMenu, TableFoot, cap, plural } from '../components/table'

const ENV_LEVELS = [['inherit', 'Same as workspace role'], ['none', 'No access'], ['viewer', 'Viewer (read only)'], ['developer', 'Developer (read & write)']]

function EnvAccess({ slugs, value, onChange }: { slugs: string[]; value: any; onChange: (...args: any[]) => void }) {
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
  const [list, setList] = useState<Row[]>([])
  const [invites, setInvites] = useState<Row[]>([])
  const [slugs, setSlugs] = useState<string[]>([])
  const [params, setParams] = useSearchParams()
  const [inviting, setInviting] = useState(params.get('invite') === '1')
  const [editing, setEditing] = useState<Row | null>(null)
  const [link, setLink] = useState<Row | null>(null)
  const [q, setQ] = useState('')
  const admin = org?.role === 'admin'
  const load = () => {
    api(`/orgs/${orgId}/members`).then(setList)
    api(`/orgs/${orgId}/env-slugs`).then(setSlugs)
    if (admin) api(`/orgs/${orgId}/invites`).then(setInvites)
  }
  useEffect(() => { load() }, [orgId, admin])
  const act = async (fn: () => any) => { try { await fn(); load() } catch (x) { toast.error(x.message) } }

  return (
    <>
      <div className="head"><div><h1>Members</h1><p>Admins manage everything. Developers read and write secrets. Viewers are read-only. Limit anyone to specific environments.</p></div>
        {admin && <button className="btn primary" onClick={() => setInviting(true)}><Icon name="plus" size={15} /> Invite member</button>}</div>
      {list.length > 5 && <div className="filters"><div className="search"><Icon name="search" size={15} /><input className="input" placeholder="Search members" value={q} onChange={(e) => setQ(e.target.value)} /></div></div>}
      <div className="scroll-x"><table className="table"><thead><tr><th>Member</th><th style={{ width: 170 }}>Role</th><th>Environment access</th><th style={{ width: 130 }}>Joined</th><th style={{ width: 56 }} /></tr></thead><tbody>
        {list.filter((m) => !q.trim() || `${m.name} ${m.email} ${m.role}`.toLowerCase().includes(q.trim().toLowerCase())).map((m) => {
          const overrides = Object.entries<string>(m.env_roles || {})
          const self = m.id === user?.id
          return (
            <tr key={m.id}>
              <td>
                <div className="who"><Avatar name={m.name} size={34} />
                  <div><div className="c-main">{m.name}{self && <span className="you">You</span>}</div><div className="c-sub">{m.email}</div></div></div>
              </td>
              <td>{admin ? (
                <select className="rolesel" value={m.role} aria-label={`Role for ${m.name}`}
                  onChange={(e) => act(() => api(`/orgs/${orgId}/members/${m.id}`, { method: 'PATCH', body: { role: e.target.value } }))}>
                  <option value="admin">Admin</option><option value="developer">Developer</option><option value="viewer">Viewer</option></select>
              ) : <span className="role">{m.role}</span>}</td>
              <td>{m.role === 'admin' ? <span className="dim">All environments</span> : overrides.length ? (
                <div className="envlist">{overrides.map(([slug, level]) => <span key={slug} className="envlvl"><EnvTag slug={slug} /><em className={level}>{level === 'none' ? 'No access' : cap(level)}</em></span>)}</div>
              ) : <span className="dim">Same as role</span>}</td>
              <td className="dim"><Rel iso={m.joined_at} /></td>
              <td>
                <RowMenu items={[
                  admin && m.role !== 'admin' && { label: 'Edit environment access', icon: 'pencil', run: () => setEditing(m) },
                  (admin || self) && { label: self ? 'Leave workspace' : 'Remove from workspace', icon: 'out', danger: true, run: async () => {
                    const ok = await ask(self ? 'Leave this workspace? You will lose access to its projects.' : `Remove ${m.name} from this workspace?`, { confirm: self ? 'Leave' : 'Remove', danger: true })
                    if (ok) act(async () => { await api(`/orgs/${orgId}/members/${m.id}`, { method: 'DELETE' }); if (self) reloadOrgs() })
                  } },
                ]} />
              </td>
            </tr>)
        })}
      </tbody></table><TableFoot>{plural(list.length, 'member')}</TableFoot></div>

      {admin && invites.length > 0 && (
        <section style={{ marginTop: 40 }}>
          <div className="sect"><h2>Pending invitations</h2></div>
          <div className="scroll-x"><table className="table"><thead><tr><th>Email</th><th style={{ width: 150 }}>Role</th><th>Invited by</th><th style={{ width: 150 }}>Expires</th><th style={{ width: 56 }} /></tr></thead><tbody>
            {invites.map((i) => (
              <tr key={i.id}>
                <td className="c-main">{i.email}</td>
                <td className="role">{i.role}</td>
                <td className="dim">{i.invited_by}</td>
                <td>{i.expired ? <span className="state bad"><i />Expired</span> : <Expiry iso={i.expires_at} />}</td>
                <td><RowMenu items={[{ label: 'Revoke invitation', icon: 'trash', danger: true, run: () => act(() => api(`/orgs/${orgId}/invites/${i.id}`, { method: 'DELETE' })) }]} /></td>
              </tr>))}
          </tbody></table><TableFoot>{plural(invites.length, 'pending invitation')}</TableFoot></div>
        </section>
      )}

      {inviting && <Invite orgId={orgId!} slugs={slugs} onClose={() => { setInviting(false); params.delete('invite'); setParams(params, { replace: true }) }} onDone={(r) => { setInviting(false); setLink(r); load() }} />}
      {editing && <EditAccess orgId={orgId!} member={editing} slugs={slugs} onClose={() => setEditing(null)} onDone={() => { setEditing(null); load() }} />}
      {link && (
        <Modal title="Invitation sent" onClose={() => setLink(null)}>
          <p style={{ marginBottom: 12 }}>We emailed <b>{link.email}</b>. You can also share the link directly — it works once, for that email address, for 7 days.</p>
          <div className="tokenbox">{link.link}</div>
          <div className="modal-actions"><button className="btn" onClick={async () => copy(link.link, 'Link copied')}>Copy link</button><button className="btn primary" onClick={() => setLink(null)}>Done</button></div>
        </Modal>
      )}
    </>
  )
}

function Invite({ orgId, slugs, onClose, onDone }: { orgId: string; slugs: string[]; onClose: (...args: any[]) => void; onDone: (...args: any[]) => void }) {
  const [f, setF] = useState<Row>({ email: '', role: 'developer' })
  const [env, setEnv] = useState<Record<string, string>>({})
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

function EditAccess({ orgId, member, slugs, onClose, onDone }: { orgId: string; member: Row; slugs: string[]; onClose: (...args: any[]) => void; onDone: (...args: any[]) => void }) {
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
