import { useEffect, useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Icon, Menu, hue } from './ui.jsx'
import CommandPalette from './CommandPalette.jsx'

const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform)

export default function Shell() {
  const { user, orgs, orgId, org, switchOrg, logout } = useAuth()
  const nav = useNavigate()
  const [projects, setProjects] = useState([])
  const [palette, setPalette] = useState(false)
  const admin = org?.role === 'admin'

  useEffect(() => {
    const load = () => orgId && api(`/orgs/${orgId}/projects`).then(setProjects).catch(() => {})
    load()
    window.addEventListener('vaultly:projects', load)
    return () => window.removeEventListener('vaultly:projects', load)
  }, [orgId])

  useEffect(() => {
    const onKey = (e) => { if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette((o) => !o) } }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const link = ({ isActive }) => `navlink${isActive ? ' active' : ''}`
  return (
    <div className="shell">
      <aside className="side">
        <div className="ws">
          <span className="sq">{org?.name?.[0]?.toUpperCase()}</span>
          <select value={orgId || ''} aria-label="Workspace" onChange={(e) => { switchOrg(e.target.value); nav('/app') }}>
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </select>
          <Icon name="down" size={14} />
        </div>
        <NavLink end to="/app" className={link}><Icon name="home" /> Overview</NavLink>
        <NavLink end to="/app/projects" className={link}><Icon name="layers" /> Projects{projects.length > 0 && <span className="navcount">{projects.length}</span>}</NavLink>
        <NavLink to="/app/members" className={link}><Icon name="users" /> Members</NavLink>
        {admin && <NavLink to="/app/audit" className={link}><Icon name="log" /> Audit log</NavLink>}
        <NavLink to="/app/settings" className={link}><Icon name="cog" /> Settings</NavLink>

        {projects.length > 0 && <div className="nav-h"><span>Projects</span></div>}
        <div className="side-projects">
          {projects.map((p) => (
            <NavLink key={p.id} to={`/app/projects/${p.id}`} className={link}>
              <i style={{ width: 8, height: 8, borderRadius: 3, background: `hsl(${hue(p.name)} 60% 58%)`, flex: 'none', margin: '0 5px 0 4px' }} />
              <span className="t">{p.name}</span>
            </NavLink>
          ))}
        </div>

        <div className="side-foot">
          <Avatar name={user.name} size={28} />
          <div className="who"><b>{user.name}</b><span>{org?.role}</span></div>
          <button className="iconbtn" title="Sign out" aria-label="Sign out" onClick={async () => { await logout(); nav('/') }}><Icon name="out" size={16} /></button>
        </div>
      </aside>

      <div className="colmain">
        <header className="topbar">
          <button className="searchbtn" onClick={() => setPalette(true)}><Icon name="search" size={15} /><span>Search projects, pages, actions…</span><kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd></button>
          <div style={{ flex: 1 }} />
          {admin && (
            <Menu align="right" trigger={<span className="btn primary sm"><Icon name="plus" size={14} /> New</span>} items={[
              { label: 'Project', icon: 'layers', run: () => nav('/app/projects?new=1') },
              { label: 'Invite member', icon: 'users', run: () => nav('/app/members?invite=1') },
            ]} />
          )}
        </header>
        <main className="main">{orgId ? <Outlet key={orgId} /> : <div className="spinner" style={{ margin: 40 }} />}</main>
      </div>
      <CommandPalette open={palette} onClose={() => setPalette(false)} projects={projects} admin={admin} />
    </div>
  )
}
