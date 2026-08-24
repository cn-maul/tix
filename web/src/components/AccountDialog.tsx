import { toast } from 'sonner'
import { changePassword } from '@/api/auth'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { useAuth } from '@/auth'
import {
  useFormState,
  validatePasswordChange,
  type PasswordChangeValues,
} from '@/lib/validation'

// 账号弹窗：展示当前用户信息 + 自助修改密码。
// 改密成功后其他端会话被服务端吊销，当前会话保持登录。
export default function AccountDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { user } = useAuth()

  const { values, errors, set, submit, reset } = useFormState<PasswordChangeValues>(
    { old_password: '', new_password: '', confirm: '' },
    validatePasswordChange,
  )

  const onFinish = async ({ old_password, new_password }: PasswordChangeValues) => {
    try {
      await changePassword(old_password, new_password)
      reset()
      onOpenChange(false)
      toast.success('密码已修改，其他设备需重新登录')
    } catch (e: any) {
      toast.error(e?.message ?? '修改失败')
    }
  }

  if (!user) return null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>账号</DialogTitle>
          <DialogDescription className="flex items-center gap-2">
            <span>
              {user.display_name}（@{user.username}）
            </span>
            <Badge variant="secondary">{user.role === 'admin' ? '管理员' : '普通用户'}</Badge>
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={submit(onFinish)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="acc-old">旧密码</Label>
            <Input
              id="acc-old"
              type="password"
              autoComplete="current-password"
              value={values.old_password}
              onChange={(e) => set('old_password', e.target.value)}
            />
            {errors.old_password && (
              <p className="text-sm text-destructive">{errors.old_password}</p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="acc-new">新密码</Label>
            <Input
              id="acc-new"
              type="password"
              autoComplete="new-password"
              placeholder="至少 6 位"
              value={values.new_password}
              onChange={(e) => set('new_password', e.target.value)}
            />
            {errors.new_password && (
              <p className="text-sm text-destructive">{errors.new_password}</p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="acc-confirm">确认新密码</Label>
            <Input
              id="acc-confirm"
              type="password"
              autoComplete="new-password"
              value={values.confirm}
              onChange={(e) => set('confirm', e.target.value)}
            />
            {errors.confirm && (
              <p className="text-sm text-destructive">{errors.confirm}</p>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit">修改密码</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
