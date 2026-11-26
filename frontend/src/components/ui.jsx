import { useEffect, useState } from 'react'
import { toast } from './feedback.jsx'

export function Logo({ size = 26 }) {
  return (
    <span className="logo">
      <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
        <rect width="32" height="32" rx="8" fill="#10b981" />
        <circle cx="16" cy="13" r="4.5" fill="#04130d" />
        <path d="M13.5 16.5h5l1.5 8h-8z" fill="#04130d" />
      </svg>
      <span className="logo-t">Vaultly</span>
    </span>
  )
}

const PATHS = {
  home: <path d="M4 11l8-7 8 7M6 10v10h12V10" />,
  layers: <><path d="M12 3l9 5-9 5-9-5z" /><path d="M3 13l9 5 9-5" /></>,
  users: <><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20c0-3.6 2.9-5.5 6.5-5.5s6.5 1.9 6.5 5.5M16.5 4.7a3.5 3.5 0 010 6.6M18 14.8c2.2.5 3.5 2.2 3.5 5.2" /></>,
  log: <><path d="M6 3h8l5 5v13H6z" /><path d="M9 13h7M9 17h7M9 9h2" /></>,
  eye: <><path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" /></>,
  eyeoff: <><path d="M3 3l18 18M10.6 5.1A9.7 9.7 0 0112 5c6.4 0 10 7 10 7a17 17 0 01-3.2 4M6.5 6.7C3.9 8.4 2 12 2 12s3.6 7 10 7c1.7 0 3.2-.5 4.5-1.2" /></>,
  copy: <><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V6a2 2 0 00-2-2H6a2 2 0 00-2 2v8a2 2 0 002 2h2" /></>,
  pencil: <path d="M4 20l1-4L16.5 4.5a2 2 0 012.8 0l.2.2a2 2 0 010 2.8L8 19z" />,
  history: <><path d="M3 12a9 9 0 109-9 9.7 9.7 0 00-7 3M3 3v4h4M12 7v5l3 2" /></>,
  rotate: <><path d="M20 11a8 8 0 00-14.3-3.8L4 9M4 4v5h5M4 13a8 8 0 0014.3 3.8L20 15M20 20v-5h-5" /></>,
  trash: <path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" />,
  plus: <path d="M12 5v14M5 12h14" />,
  down: <path d="M6 9l6 6 6-6" />,
  right: <path d="M9 6l6 6-6 6" />,
  folder: <path d="M3 6.5A1.5 1.5 0 014.5 5H10l2 2.5h7.5A1.5 1.5 0 0121 9v9.5a1.5 1.5 0 01-1.5 1.5h-15A1.5 1.5 0 013 18.5z" />,
  download: <path d="M12 4v11M7 11l5 5 5-5M5 20h14" />,
  upload: <path d="M12 16V5M7 9l5-5 5 5M5 20h14" />,
  check: <path d="M5 12l5 5 9-10" />,
  x: <path d="M6 6l12 12M18 6L6 18" />,
  out: <path d="M10 4H5v16h5M15 8l4 4-4 4M19 12H9" />,
  cog: <><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z" /></>,
  command: <path d="M9 6a3 3 0 10-3 3h12a3 3 0 10-3-3v12a3 3 0 103-3H6a3 3 0 103 3z" />,
  key2: <><circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 20 3m-4 4 3 3" /></>,
  dots: <><circle cx="5" cy="12" r="1.2" /><circle cx="12" cy="12" r="1.2" /><circle cx="19" cy="12" r="1.2" /></>,
  search: <><circle cx="11" cy="11" r="6.5" /><path d="M16 16l4.5 4.5" /></>,
  alert: <><path d="M12 4l9.5 16h-19z" /><path d="M12 10v4M12 17.2v.1" /></>,
  panel: <><rect x="3.5" y="4.5" width="17" height="15" rx="2.5" /><path d="M9.5 4.5v15" /></>,
  updown: <path d="M8 9.5l4-4 4 4M8 14.5l4 4 4-4" />,
  book: <><path d="M5 4.5h10a3 3 0 013 3V20H8a3 3 0 01-3-3z" /><path d="M5 17a3 3 0 013-3h10" /></>,
  pulse: <path d="M3 12h4l2.5-7 4 14 2.5-7H21" />,
  shield: <><path d="M12 3l8 3v6c0 4.5-3.3 8-8 9-4.7-1-8-4.5-8-9V6z" /><path d="M9 12l2 2 4-4" /></>,
  sun: <><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></>,
  moon: <path d="M20.5 13.2A8.5 8.5 0 1110.8 3.5a6.8 6.8 0 009.7 9.7z" />,
}

export function Icon({ name, size = 18, stroke = 1.7 }) {
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{PATHS[name]}</svg>
}

// Freezes whatever scrolls behind a dialog while it is open (works when dialogs nest).
export function useScrollLock(active = true) {
  useEffect(() => {
    if (!active) return undefined
    const n = Number(document.body.dataset.locks || 0) + 1
    document.body.dataset.locks = String(n)
    document.body.classList.add('scroll-locked')
    return () => {
      const left = Math.max(0, Number(document.body.dataset.locks || 1) - 1)
      document.body.dataset.locks = String(left)
      if (left === 0) document.body.classList.remove('scroll-locked')
    }
  }, [active])
}

export function Modal({ title, onClose, children, wide }) {
  useScrollLock()
  useEffect(() => {
    const h = (e) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [onClose])
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`modal${wide ? ' wide' : ''}`} role="dialog" aria-label={title}>
        <div className="modal-h"><h2>{title}</h2><button className="iconbtn" onClick={onClose} aria-label="Close"><Icon name="x" size={16} /></button></div>
        {children}
      </div>
    </div>
  )
}

export const copy = async (text, label = 'Copied') => {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(label)
    return true
  } catch {
    toast.error('Could not copy. Select the text and copy it manually.')
    return false
  }
}

export const ago = (iso) => {
  if (!iso) return 'Never'
  const s = (Date.now() - new Date(iso).getTime()) / 1000
  if (s < 60) return 'Just now'
  if (s < 3600) return `${Math.floor(s / 60)} min ago`
  if (s < 86400) return `${Math.floor(s / 3600)} hr ago`
  if (s < 86400 * 30) return `${Math.floor(s / 86400)} d ago`
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })
}

const HUES = [152, 200, 262, 24, 330, 48, 180, 12]
export const hue = (str = '') => HUES[[...str].reduce((a, c) => a + c.charCodeAt(0), 0) % HUES.length]

export function Avatar({ name = '?', size = 26, square }) {
  const h = hue(name)
  return (
    <span className="av" style={{ width: size, height: size, fontSize: size * 0.42, borderRadius: square ? 7 : '50%', '--h': h }}>
      {name.trim()[0]?.toUpperCase()}
    </span>
  )
}

// up / left open the menu above / to the right of its trigger; block makes the trigger fill its row.
export function Menu({ items, trigger, up, left, block }) {
  const [open, setOpen] = useState(false)
  useEffect(() => {
    if (!open) return
    const close = () => setOpen(false)
    window.addEventListener('click', close)
    window.addEventListener('keydown', (e) => e.key === 'Escape' && close(), { once: true })
    return () => window.removeEventListener('click', close)
  }, [open])
  return (
    <span className={`menu-wrap${block ? ' fill' : ''}`}>
      {trigger ? <button className={`trigger${block ? ' fill' : ''}`} aria-label="Open menu" onClick={(e) => { e.stopPropagation(); setOpen(!open) }}>{trigger}</button>
        : <button className="iconbtn" aria-label="More actions" onClick={(e) => { e.stopPropagation(); setOpen(!open) }}><Icon name="dots" size={16} /></button>}
      {open && (
        <div className={`menu${up ? ' up' : ''}${left ? ' left' : ''}`} role="menu">
          {items.filter(Boolean).map((it) => (
            <button key={it.label} role="menuitem" className={it.danger ? 'danger' : ''} onClick={() => { setOpen(false); it.run() }}><Icon name={it.icon} size={15} />{it.label}{it.active && <Icon name="check" size={14} />}</button>
          ))}
        </div>
      )}
    </span>
  )
}

export const shortName = (actor = '') => actor.startsWith('token:') ? actor : actor.split('@')[0]

export const currentTheme = () => (document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark')

// Switches the colours immediately, remembers the choice, and returns the new theme.
export function flipTheme() {
  const next = currentTheme() === 'light' ? 'dark' : 'light'
  document.documentElement.setAttribute('data-theme', next)
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', next === 'light' ? '#FBFCFC' : '#07090d')
  try { localStorage.setItem('vaultly-theme', next) } catch { /* private mode: the choice just is not remembered */ }
  return next
}

// Sun / moon button.
export function ThemeToggle() {
  const [theme, setTheme] = useState(currentTheme)
  const label = theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'
  return <button className="iconbtn" onClick={() => setTheme(flipTheme())} title={label} aria-label={label}><Icon name={theme === 'light' ? 'moon' : 'sun'} size={16} /></button>
}
