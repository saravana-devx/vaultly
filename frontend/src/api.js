// Access token lives in memory only; the refresh token is an httpOnly cookie.
let accessToken = null
let refreshing = null

export const setToken = (t) => { accessToken = t }

async function raw(path, { method = 'GET', body, auth = true } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (auth && accessToken) headers.Authorization = `Bearer ${accessToken}`
  return fetch(`/api${path}`, { method, headers, credentials: 'same-origin', body: body === undefined ? undefined : JSON.stringify(body) })
}

export async function refreshSession() {
  refreshing ||= raw('/auth/refresh', { method: 'POST', auth: false })
    .then(async (r) => {
      if (!r.ok) return null
      const d = await r.json()
      accessToken = d.access_token
      return d.user
    })
    .finally(() => { refreshing = null })
  return refreshing
}

export class ApiError extends Error {
  constructor(status, message) { super(message); this.status = status }
}

export async function api(path, opts = {}) {
  let r = await raw(path, opts)
  if (r.status === 401 && opts.auth !== false) {
    if (await refreshSession()) r = await raw(path, opts)
    else window.dispatchEvent(new Event('vaultly:logout'))
  }
  if (r.status === 204) return null
  const ct = r.headers.get('content-type') || ''
  const data = ct.includes('json') ? await r.json() : await r.text()
  if (!r.ok) throw new ApiError(r.status, data?.error || 'Something went wrong')
  return data
}

export const authApi = {
  login: async (email, password) => {
    const d = await api('/auth/login', { method: 'POST', body: { email, password }, auth: false })
    accessToken = d.access_token
    return d.user
  },
  register: async (body) => {
    const d = await api('/auth/register', { method: 'POST', body, auth: false })
    accessToken = d.access_token
    return d.user
  },
  logout: async () => { await api('/auth/logout', { method: 'POST', auth: false }).catch(() => {}); accessToken = null },
}

async function authed(path, init) {
  const go = () => fetch(`/api${path}`, { ...init, credentials: 'same-origin', headers: { ...(init.headers || {}), ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}) } })
  let r = await go()
  if (r.status === 401 && (await refreshSession())) r = await go()
  return r
}

export async function apiBlob(path, body) {
  const r = await authed(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
  if (!r.ok) throw new ApiError(r.status, (await r.json().catch(() => ({}))).error || 'Something went wrong')
  return r.blob()
}

export async function apiForm(path, form) {
  const r = await authed(path, { method: 'POST', body: form })
  const d = await r.json().catch(() => ({}))
  if (!r.ok) throw new ApiError(r.status, d.error || 'Something went wrong')
  return d
}
