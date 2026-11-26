// Shapes the API returns. Responses are JSON from the backend, so only the fields the UI relies on are listed.
export interface User {
  id: string
  email: string
  name?: string
  [key: string]: any
}

export interface Org {
  id: string
  name: string
  slug?: string
  role?: string
  [key: string]: any
}

// A generic API record (project, secret, member, token ...) whose fields are read by name.
export type Row = Record<string, any>

export type Tone = 'ok' | 'warn' | 'bad' | 'info' | 'idle'
