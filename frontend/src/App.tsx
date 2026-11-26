import type { ReactElement } from 'react'
import { Routes, Route, Navigate, useSearchParams } from 'react-router-dom'
import { useAuth } from './auth'
import Landing from './pages/Landing'
import { Login, Register } from './pages/AuthPages'
import Shell from './components/Shell'
import Dashboard from './pages/Dashboard'
import Projects from './pages/Projects'
import Project from './pages/Project'
import Members from './pages/Members'
import Audit from './pages/Audit'
import Settings from './pages/Settings'
import Invite from './pages/Invite'
import Docs from './pages/Docs'
import { ConfirmHost, Toaster } from './components/feedback'

function Private({ children }: { children: ReactElement }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="splash"><div className="spinner" /></div>
  return user ? children : <Navigate to="/login" replace />
}

function Public({ children }: { children: ReactElement }) {
  const { user, loading } = useAuth()
  const [params] = useSearchParams()
  if (loading) return <div className="splash"><div className="spinner" /></div>
  const next = params.get('next')
  return user ? <Navigate to={next && next.startsWith('/') && !next.startsWith('//') ? next : '/app'} replace /> : children
}

export default function App() {
  const { user, loading } = useAuth()
  return (
    <>
    <Routes>
      <Route path="/" element={loading ? <div className="splash"><div className="spinner" /></div> : <Landing signedIn={!!user} />} />
      <Route path="/login" element={<Public><Login /></Public>} />
      <Route path="/register" element={<Public><Register /></Public>} />
      <Route path="/invite/:token" element={<Invite />} />
      <Route path="/docs" element={<Docs />} />
      <Route path="/app" element={<Private><Shell /></Private>}>
        <Route index element={<Dashboard />} />
        <Route path="projects" element={<Projects />} />
        <Route path="projects/:pid" element={<Project />} />
        <Route path="members" element={<Members />} />
        <Route path="audit" element={<Audit />} />
        <Route path="settings" element={<Settings />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
    <Toaster />
    <ConfirmHost />
    </>
  )
}
