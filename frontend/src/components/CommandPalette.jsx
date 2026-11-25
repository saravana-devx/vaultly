import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Avatar, Icon } from './ui.jsx'

export default function CommandPalette({ open, onClose, projects, admin }) {
  const nav = useNavigate()
  const [q, setQ] = useState('')
  const [idx, setIdx] = useState(0)
  const input = useRef()
  const list = useRef()

  const items = useMemo(() => {
    const go = (to) => () => nav(to)
    const base = [
      { group: 'Go to', label: 'Overview', icon: 'home', run: go('/app') },
      { group: 'Go to', label: 'All projects', icon: 'layers', run: go('/app/projects') },
      { group: 'Go to', label: 'Members', icon: 'users', run: go('/app/members') },
      ...(admin ? [{ group: 'Go to', label: 'Audit log', icon: 'log', run: go('/app/audit') }] : []),
      { group: 'Go to', label: 'Settings', icon: 'cog', run: go('/app/settings') },
    ]
    const actions = admin ? [
      { group: 'Actions', label: 'Create project', icon: 'plus', run: go('/app/projects?new=1') },
      { group: 'Actions', label: 'Invite a member', icon: 'plus', run: go('/app/members?invite=1') },
    ] : []
    const proj = projects.map((p) => ({ group: 'Projects', label: p.name, avatar: p.name, hint: `${p.secret_count} secrets`, run: go(`/app/projects/${p.id}`) }))
    return [...actions, ...base, ...proj]
  }, [projects, admin, nav])

  const shown = useMemo(() => {
    const t = q.trim().toLowerCase()
    return t ? items.filter((i) => i.label.toLowerCase().includes(t)) : items
  }, [items, q])

  useEffect(() => { if (open) { setQ(''); setIdx(0); setTimeout(() => input.current?.focus(), 0) } }, [open])
  useEffect(() => { setIdx(0) }, [q])
  useEffect(() => { list.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' }) }, [idx])
  if (!open) return null

  const choose = (it) => { onClose(); it?.run() }
  const onKey = (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setIdx((i) => Math.min(i + 1, shown.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setIdx((i) => Math.max(i - 1, 0)) }
    else if (e.key === 'Enter') { e.preventDefault(); choose(shown[idx]) }
    else if (e.key === 'Escape') onClose()
  }
  let last = ''
  return (
    <div className="overlay cmdk" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="cmdk-box" role="dialog" aria-label="Command palette">
        <div className="cmdk-in"><Icon name="search" size={16} /><input ref={input} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={onKey} placeholder="Search projects, pages and actions…" spellCheck={false} /><kbd>esc</kbd></div>
        <div className="cmdk-list" ref={list}>
          {shown.length === 0 && <div className="cmdk-empty">No results for “{q}”</div>}
          {shown.map((it, i) => {
            const head = it.group !== last ? <div className="cmdk-group">{(last = it.group)}</div> : null
            return (
              <div key={it.group + it.label}>
                {head}
                <button className="cmdk-item" data-active={i === idx} onMouseEnter={() => setIdx(i)} onClick={() => choose(it)}>
                  {it.avatar ? <Avatar name={it.avatar} size={20} square /> : <Icon name={it.icon} size={16} />}
                  <span>{it.label}</span>{it.hint && <em>{it.hint}</em>}
                </button>
              </div>
            )
          })}
        </div>
        <div className="cmdk-foot"><span><kbd>↑</kbd><kbd>↓</kbd> navigate</span><span><kbd>↵</kbd> open</span></div>
      </div>
    </div>
  )
}
