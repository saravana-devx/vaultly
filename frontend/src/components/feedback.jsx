import { useEffect, useRef, useState } from 'react'
import { Icon, Modal } from './ui.jsx'

const TOAST_EVENT = 'vaultly:toast'
const ASK_EVENT = 'vaultly:ask'

const emit = (type, message) => window.dispatchEvent(new CustomEvent(TOAST_EVENT, { detail: { type, message } }))

// toast('Saved') shows a success toast; toast.error('...') and toast.info('...') pick the style.
export const toast = Object.assign((message) => emit('success', message), {
  success: (message) => emit('success', message),
  error: (message) => emit('error', message),
  info: (message) => emit('info', message),
})

// const ok = await ask('Delete this secret?', { confirm: 'Delete', danger: true })
export const ask = (message, opts = {}) =>
  new Promise((resolve) => window.dispatchEvent(new CustomEvent(ASK_EVENT, { detail: { message, resolve, ...opts } })))

let nextId = 1
const LIFETIME = { success: 3500, info: 4500, error: 7000 }

export function Toaster() {
  const [items, setItems] = useState([])
  const timers = useRef(new Map())

  const dismiss = (id) => {
    clearTimeout(timers.current.get(id))
    timers.current.delete(id)
    setItems((list) => list.filter((t) => t.id !== id))
  }
  const arm = (id, type) => timers.current.set(id, setTimeout(() => dismiss(id), LIFETIME[type] || 4000))

  useEffect(() => {
    const onToast = (e) => {
      const { type, message } = e.detail
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
  const [req, setReq] = useState(null)
  useEffect(() => {
    const onAsk = (e) => setReq(e.detail)
    window.addEventListener(ASK_EVENT, onAsk)
    return () => window.removeEventListener(ASK_EVENT, onAsk)
  }, [])
  if (!req) return null
  const close = (value) => { req.resolve(value); setReq(null) }
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
