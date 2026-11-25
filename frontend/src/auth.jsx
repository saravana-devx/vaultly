import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import { authApi, refreshSession, api } from './api.js'

const Ctx = createContext(null)
export const useAuth = () => useContext(Ctx)

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [orgs, setOrgs] = useState([])
  const [orgId, setOrgId] = useState(() => localStorage.getItem('vaultly_org'))
  const [loading, setLoading] = useState(true)

  const loadOrgs = useCallback(async () => {
    const list = await api('/orgs')
    setOrgs(list)
    setOrgId((cur) => {
      const id = list.find((o) => o.id === cur)?.id || list[0]?.id || null
      if (id) localStorage.setItem('vaultly_org', id)
      return id
    })
    return list
  }, [])

  const start = useCallback(async (u) => { setUser(u); await loadOrgs() }, [loadOrgs])

  useEffect(() => {
    refreshSession().then((u) => (u ? start(u) : null)).finally(() => setLoading(false))
    const onLogout = () => { setUser(null); setOrgs([]) }
    window.addEventListener('vaultly:logout', onLogout)
    return () => window.removeEventListener('vaultly:logout', onLogout)
  }, [start])

  const value = {
    user, orgs, loading, orgId,
    org: orgs.find((o) => o.id === orgId),
    switchOrg: (id) => { localStorage.setItem('vaultly_org', id); setOrgId(id) },
    reloadOrgs: loadOrgs,
    login: async (e, p) => start(await authApi.login(e, p)),
    register: async (b) => start(await authApi.register(b)),
    logout: async () => { await authApi.logout(); setUser(null); setOrgs([]) },
  }
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}
