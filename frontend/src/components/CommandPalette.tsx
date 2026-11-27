import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Avatar, Icon, useScrollLock } from './ui'
import type { Row } from '../types'

interface PaletteItem { id: string; kind: string; label: string; desc: string; icon?: string; avatar?: string; run: () => void; group?: string }

const SCOPES: [string, string][] = [['all', 'All'], ['Page', 'Pages'], ['Action', 'Actions'], ['Project', 'Projects']]
const RECENT_KEY = 'vaultly-recent'

const readRecent = (): string[] => { try { return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]') } catch { return [] } }
const saveRecent = (id: string) => { try { localStorage.setItem(RECENT_KEY, JSON.stringify([id, ...readRecent().filter((x) => x !== id)].slice(0, 4))) } catch { /* not remembered */ } }

export default function CommandPalette({ open, onClose, projects, admin }: { open: boolean; onClose: () => void; projects: Row[]; admin: boolean }) {
  const nav = useNavigate()
  useScrollLock(open)
  const [q, setQ] = useState('')
  const [scope, setScope] = useState('all')
  const [idx, setIdx] = useState(0)
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)

  const items = useMemo<PaletteItem[]>(() => {
    const go = (to: string) => () => nav(to)
    const pages = [
      { id: 'p:overview', kind: 'Page', label: 'Overview', desc: 'Your workspace at a glance', icon: 'home', run: go('/dashboard') },
      { id: 'p:projects', kind: 'Page', label: 'All projects', desc: 'Browse every project', icon: 'layers', run: go('/dashboard/projects') },
      { id: 'p:members', kind: 'Page', label: 'Members', desc: 'People and their access', icon: 'users', run: go('/dashboard/members') },
      ...(admin ? [{ id: 'p:audit', kind: 'Page', label: 'Audit log', desc: 'Who did what, and when', icon: 'log', run: go('/dashboard/audit') }] : []),
      { id: 'p:settings', kind: 'Page', label: 'Settings', desc: 'Your account, tokens and backups', icon: 'cog', run: go('/dashboard/settings') },
      { id: 'p:docs', kind: 'Page', label: 'Documentation', desc: 'Quickstart, CLI and API', icon: 'book', run: () => window.open('/docs', '_blank', 'noopener') },
    ]
    const actions = admin ? [
      { id: 'a:project', kind: 'Action', label: 'Create project', desc: 'Start a new set of secrets', icon: 'plus', run: go('/dashboard/projects?new=1') },
      { id: 'a:invite', kind: 'Action', label: 'Invite a member', desc: 'Send an invitation by email', icon: 'users', run: go('/dashboard/members?invite=1') },
    ] : []
    const proj = projects.map((p): PaletteItem => ({
      id: `j:${p.id}`, kind: 'Project', label: p.name, avatar: p.name,
      desc: `${p.secret_count} secret${p.secret_count === 1 ? '' : 's'} · ${p.environments.length} environment${p.environments.length === 1 ? '' : 's'}`,
      run: go(`/dashboard/projects/${p.id}`),
    }))
    return [...actions, ...pages, ...proj]
  }, [projects, admin, nav])

  // With nothing typed: Recent first, then the rest in groups. While typing: best matches first.
  const { shown, grouped } = useMemo(() => {
    const t = q.trim().toLowerCase()
    const pool = items.filter((i) => scope === 'all' || i.kind === scope)
    if (t) {
      const score = (i: PaletteItem) => { const l = i.label.toLowerCase(); return l.startsWith(t) ? 0 : l.split(/[\s-_/]+/).some((w) => w.startsWith(t)) ? 1 : l.includes(t) ? 2 : i.desc.toLowerCase().includes(t) ? 3 : 9 }
      return { shown: pool.map((i): [number, PaletteItem] => [score(i), i]).filter(([s]) => s < 9).sort((a, b) => a[0] - b[0]).map(([, i]) => i), grouped: false }
    }
    const recent = scope === 'all' ? readRecent().map((id) => pool.find((i) => i.id === id)).filter((i): i is PaletteItem => Boolean(i)).map((i) => ({ ...i, group: 'Recent' })) : []
    const rest = pool.filter((i) => !recent.some((r) => r.id === i.id)).map((i) => ({ ...i, group: i.kind === 'Page' ? 'Go to' : i.kind === 'Action' ? 'Actions' : 'Projects' }))
    return { shown: [...recent, ...rest], grouped: true }
  }, [items, q, scope])

  useEffect(() => { if (open) { setQ(''); setScope('all'); setIdx(0); setTimeout(() => input.current?.focus(), 0) } }, [open])
  useEffect(() => { setIdx(0) }, [q, scope])
  useEffect(() => { list.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' }) }, [idx])
  if (!open) return null

  const choose = (it?: PaletteItem) => { if (!it) return; saveRecent(it.id); onClose(); it.run() }
  const cycleScope = (dir: number) => { const i = SCOPES.findIndex(([k]) => k === scope); setScope(SCOPES[(i + dir + SCOPES.length) % SCOPES.length][0]) }
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setIdx((i) => Math.min(i + 1, shown.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setIdx((i) => Math.max(i - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); choose(shown[idx]) }
    else if (e.key === 'Tab') { e.preventDefault(); cycleScope(e.shiftKey ? -1 : 1) }
    else if (e.key === 'Escape') onClose()
  }

  let last = ''
  return (
    <div className="overlay cmdk" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="pal" role="dialog" aria-label="Command palette">
        <div className="pal-in">
          <Icon name="search" size={20} />
          <input ref={input} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onKey} placeholder="Search or jump to…" spellCheck={false} aria-label="Search" />
          <kbd>esc</kbd>
        </div>
        <div className="pal-scopes" role="tablist" aria-label="Filter results">
          {SCOPES.map(([key, label]) => <button key={key} role="tab" aria-selected={scope === key} className={scope === key ? 'on' : ''} onClick={() => { setScope(key); input.current?.focus() }}>{label}</button>)}
        </div>
        <div className="pal-list" ref={list}>
          {shown.length === 0 && <div className="pal-empty"><Icon name="search" size={22} /><b>Nothing found for “{q}”</b><span>Try a project name, or press Tab to change the filter.</span></div>}
          {shown.map((it, i) => {
            const head = grouped && it.group !== last ? <div className="pal-group">{(last = it.group ?? '')}</div> : null
            return (
              <div key={it.id + (it.group || '')}>
                {head}
                <button className="pal-row" data-active={i === idx} onMouseEnter={() => setIdx(i)} onClick={() => choose(it)}>
                  <span className="pal-tile">{it.avatar ? <Avatar name={it.avatar} size={30} square /> : <Icon name={it.icon} size={17} />}</span>
                  <span className="pal-t"><b>{it.label}</b><small>{it.desc}</small></span>
                  <span className="pal-kind">{it.kind}</span>
                  <span className="pal-go"><Icon name="right" size={14} /></span>
                </button>
              </div>
            )
          })}
        </div>
        <div className="pal-foot">
          <span><kbd>↑</kbd><kbd>↓</kbd> move</span><span><kbd>↵</kbd> open</span><span><kbd>tab</kbd> filter</span>
          <em>{shown.length} result{shown.length === 1 ? '' : 's'}</em>
        </div>
      </div>
    </div>
  )
}
