import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Shield } from 'lucide-react'
import { fetchUsers, createUser, updateUser, deleteUser, type User as UserType } from '../../api/auth'
import { useAuth } from '@/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
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
import { DataTable, type Column } from '@/components/Table'
import DeleteConfirm from '@/components/DeleteConfirm'
import {
  useFormState,
  validateUserCreate,
  validateUserEdit,
  type UserCreateValues,
  type UserEditValues,
} from '@/lib/validation'

export default function Users() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { user: currentUser, logout } = useAuth()
  const [showCreate, setShowCreate] = useState(false)
  const [editing, setEditing] = useState<UserType | null>(null)

  const { data: users = [], isLoading } = useQuery({
    queryKey: ['users'],
    queryFn: fetchUsers,
  })

  const handleCreate = async (data: {
    username: string
    password: string
    display_name: string
    role: string
  }) => {
    try {
      await createUser(data)
      toast.success('用户创建成功')
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setShowCreate(false)
    } catch (e: any) {
      toast.error(e?.message ?? '创建失败')
    }
  }

  const handleUpdate = async (data: {
    display_name: string
    role: string
    password?: string
  }) => {
    if (!editing) return
    const changesOwnPassword = editing.id === currentUser?.id && !!data.password
    try {
      await updateUser(editing.id, data)
      if (changesOwnPassword) {
        // 后端已吊销该用户全部会话：本地登出并回到登录页
        await logout()
        toast.success('密码已修改，请重新登录')
        navigate('/login', { replace: true })
        return
      }
      toast.success('更新成功')
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setEditing(null)
    } catch (e: any) {
      toast.error(e?.message ?? '更新失败')
    }
  }

  const handleDelete = async (id: number) => {
    try {
      await deleteUser(id)
      toast.success('删除成功')
      queryClient.invalidateQueries({ queryKey: ['users'] })
    } catch (e: any) {
      toast.error(e?.message ?? '删除失败')
    }
  }

  // 与 Categories 一致：实体管理统一使用 DataTable 呈现
  const columns: Column<UserType>[] = [
    {
      title: '用户',
      key: 'display_name',
      render: (u) => (
        <div className="flex items-center gap-2">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-blue-500 to-indigo-600 text-xs font-medium text-white">
            {u.display_name.charAt(0)}
          </div>
          <span className="font-medium">{u.display_name}</span>
          <span className="text-sm text-muted-foreground">@{u.username}</span>
          {u.id === currentUser?.id && (
            <Badge variant="outline" className="text-xs">我</Badge>
          )}
        </div>
      ),
    },
    {
      title: '角色',
      key: 'role',
      width: 120,
      render: (u) =>
        u.role === 'admin' ? (
          <Badge variant="secondary" className="gap-1">
            <Shield className="size-3" /> 管理员
          </Badge>
        ) : (
          <span className="text-muted-foreground">普通用户</span>
        ),
    },
    { title: '创建时间', key: 'created_at', width: 180 },
    {
      title: '操作',
      key: 'action',
      width: 110,
      render: (u) => (
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => setEditing(u)}>
            <Pencil /> 编辑
          </Button>
          <DeleteConfirm
            title="确认删除"
            description={`确定要删除用户 "${u.display_name}" 吗？`}
            trigger={
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive"
                disabled={u.id === currentUser?.id}
              >
                <Trash2 />
              </Button>
            }
            onConfirm={() => handleDelete(u.id)}
          />
        </div>
      ),
    },
  ]

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">用户管理</h1>
        <Button onClick={() => setShowCreate(true)}>
          <Plus className="size-4" /> 新建用户
        </Button>
      </div>

      <DataTable<UserType>
        columns={columns}
        dataSource={users}
        rowKey={(u) => u.id}
        loading={isLoading}
        empty="还没有用户"
      />

      <CreateDialog open={showCreate} onOpenChange={setShowCreate} onSubmit={handleCreate} />
      {editing && (
        <EditDialog
          user={editing}
          isSelf={editing.id === currentUser?.id}
          open={!!editing}
          onOpenChange={(v) => !v && setEditing(null)}
          onSubmit={handleUpdate}
        />
      )}
    </div>
  )
}

// 表单校验错误行内提示（与全站风格一致；toast 仅用于操作结果）
function FieldError({ msg }: { msg?: string }) {
  if (!msg) return null
  return <p className="text-sm text-destructive">{msg}</p>
}

function CreateDialog({
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
    { username: '', password: '', display_name: '' },
    validateUserCreate,
  )

  const onFinish = async (v: UserCreateValues) => {
    try {
      await onSubmit({ username: v.username.trim(), password: v.password, display_name: v.display_name.trim(), role })
      reset()
      setRole('operator')
    } catch {
      /* 错误已由上层 toast */
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>新建用户</DialogTitle>
        </DialogHeader>
        <form onSubmit={submit(onFinish)} className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="new-username">用户名</Label>
            <Input
              id="new-username"
              value={values.username}
              onChange={(e) => set('username', e.target.value)}
              placeholder="用于登录，3-32 位字母/数字/下划线"
              autoComplete="off"
            />
            <FieldError msg={errors.username} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-password">密码</Label>
            <Input
              id="new-password"
              type="password"
              value={values.password}
              onChange={(e) => set('password', e.target.value)}
              placeholder="至少 6 位"
              autoComplete="new-password"
            />
            <FieldError msg={errors.password} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-display-name">显示名称</Label>
            <Input
              id="new-display-name"
              value={values.display_name}
              onChange={(e) => set('display_name', e.target.value)}
              placeholder="中文名或昵称"
            />
            <FieldError msg={errors.display_name} />
          </div>
          <div className="space-y-2">
            <Label>角色</Label>
            <Select value={role} onValueChange={(v: string) => setRole(v as 'admin' | 'operator')}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="operator">普通用户</SelectItem>
                <SelectItem value="admin">管理员</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit">创建</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function EditDialog({
  user,
  isSelf,
  open,
  onOpenChange,
  onSubmit,
}: {
  user: UserType
  isSelf: boolean
  open: boolean
  onOpenChange: (v: boolean) => void
  onSubmit: (data: { display_name: string; role: string; password?: string }) => void
}) {
  const [role, setRole] = useState(user.role)

  const { values, errors, set, submit } = useFormState<UserEditValues>(
    { display_name: user.display_name, password: '' },
    validateUserEdit,
  )

  const onFinish = async (v: UserEditValues) => {
    await onSubmit({
      display_name: v.display_name.trim(),
      role,
      password: v.password.trim() || undefined,
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>编辑用户</DialogTitle>
        </DialogHeader>
        <form onSubmit={submit(onFinish)} className="space-y-4 py-2">
          <div className="space-y-2">
            <Label>用户名</Label>
            <Input value={user.username} disabled />
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit-display-name">显示名称</Label>
            <Input
              id="edit-display-name"
              value={values.display_name}
              onChange={(e) => set('display_name', e.target.value)}
            />
            <FieldError msg={errors.display_name} />
          </div>
          <div className="space-y-2">
            <Label>角色</Label>
            <Select
              value={role}
              onValueChange={(v: string) => setRole(v as 'admin' | 'operator')}
              disabled={isSelf}
            >
              <SelectTrigger className={isSelf ? 'opacity-60' : undefined}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="operator">普通用户</SelectItem>
                <SelectItem value="admin">管理员</SelectItem>
              </SelectContent>
            </Select>
            {isSelf && (
              <p className="text-xs text-muted-foreground">不能修改自己的角色，可修改显示名和密码</p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit-password">新密码（留空则不修改）</Label>
            <Input
              id="edit-password"
              type="password"
              value={values.password}
              onChange={(e) => set('password', e.target.value)}
              placeholder="留空则不修改"
              autoComplete="new-password"
            />
            <FieldError msg={errors.password} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit">保存</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
