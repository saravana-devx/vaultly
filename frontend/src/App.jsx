import { Routes, Route, Navigate, useSearchParams } from 'react-router-dom'
import { useAuth } from './auth.jsx'
import Landing from './pages/Landing.jsx'
import { Login, Register } from './pages/AuthPages.jsx'
import Shell from './components/Shell.jsx'
import Dashboard from './pages/Dashboard.jsx'
import Projects from './pages/Projects.jsx'
import Project from './pages/Project.jsx'
import Members from './pages/Members.jsx'
import Audit from './pages/Audit.jsx'
import Settings from './pages/Settings.jsx'
import Invite from './pages/Invite.jsx'
import Docs from './pages/Docs.jsx'
import { ConfirmHost, Toaster } from './components/feedback.jsx'

function Private({ children }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="splash"><div className="spinner" /></div>
  return user ? children : <Navigate to="/login" replace />
}

function Public({ children }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="splash"><div className="spinner" /></div>
  const [params] = useSearchParams()
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
