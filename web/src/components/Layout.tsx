import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import {
  LayoutDashboard,
  Clock,
  CheckCircle2,
  ListTodo,
  Settings,
  Search,
  Sun,
  Moon,
  LogOut,
  PanelLeft,
  ListChecks,
} from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { useTheme } from '@/lib/theme'
import { useAuth } from '@/auth'
import { getSettings } from '@/api/settings'

const DEFAULT_SITE_NAME = 'tix 工单'

const navGroups = [
  {
    title: '工单',
    items: [
      { to: '/tickets/done', icon: CheckCircle2, label: '已处理' },
      { to: '/tickets', icon: ListTodo, label: '全部工单', end: true },
    ],
  },
]

function NavItem({ to, icon: Icon, label, collapsed, end = false }: {
  to: string
  icon: typeof Clock
  label: string
  collapsed: boolean
  end?: boolean
}) {
  return (
    <NavLink
      to={to}
      end={end}
      className={({ isActive }) =>
        cn(
          'flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
          collapsed && 'justify-center px-2',
          isActive
            ? 'bg-primary/10 text-primary'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground',
        )
      }
    >
      <Icon className="size-4 shrink-0" />
      {!collapsed && <span>{label}</span>}
    </NavLink>
  )
}

export default function Layout() {
  const [collapsed, setCollapsed] = useState(false)
  const [search, setSearch] = useState('')
  const navigate = useNavigate()
  const { dark, toggle } = useTheme()
  const { logout, user } = useAuth()
  const isAdmin = user?.role === 'admin'

  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
  })
  const siteName = settings?.site_name || DEFAULT_SITE_NAME

  const onLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  const displayName = user?.display_name || user?.username || '管'
  const initial = displayName.charAt(0)

  return (
    <div className="flex h-screen overflow-hidden bg-muted/30">
      <aside
        className={cn(
          'flex shrink-0 flex-col border-r bg-sidebar transition-[width] duration-200',
          collapsed ? 'w-16' : 'w-56',
        )}
      >
        <div className="flex h-14 items-center gap-2 border-b px-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <ListChecks className="size-4" />
          </div>
          {!collapsed && <span className="text-base font-semibold">{siteName}</span>}
        </div>

        <nav className="flex-1 space-y-4 overflow-y-auto px-2 pb-4 pt-2">
          <NavItem to="/" icon={Clock} label="待处理" collapsed={collapsed} end />
          {navGroups.map((g) => (
            <div key={g.title}>
              {!collapsed && (
                <div className="px-3 pb-1 text-xs font-medium text-muted-foreground">{g.title}</div>
              )}
              <div className="space-y-0.5">
                {g.items.map((it) => (
                  <NavItem key={it.to} {...it} collapsed={collapsed} />
                ))}
              </div>
            </div>
          ))}
          <NavItem to="/dashboard" icon={LayoutDashboard} label="统计" collapsed={collapsed} />
          {isAdmin && <NavItem to="/settings" icon={Settings} label="设置" collapsed={collapsed} />}
        </nav>

        <div className="border-t p-2">
          <div className={cn('flex items-center gap-1', collapsed && 'flex-col')}>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setCollapsed((c) => !c)}
              aria-label="收起侧边栏"
            >
              <PanelLeft className="size-4" />
            </Button>
            <div className={cn('flex-1', collapsed && 'hidden')} />
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon" onClick={toggle} aria-label="切换主题">
                  {dark ? <Sun /> : <Moon />}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{dark ? '切换到亮色模式' : '切换到暗色模式'}</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center justify-between gap-4 border-b bg-background px-4">
          <div className="flex min-w-0 items-center gap-2">
            <div className="relative w-full max-w-xs">
              <Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                className="pl-8"
                placeholder="搜索工单…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    navigate(`/tickets?keyword=${encodeURIComponent(search.trim())}`)
                  }
                }}
              />
            </div>
          </div>

          <div className="flex items-center gap-1.5">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon" onClick={onLogout} aria-label="退出登录">
                  <LogOut />
                </Button>
              </TooltipTrigger>
              <TooltipContent>退出登录</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="ml-1 flex size-8 cursor-default items-center justify-center rounded-full bg-gradient-to-br from-blue-500 to-indigo-600 text-sm font-medium text-white">
                  {initial}
                </div>
              </TooltipTrigger>
              <TooltipContent>{displayName}</TooltipContent>
            </Tooltip>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}