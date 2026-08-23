import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../auth'

export default function RequireAuth() {
  const { authed } = useAuth()
  const location = useLocation()

  if (authed === null) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
      </div>
    )
  }

  if (!authed) {
    const next = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?next=${next}`} replace />
  }

  return <Outlet />
}