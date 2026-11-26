import type { ReactNode, CSSProperties } from 'react'
import { ago, hue, Icon, Menu, type MenuItem } from './ui'

const ENV_COLORS: Record<string, string> = { dev: '#60a5fa', development: '#60a5fa', staging: '#fbbf24', stage: '#fbbf24', prod: '#34d399', production: '#34d399' }

// Environment name with a colour dot: dev blue, staging amber, prod green, others by name.
export function EnvTag({ slug }: { slug: string }) {
  const color = ENV_COLORS[slug] || `hsl(${hue(slug)} 65% 62%)`
  return <span className="envtag" style={{ '--c': color } as CSSProperties}><i />{slug}</span>
}

// tone: ok | warn | bad | info | idle
export function StateDot({ tone = 'idle', children }: { tone?: string; children?: ReactNode }) {
  return <span className={`state ${tone}`}><i />{children}</span>
}

export function Rel({ iso, empty = 'Never' }: { iso?: string | null; empty?: string }) {
  if (!iso) return <span className="dim">{empty}</span>
  return <time dateTime={iso} title={new Date(iso).toLocaleString()}>{ago(iso)}</time>
}

export function TableFoot({ children, right }: { children?: ReactNode; right?: ReactNode }) {
  return <div className="tfoot"><span>{children}</span>{right}</div>
}

export function EmptyRow({ cols, title, hint, icon = 'layers' }: { cols: number; title: string; hint?: ReactNode; icon?: string }) {
  return (
    <tr className="empty-row"><td colSpan={cols}>
      <div className="empty-cell"><span className="empty-ic"><Icon name={icon} size={18} /></span><b>{title}</b>{hint && <span>{hint}</span>}</div>
    </td></tr>
  )
}

// Row actions stay out of the way until the row is hovered or focused.
export function RowMenu({ items }: { items: (MenuItem | false | null | undefined)[] }) {
  if (!items.filter(Boolean).length) return null
  return <span className="rowmenu"><Menu items={items} /></span>
}

export const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`
export const cap = (s: string = '') => s.charAt(0).toUpperCase() + s.slice(1)
