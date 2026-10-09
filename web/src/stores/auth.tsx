import { createContext, useContext, useEffect, useState } from 'react'
import { api } from '../services/api'

type User = { username: string; role: string }

type AuthCtx = {
  user: User | null | undefined // undefined = loading
  refresh: () => Promise<void>
  logout: () => Promise<void>
}

const Ctx = createContext<AuthCtx>({ user: undefined, refresh: async () => {}, logout: async () => {} })

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null | undefined>(undefined)

  const refresh = async () => {
    try {
      const me = await api.get<User>('/api/auth/me')
      setUser(me)
    } catch {
      setUser(null)
    }
  }

  const logout = async () => {
    try { await api.post('/api/auth/logout', {}) } catch { /* ignore */ }
    setUser(null)
  }

  useEffect(() => { refresh() }, [])

  return <Ctx.Provider value={{ user, refresh, logout }}>{children}</Ctx.Provider>
}

export function useAuth() { return useContext(Ctx) }
