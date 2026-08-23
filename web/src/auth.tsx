import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { fetchAuthStatus, login as apiLogin, logout as apiLogout, type User } from './api/auth'

interface AuthCtx {
  authed: boolean | null
  user: User | null
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthCtx>({
  authed: null,
  user: null,
  login: async () => {},
  logout: async () => {},
})

// Session检查间隔（5分钟）
const SESSION_CHECK_INTERVAL = 5 * 60 * 1000

export function AuthProvider({ children }: { children: ReactNode }) {
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [user, setUser] = useState<User | null>(null)

  // 初始检查
  useEffect(() => {
    fetchAuthStatus().then(({ ok, user }) => {
      setAuthed(ok)
      setUser(user ?? null)
    })
  }, [])

  // 定期检查session是否过期
  useEffect(() => {
    if (!authed) return

    const interval = setInterval(async () => {
      try {
        const { ok } = await fetchAuthStatus()
        if (!ok) {
          setAuthed(false)
          setUser(null)
        }
      } catch {
        // 网络错误时不处理，下次检查时再判断
      }
    }, SESSION_CHECK_INTERVAL)

    return () => clearInterval(interval)
  }, [authed])

  const login = useCallback(async (username: string, password: string) => {
    const { user } = await apiLogin(username, password)
    setAuthed(true)
    setUser(user)
  }, [])

  const logout = useCallback(async () => {
    await apiLogout()
    setAuthed(false)
    setUser(null)
  }, [])

  return (
    <AuthContext.Provider value={{ authed, user, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  return useContext(AuthContext)
}
