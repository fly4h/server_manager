// Thin fetch wrapper matching the backend envelope:
// {success:true,data} | {success:false,error:{code,message,data}}

export class ApiError extends Error {
  code: string
  status: number
  data: any
  constructor(status: number, code: string, message: string, data?: any) {
    super(message)
    this.status = status
    this.code = code
    this.data = data
  }
}

async function request<T>(method: string, path: string, body?: any): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'include',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  let json: any = null
  try { json = await res.json() } catch { /* non-json */ }
  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent('sm:unauthorized'))
  }
  if (!res.ok || !json?.success) {
    const err = json?.error
    throw new ApiError(res.status, err?.code ?? 'UNKNOWN', err?.message ?? `HTTP ${res.status}`, err?.data)
  }
  return json.data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body: any = {}) => request<T>('POST', path, body),
  put: <T>(path: string, body: any) => request<T>('PUT', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}

// ---- shared types ----

export interface ServerRow {
  id: number
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'private_key'
  credential_id: number
  credential_name?: string
  description: string
  tags: string
  enabled: boolean
  last_status: string
  last_check_at?: string | null
  created_at: string
  updated_at: string
}

export interface CommandRow {
  id: number
  name: string
  description: string
  command: string
  danger_level: 'low' | 'medium' | 'high' | 'critical'
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface WebsiteRow {
  id: number
  name: string
  url: string
  icon: string
  category: string
  auth_type: string
  credential_id: number
  credential_name?: string
  description: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface CredentialRow {
  id: number
  name: string
  type: string
  created_at: string
  updated_at: string
}

export interface ActivityEntry {
  id: number
  username: string
  action: string
  target_type: string
  target_id: string
  metadata?: Record<string, string>
  created_at: string
}

export interface ContainerRow {
  id: string
  name: string
  image: string
  state: string
  status: string
}

export interface ImageRow {
  id: string
  tag: string
  size: number
  created: number
}

export interface Stats {
  servers: number
  servers_enabled: number
  servers_online: number
  websites: number
  commands: number
  server_status: Record<string, string>
  recent_activity: ActivityEntry[]
}
