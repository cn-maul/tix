import { useState } from 'react'
import { type User as UserType } from '../../api/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  useFormState,
  validateUserCreate,
  validateUserEdit,
  type UserCreateValues,
  type UserEditValues,
} from '@/lib/validation'

export function FieldError({ msg }: { msg?: string }) {
  if (!msg) return null
  return <p className="text-sm text-destructive">{msg}</p>
}

export function CreateDialog({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onSubmit: (data: { username: string; password: string; display_name: string; role: string }) => void
}) {
  const [role, setRole] = useState<'operator' | 'admin'>('operator')
  const { values, errors, set, reset, submit } = useFormState<UserCreateValues>(
    { username: '', password: '', display_name: '' }, validateUserCreate,
  )
  const onFinish = async (v: UserCreateValues) => {
    try {
      await onSubmit({ username: v.username.trim(), password: v.password, display_name: v.display_name.trim(), role })
      reset()
      setRole('operator')
    } catch { /* 错误已由上层 toast */ }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader><DialogTitle>新建用户</DialogTitle></DialogHeader>
        <form onSubmit={submit(onFinish)} className="space-y-4 py-2">
          <div className="space-y-2"><Label htmlFor="new-username">用户名</Label><Input id="new-username" value={values.username} onChange={(e) => set('username', e.target.value)} placeholder="用于登录，3-32 位字母/数字/下划线" autoComplete="off" /><FieldError msg={errors.username} /></div>
          <div className="space-y-2"><Label htmlFor="new-password">密码</Label><Input id="new-password" type="password" value={values.password} onChange={(e) => set('password', e.target.value)} placeholder="至少 6 位" autoComplete="new-password" /><FieldError msg={errors.password} /></div>
          <div className="space-y-2"><Label htmlFor="new-display-name">显示名称</Label><Input id="new-display-name" value={values.display_name} onChange={(e) => set('display_name', e.target.value)} placeholder="中文名或昵称" /><FieldError msg={errors.display_name} /></div>
          <div className="space-y-2"><Label>角色</Label><Select value={role} onValueChange={(v: string) => setRole(v as 'admin' | 'operator')}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="operator">普通用户</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select></div>
          <DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button type="submit">创建</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export function EditDialog({ user, isSelf, open, onOpenChange, onSubmit }: {
  user: UserType; isSelf: boolean; open: boolean; onOpenChange: (v: boolean) => void
  onSubmit: (data: { display_name: string; role: string; password?: string }) => void
}) {
  const [role, setRole] = useState(user.role)
  const { values, errors, set, submit } = useFormState<UserEditValues>({ display_name: user.display_name, password: '' }, validateUserEdit)
  const onFinish = async (v: UserEditValues) => onSubmit({ display_name: v.display_name.trim(), role, password: v.password.trim() || undefined })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}><DialogContent onInteractOutside={(e) => e.preventDefault()}><DialogHeader><DialogTitle>编辑用户</DialogTitle></DialogHeader>
      <form onSubmit={submit(onFinish)} className="space-y-4 py-2"><div className="space-y-2"><Label>用户名</Label><Input value={user.username} disabled /></div><div className="space-y-2"><Label htmlFor="edit-display-name">显示名称</Label><Input id="edit-display-name" value={values.display_name} onChange={(e) => set('display_name', e.target.value)} /><FieldError msg={errors.display_name} /></div><div className="space-y-2"><Label>角色</Label><Select value={role} onValueChange={(v: string) => setRole(v as 'admin' | 'operator')} disabled={isSelf}><SelectTrigger className={isSelf ? 'opacity-60' : undefined}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="operator">普通用户</SelectItem><SelectItem value="admin">管理员</SelectItem></SelectContent></Select>{isSelf && <p className="text-xs text-muted-foreground">不能修改自己的角色，可修改显示名和密码</p>}</div><div className="space-y-2"><Label htmlFor="edit-password">新密码（留空则不修改）</Label><Input id="edit-password" type="password" value={values.password} onChange={(e) => set('password', e.target.value)} placeholder="留空则不修改" autoComplete="new-password" /><FieldError msg={errors.password} /></div><DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button type="submit">保存</Button></DialogFooter></form>
    </DialogContent></Dialog>
  )
}
