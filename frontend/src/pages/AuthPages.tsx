import type { Row } from '../types'
import type { ReactNode } from 'react'
import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../auth'
import { Logo } from '../components/ui'

function Aside({ heading }: { heading: ReactNode }) {
  return (
    <aside className="auth-r">
      <h2>{heading}</h2>
      <div>
        <div className="code"><span className="c"># what your app sees</span>{'\n'}DATABASE_URL=<span className="y">••••••••••••</span>{'\n'}STRIPE_KEY=<span className="y">••••••••••••</span>{'\n'}JWT_SECRET=<span className="y">••••••••••••</span></div>
        <ul>
          <li><b>Encrypted at rest.</b> AES-256-GCM with a key per project.</li>
          <li><b>Versioned.</b> Roll back any change.</li>
          <li><b>Audited.</b> Every read and write is logged.</li>
        </ul>
      </div>
    </aside>
  )
}

function Layout({ title, sub, onSubmit, cta, alt, aside, children }: { title: string; sub?: string; onSubmit: (...args: any[]) => void; cta: string; alt?: any; aside?: any; children: ReactNode }) {
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  return (
    <div className="auth">
      <div className="auth-l">
        <header><Link to="/"><Logo /></Link><Link to="/" className="back">← Back to home</Link></header>
        <form className="auth-form" noValidate={false} onSubmit={async (e) => {
          e.preventDefault(); setErr(''); setBusy(true)
          try { await onSubmit(new FormData(e.target)) } catch (x) { setErr(x.message); setBusy(false) }
        }}>
          <h1>{title}</h1>
          <p className="sub">{sub}</p>
          {err && <div className="alert" role="alert">{err}</div>}
          {children}
          <button className="btn primary lg block" disabled={busy}>{busy ? <span className="spinner" style={{ width: 16, height: 16, borderColor: 'rgba(128,128,128,.5)', borderTopColor: 'var(--btn-ink)' }} /> : cta}</button>
          <p className="alt">{alt}</p>
        </form>
        <footer>Secrets are encrypted with AES-256-GCM before they reach the database.</footer>
      </div>
      {aside}
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint?: ReactNode; children: ReactNode }) {
  return <div className="field"><label>{label}{hint && <span className="hint">{hint}</span>}</label>{children}</div>
}

function Password({ name, autoComplete, placeholder, onChange }: { name: string; autoComplete: string; placeholder?: string; onChange?: (...args: any[]) => void }) {
  const [show, setShow] = useState(false)
  return (
    <div className="pwd">
      <input className="input" name={name} type={show ? 'text' : 'password'} required minLength={name === 'new' ? 8 : undefined} autoComplete={autoComplete} placeholder={placeholder} onChange={onChange} />
      <button type="button" onClick={() => setShow(!show)} tabIndex={-1}>{show ? 'Hide' : 'Show'}</button>
    </div>
  )
}

const safeNext = (p: URLSearchParams) => { const n = p.get('next'); return n && n.startsWith('/') && !n.startsWith('//') ? n : '/app' }

export function Login() {
  const { login } = useAuth(); const nav = useNavigate(); const [params] = useSearchParams()
  const next = safeNext(params); const q = params.get('next') ? `?next=${encodeURIComponent(next)}` : ''
  return (
    <Layout title="Welcome back" sub="Sign in to your workspace." cta="Sign in" aside={<Aside heading={<>Everything your app needs to <em>know</em>, and nothing it doesn’t.</>} />}
      alt={<>Don’t have an account? <Link to={`/register${q}`}>Create one</Link></>}
      onSubmit={async (f) => { await login(f.get('email'), f.get('password')); nav(next) }}>
      <Field label="Email"><input className="input" name="email" type="email" required autoFocus autoComplete="email" placeholder="you@company.com" /></Field>
      <Field label="Password"><Password name="password" autoComplete="current-password" /></Field>
    </Layout>
  )
}

const strength = (p: any) => (p.length === 0 ? 0 : Math.min(4, Number(p.length >= 8) + Number(p.length >= 12) + Number(/[A-Z]/.test(p) && /[a-z]/.test(p)) + Number(/\d/.test(p) && /[^A-Za-z0-9]/.test(p))) || 1)
const labels = ['', 'Weak', 'Okay', 'Good', 'Strong']

export function Register() {
  const { register } = useAuth(); const nav = useNavigate(); const [params] = useSearchParams()
  const next = safeNext(params); const q = params.get('next') ? `?next=${encodeURIComponent(next)}` : ''
  const [pw, setPw] = useState('')
  const s = strength(pw)
  return (
    <Layout title="Create your workspace" sub="Set up secure storage for your team in a minute." cta="Create account" aside={<Aside heading={<>Give every secret a <em>home</em> and a history.</>} />}
      alt={<>Already have an account? <Link to={`/login${q}`}>Sign in</Link></>}
      onSubmit={async (f) => { await register({ name: f.get('name'), email: f.get('email'), password: f.get('new'), org_name: f.get('org') }); nav(next) }}>
      <Field label="Full name"><input className="input" name="name" required autoFocus autoComplete="name" placeholder="Maya Patel" /></Field>
      <Field label="Work email"><input className="input" name="email" type="email" required autoComplete="email" placeholder="you@company.com" /></Field>
      <Field label="Password" hint={pw ? labels[s] : 'At least 8 characters'}>
        <Password name="new" autoComplete="new-password" onChange={(e) => setPw(e.target.value)} />
        <div className="meter" data-s={s}><i /><i /><i /><i /></div>
      </Field>
      <Field label="Workspace name" hint="Optional"><input className="input" name="org" placeholder="Acme Inc." /></Field>
    </Layout>
  )
}
