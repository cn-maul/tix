import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

interface ThemeCtx {
  dark: boolean
  toggle: () => void
}

const ThemeContext = createContext<ThemeCtx>({ dark: false, toggle: () => {} })

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [dark, setDark] = useState(() => {
    try { return localStorage.getItem('tix.dark') === '1' } catch { return false }
  })

  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    try { localStorage.setItem('tix.dark', dark ? '1' : '0') } catch {}
  }, [dark])

  return (
    <ThemeContext.Provider value={{ dark, toggle: () => setDark((d) => !d) }}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme() {
  return useContext(ThemeContext)
}