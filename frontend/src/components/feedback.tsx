import { useEffect, useRef, useState } from 'react'

type ToastType = 'success' | 'error' | 'info'
interface ToastItem { id: number; type: ToastType; message: string }
interface AskRequest { message: string; title?: string; confirm?: string; danger?: boolean; resolve: (v: boolean) => void }
import { Icon, Modal } from './ui'

const TOAST_EVENT = 'vaultly:toast'
const ASK_EVENT = 'vaultly:ask'

const emit = (type: ToastType, message: string) => window.dispatchEvent(new CustomEvent(TOAST_EVENT, { detail: { type, message } }))

// toast('Saved') shows a success toast; toast.error('...') and toast.info('...') pick the style.
export const toast = Object.assign((message: string) => emit('success', message), {
  success: (message: string) => emit('success', message),
  error: (message: string) => emit('error', message),
  info: (message: string) => emit('info', message),
})

// const ok = await ask('Delete this secret?', { confirm: 'Delete', danger: true })
export const ask = (message: string, opts: { title?: string; confirm?: string; danger?: boolean } = {}) =>
  new Promise<boolean>((resolve) => window.dispatchEvent(new CustomEvent(ASK_EVENT, { detail: { message, resolve, ...opts } })))

let nextId = 1
const LIFETIME: Record<string, number> = { success: 3500, info: 4500, error: 7000 }

export function Toaster() {
  const [items, setItems] = useState<ToastItem[]>([])
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>())

  const dismiss = (id: number) => {
    clearTimeout(timers.current.get(id))
    timers.current.delete(id)
    setItems((list) => list.filter((t) => t.id !== id))
  }
  const arm = (id: number, type: ToastType) => timers.current.set(id, setTimeout(() => dismiss(id), LIFETIME[type] || 4000))

  useEffect(() => {
    const onToast = (e: Event) => {
      const { type, message } = (e as CustomEvent<{ type: ToastType; message: string }>).detail
      if (!message) return
      const id = nextId++
      setItems((list) => [...list.filter((t) => t.message !== message || t.type !== type), { id, type, message }].slice(-4))
      arm(id, type)
    }
    window.addEventListener(TOAST_EVENT, onToast)
    return () => window.removeEventListener(TOAST_EVENT, onToast)
  }, [])

  return (
    <div className="toaster" role="region" aria-label="Notifications">
      {items.map((t) => (
        <div
          key={t.id}
          className={`toast-item ${t.type}`}
          role={t.type === 'error' ? 'alert' : 'status'}
          onMouseEnter={() => clearTimeout(timers.current.get(t.id))}
          onMouseLeave={() => arm(t.id, t.type)}
        >
          <span className="toast-ic"><Icon name={t.type === 'error' ? 'alert' : 'check'} size={14} stroke={2.4} /></span>
          <span className="toast-msg">{t.message}</span>
          <button className="toast-x" aria-label="Dismiss" onClick={() => dismiss(t.id)}><Icon name="x" size={14} /></button>
        </div>
      ))}
    </div>
  )
}

export function ConfirmHost() {
  const [req, setReq] = useState<AskRequest | null>(null)
  useEffect(() => {
    const onAsk = (e: Event) => setReq((e as CustomEvent<AskRequest>).detail)
    window.addEventListener(ASK_EVENT, onAsk)
    return () => window.removeEventListener(ASK_EVENT, onAsk)
  }, [])
  if (!req) return null
  const close = (value: boolean) => { req.resolve(value); setReq(null) }
  return (
    <Modal title={req.title || 'Are you sure?'} onClose={() => close(false)}>
      <p className="confirm-msg">{req.message}</p>
      <div className="modal-actions">
        <button className="btn" onClick={() => close(false)}>Cancel</button>
        <button className={`btn ${req.danger ? 'solid-danger' : 'primary'}`} autoFocus onClick={() => close(true)}>{req.confirm || 'Confirm'}</button>
      </div>
    </Modal>
  )
}
