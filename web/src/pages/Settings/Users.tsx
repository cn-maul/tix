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
import { Card, CardContent } from '@/components/ui/card'
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
import DeleteConfirm from '@/components/DeleteConfirm'

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

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">用户管理</h1>
        <Button onClick={() => setShowCreate(true)}>
          <Plus className="size-4" /> 新建用户
        </Button>
      </div>

      {isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <div className="size-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
        </div>
      ) : (
        <div className="grid gap-3">
          {users.map((u) => (
            <Card key={u.id}>
              <CardContent className="flex items-center justify-between py-4">
                <div className="flex items-center gap-3">
                  <div className="flex size-10 items-center justify-center rounded-full bg-gradient-to-br from-blue-500 to-indigo-600 text-sm font-medium text-white">
                    {u.display_name.charAt(0)}
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{u.display_name}</span>
                      <span className="text-sm text-muted-foreground">@{u.username}</span>
                      {u.role === 'admin' && (
                        <Badge variant="secondary" className="gap-1">
                          <Shield className="size-3" /> 管理员
                        </Badge>
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      创建于 {u.created_at}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="sm" onClick={() => setEditing(u)}>
                    <Pencil className="size-4" />
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
                        <Trash2 className="size-4" />
                      </Button>
                    }
                    onConfirm={() => handleDelete(u.id)}
                  />
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

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

function CreateDialog({
  open,
  onOpenChange,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onSubmit: (data: { username: string; password: string; display_name: string; role: string }) => void
}) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [role, setRole] = useState('operator')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async () => {
    if (!username.trim() || !password.trim() || !displayName.trim()) {
      toast.error('请填写所有字段')
      return
    }
    setLoading(true)
    try {
      await onSubmit({ username: username.trim(), password, display_name: displayName.trim(), role })
      setUsername('')
      setPassword('')
      setDisplayName('')
      setRole('operator')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>新建用户</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="new-username">用户名</Label>
            <Input
              id="new-username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="用于登录"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-password">密码</Label>
            <Input
              id="new-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="登录密码"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-display-name">显示名称</Label>
            <Input
              id="new-display-name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="中文名或昵称"
            />
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
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={handleSubmit} loading={loading}>
            创建
          </Button>
        </DialogFooter>
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
  const [displayName, setDisplayName] = useState(user.display_name)
  const [role, setRole] = useState(user.role)
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async () => {
    if (!displayName.trim()) {
      toast.error('显示名称不能为空')
      return
    }
    setLoading(true)
    try {
      await onSubmit({
        display_name: displayName.trim(),
        role,
        password: password.trim() || undefined,
      })
      setPassword('')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onInteractOutside={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>编辑用户</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label>用户名</Label>
            <Input value={user.username} disabled />
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit-display-name">显示名称</Label>
            <Input
              id="edit-display-name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
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
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="留空则不修改"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={handleSubmit} loading={loading}>
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
