import { ago, hue, Icon, Menu } from './ui.jsx'

const ENV_COLORS = { dev: '#60a5fa', development: '#60a5fa', staging: '#fbbf24', stage: '#fbbf24', prod: '#34d399', production: '#34d399' }

// Environment name with a colour dot: dev blue, staging amber, prod green, others by name.
export function EnvTag({ slug }) {
  const color = ENV_COLORS[slug] || `hsl(${hue(slug)} 65% 62%)`
  return <span className="envtag" style={{ '--c': color }}><i />{slug}</span>
}

// tone: ok | warn | bad | info | idle
export function StateDot({ tone = 'idle', children }) {
  return <span className={`state ${tone}`}><i />{children}</span>
}

export function Rel({ iso, empty = 'Never' }) {
  if (!iso) return <span className="dim">{empty}</span>
  return <time dateTime={iso} title={new Date(iso).toLocaleString()}>{ago(iso)}</time>
}

export function TableFoot({ children, right }) {
  return <div className="tfoot"><span>{children}</span>{right}</div>
}

export function EmptyRow({ cols, title, hint, icon = 'layers' }) {
  return (
    <tr className="empty-row"><td colSpan={cols}>
      <div className="empty-cell"><span className="empty-ic"><Icon name={icon} size={18} /></span><b>{title}</b>{hint && <span>{hint}</span>}</div>
    </td></tr>
  )
}

// Row actions stay out of the way until the row is hovered or focused.
export function RowMenu({ items }) {
  if (!items.filter(Boolean).length) return null
  return <span className="rowmenu"><Menu items={items} /></span>
}

export const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`
export const cap = (s = '') => s.charAt(0).toUpperCase() + s.slice(1)
