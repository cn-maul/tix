import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ListChecks, User, Lock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { loginSchema, type LoginFormValues } from '@/lib/validation'
import { useAuth } from '@/auth'
import { getSettings } from '@/api/settings'

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [loading, setLoading] = useState(false)

  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
    staleTime: 60_000,
  })
  const siteName = settings?.site_name || 'tix 工单系统'

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginFormValues>({ resolver: zodResolver(loginSchema) })

  const onFinish = async ({ username, password }: LoginFormValues) => {
    setLoading(true)
    try {
      await login(username, password)
      const next = params.get('next')
      // 防止开放重定向：必须以/开头，不能是/login，不能以//开头（协议相对URL）
      const safeNext =
        next && next.startsWith('/') && !next.startsWith('/login') && !next.startsWith('//')
          ? decodeURIComponent(next)
          : '/'
      navigate(safeNext, { replace: true })
    } catch (e: any) {
      toast.error(e?.message ?? '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gradient-to-br from-blue-50 via-background to-indigo-100 p-4 dark:from-transparent dark:via-transparent dark:to-transparent">
      <Card className="w-full max-w-sm shadow-lg">
        <CardHeader className="items-center text-center">
          <div className="mb-2 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
            <ListChecks className="size-6" />
          </div>
          <CardTitle className="text-xl">{siteName}</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onFinish)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">用户名</Label>
              <div className="relative">
                <User className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  id="username"
                  className="pl-9"
                  placeholder="用户名"
                  autoFocus
                  autoComplete="username"
                  {...register('username')}
                />
              </div>
              {errors.username && (
                <p className="text-sm text-destructive">{errors.username.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <div className="relative">
                <Lock className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  id="password"
                  type="password"
                  className="pl-9"
                  placeholder="密码"
                  autoComplete="current-password"
                  {...register('password')}
                />
              </div>
              {errors.password && (
                <p className="text-sm text-destructive">{errors.password.message}</p>
              )}
            </div>
            <Button type="submit" className="w-full" loading={loading}>
              登录
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
