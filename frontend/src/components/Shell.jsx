import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Avatar, Icon, Logo, Menu, ThemeToggle, currentTheme, flipTheme, hue } from './ui.jsx'
import CommandPalette from './CommandPalette.jsx'

const isMac = typeof navigator !== 'undefined' && /Mac/i.test(navigator.platform)

export default function Shell() {
  const { user, orgs, orgId, org, switchOrg, logout } = useAuth()
  const nav = useNavigate()
  const { pathname } = useLocation()
  const scroller = useRef(null)
  // every page opens at the top, like a fresh page load
  useEffect(() => { scroller.current?.scrollTo(0, 0) }, [pathname])
  const [projects, setProjects] = useState([])
  const [palette, setPalette] = useState(false)
  const [theme, setTheme] = useState(currentTheme)
  const [collapsed, setCollapsed] = useState(() => { try { return localStorage.getItem('vaultly-sidebar') === 'collapsed' } catch { return false } })
  const toggleSidebar = () => setCollapsed((c) => {
    try { localStorage.setItem('vaultly-sidebar', c ? 'open' : 'collapsed') } catch { /* remembered only when storage works */ }
    return !c
  })
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
    <div className={`shell${collapsed ? ' collapsed' : ''}`}>
      <aside className="side">
        <div className="side-top">
          <Link to="/app" className="side-brand" aria-label="Vaultly overview"><Logo size={26} /></Link>
          <button className="iconbtn side-collapse" onClick={toggleSidebar} title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}><Icon name="panel" size={16} /></button>
        </div>

        <Menu block left trigger={
          <span className="ws-card">
            <span className="sq">{org?.name?.[0]?.toUpperCase()}</span>
            <span className="ws-t"><b>{org?.name}</b><small>{org?.role}</small></span>
            <Icon name="updown" size={14} />
          </span>
        } items={orgs.map((o) => ({ label: o.name, icon: 'layers', active: o.id === orgId, run: () => { switchOrg(o.id); nav('/app') } }))} />

        <div className="nav-h first"><span>General</span></div>
        <NavLink end to="/app" className={link} title="Overview"><Icon name="home" /><span className="nl">Overview</span></NavLink>
        <NavLink end to="/app/projects" className={link} title="Projects"><Icon name="layers" /><span className="nl">Projects</span>{projects.length > 0 && <span className="navcount">{projects.length}</span>}</NavLink>
        <NavLink to="/app/members" className={link} title="Members"><Icon name="users" /><span className="nl">Members</span></NavLink>
        {admin && <NavLink to="/app/audit" className={link} title="Audit log"><Icon name="log" /><span className="nl">Audit log</span></NavLink>}

        {projects.length > 0 && (
          <div className="nav-h"><span>Projects</span>{admin && <Link to="/app/projects?new=1" className="nav-plus" title="New project" aria-label="New project"><Icon name="plus" size={14} /></Link>}</div>
        )}
        <div className="side-projects">
          {projects.map((p) => (
            <NavLink key={p.id} to={`/app/projects/${p.id}`} className={link}>
              <i style={{ width: 8, height: 8, borderRadius: 3, background: `hsl(${hue(p.name)} 60% 58%)`, flex: 'none', margin: '0 5px 0 4px' }} />
              <span className="t">{p.name}</span>
            </NavLink>
          ))}
        </div>

        <div className="side-bottom">
          <div className="nav-h"><span>Support</span></div>
          <a className="navlink" href="/docs" target="_blank" rel="noreferrer" title="Documentation"><Icon name="book" /><span className="nl">Documentation</span></a>
          <NavLink to="/app/settings" className={link} title="Settings"><Icon name="cog" /><span className="nl">Settings</span></NavLink>

          <div className="side-foot">
            <Menu block left up trigger={
              <span className="profile">
                <Avatar name={user.name} size={32} />
                <span className="who"><b>{user.name}</b><span>{user.email}</span></span>
                <Icon name="updown" size={14} />
              </span>
            } items={[
              { label: 'Settings', icon: 'cog', run: () => nav('/app/settings') },
              { label: theme === 'light' ? 'Dark mode' : 'Light mode', icon: theme === 'light' ? 'moon' : 'sun', run: () => setTheme(flipTheme()) },
              { label: 'Sign out', icon: 'out', danger: true, run: async () => { await logout(); nav('/') } },
            ]} />
          </div>
        </div>
      </aside>

      <div className="colmain" ref={scroller}>
        <header className="topbar">
          <button className="searchbtn" onClick={() => setPalette(true)}><Icon name="search" size={15} /><span>Search projects, pages, actions…</span><kbd>{isMac ? '⌘' : 'Ctrl'} K</kbd></button>
          <div style={{ flex: 1 }} />
          <ThemeToggle />
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
