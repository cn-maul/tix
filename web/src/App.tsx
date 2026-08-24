import { useEffect } from 'react'
import { useQuery, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from 'sonner'
import Router from './router'
import { AuthProvider } from './auth'
import { ThemeProvider } from './lib/theme'
import { getSettings } from './api/settings'
import { ErrorBoundary } from './components/ErrorBoundary'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false },
  },
})

// SiteTitle 全局同步浏览器标签页标题：跟随「系统设置-站点名称」，
// 覆盖登录页、游客提交页与所有管理页，改名后即时生效（设置页已失效该查询）。
function SiteTitle() {
  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
  })
  useEffect(() => {
    document.title = settings?.site_name || 'tix 工单系统'
  }, [settings])
  return null
}

export default function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <SiteTitle />
        <ThemeProvider>
          <BrowserRouter>
            <AuthProvider>
              <Router />
              <Toaster richColors position="top-center" closeButton />
            </AuthProvider>
          </BrowserRouter>
        </ThemeProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  )
}