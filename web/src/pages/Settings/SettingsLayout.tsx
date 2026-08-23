import { NavLink, Outlet, Navigate } from 'react-router-dom'
import { Settings2, Tags, Database, Users, BellRing } from 'lucide-react'
import { cn } from '@/lib/utils'
import { APP_VERSION_LABEL } from '@/lib/version'
import { useAuth } from '@/auth'

const tabs = [
  { to: '/settings/general', icon: Settings2, label: '通用' },
  { to: '/settings/notifications', icon: BellRing, label: '消息推送' },
  { to: '/settings/users', icon: Users, label: '用户管理' },
  { to: '/settings/categories', icon: Tags, label: '分类管理' },
  { to: '/settings/data', icon: Database, label: '数据与备份' },
]

export default function SettingsLayout() {
  const { authed, user } = useAuth()

  if (authed === null) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
      </div>
    )
  }

  if (!authed || user?.role !== 'admin') {
    return <Navigate to="/" replace />
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <h1 className="text-xl font-semibold">设置</h1>
        <span className="rounded-md border px-2 py-0.5 font-mono text-xs text-muted-foreground">
          {APP_VERSION_LABEL}
        </span>
      </div>
      <div className="flex w-fit items-center gap-1 rounded-lg bg-muted p-1">
        {tabs.map((t) => (
          <NavLink
            key={t.to}
            to={t.to}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground',
              )
            }
          >
            <t.icon className="size-4" />
            {t.label}
          </NavLink>
        ))}
      </div>
      <Outlet />
    </div>
  )
}
