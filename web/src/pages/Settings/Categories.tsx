import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import {
  fetchCategories,
  createCategory,
  updateCategory,
  deleteCategory,
  type Category,
} from '../../api/categories'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DataTable, type Column } from '@/components/Table'
import DeleteConfirm from '@/components/DeleteConfirm'
import { useFormState, validateCategory, type CategoryFormValues } from '@/lib/validation'

const emptyCategory: CategoryFormValues = { name: '', color: '#2563eb', sort: 0, enabled: true }

export default function Categories() {
  const [modal, setModal] = useState<{ open: boolean; item?: Category }>({ open: false })
  const queryClient = useQueryClient()

  const { data: categories = [], isLoading: loading } = useQuery({
    queryKey: ['categories'],
    queryFn: fetchCategories,
  })

  const createMutation = useMutation({
    mutationFn: createCategory,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['categories'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      toast.success('已创建')
      setModal({ open: false })
    },
    onError: (e: any) => toast.error(e.message || '创建失败'),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Category> }) => updateCategory(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['categories'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      toast.success('已更新')
      setModal({ open: false })
    },
    onError: (e: any) => toast.error(e.message || '更新失败'),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteCategory,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['categories'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
      toast.success('已删除')
    },
    onError: (e: any) => toast.error(e.message || '删除失败'),
  })

  const { values, errors, set, reset, submit } = useFormState<CategoryFormValues>(
    emptyCategory,
    validateCategory,
  )

  const openModal = (item?: Category) => {
    if (item) {
      reset({
        name: item.name,
        color: item.color || '#2563eb',
        sort: item.sort,
        enabled: item.enabled === 1,
      })
    } else {
      reset({ name: '', color: '#2563eb', sort: 0, enabled: true })
    }
    setModal({ open: true, item })
  }

  const onToggle = (r: Category, checked: boolean) => {
    updateMutation.mutate({
      id: r.id,
      data: {
        name: r.name,
        color: r.color || '#2563eb',
        sort: r.sort,
        enabled: checked ? 1 : 0,
      },
    })
  }

  const onFinish = (values: CategoryFormValues) => {
    const payload = { name: values.name, color: values.color, sort: values.sort, enabled: values.enabled ? 1 : 0 }
    if (modal.item) {
      updateMutation.mutate({ id: modal.item.id, data: payload })
    } else {
      createMutation.mutate(payload)
    }
  }

  const columns: Column<Category>[] = [
    {
      title: '名称',
      key: 'name',
      render: (r) => (
        <Badge variant="secondary" style={{ backgroundColor: r.color, color: '#fff' }}>
          {r.name}
        </Badge>
      ),
    },
    {
      title: '颜色',
      key: 'color',
      width: 120,
      render: (r) => (
        <span className="flex items-center gap-2">
          <span className="size-4 rounded-full border" style={{ backgroundColor: r.color }} />
          <span className="font-mono text-xs">{r.color}</span>
        </span>
      ),
    },
    { title: '排序', key: 'sort', width: 80 },
    {
      title: '启用',
      key: 'enabled',
      width: 100,
      render: (r) => (
        <Switch
          checked={r.enabled === 1}
          onCheckedChange={(c) => onToggle(r, c)}
          disabled={updateMutation.isPending}
          aria-label={`切换 ${r.name}`}
        />
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 140,
      render: (r) => (
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => openModal(r)}>
            <Pencil /> 编辑
          </Button>
          <DeleteConfirm
            title="确认删除"
            description={`确定删除分类「${r.name}」吗？已有工单不受影响。`}
            trigger={
              <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive">
                <Trash2 />
              </Button>
            }
            onConfirm={() => deleteMutation.mutate(r.id)}
          />
        </div>
      ),
    },
  ]

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">分类管理</h1>
        <Button size="sm" onClick={() => openModal()}>
          <Plus /> 新增分类
        </Button>
      </div>

      <DataTable<Category>
        columns={columns}
        dataSource={categories}
        rowKey={(r) => r.id}
        loading={loading}
      />

      <Dialog open={modal.open} onOpenChange={(open) => setModal((m) => ({ ...m, open }))}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{modal.item ? '编辑分类' : '新增分类'}</DialogTitle>
          </DialogHeader>
          <form onSubmit={submit(onFinish)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">名称</Label>
              <Input
                id="name"
                placeholder="如：视频会议"
                value={values.name}
                onChange={(e) => set('name', e.target.value)}
              />
              {errors.name && <p className="text-sm text-destructive">{errors.name}</p>}
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="color">颜色</Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="color"
                    type="color"
                    className="size-10 w-14 p-1"
                    value={values.color}
                    onChange={(e) => set('color', e.target.value)}
                  />
                  <span className="font-mono text-xs text-muted-foreground">{values.color}</span>
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="sort">排序</Label>
                <Input
                  id="sort"
                  type="number"
                  value={values.sort}
                  onChange={(e) => set('sort', Number(e.target.value))}
                />
              </div>
            </div>
            <div className="flex items-center justify-between rounded-lg border p-3">
              <div>
                <Label htmlFor="enabled">启用</Label>
                <p className="text-xs text-muted-foreground">停用的分类不出现在工单表单中</p>
              </div>
              <Switch
                id="enabled"
                checked={values.enabled}
                onCheckedChange={(c) => set('enabled', c)}
              />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setModal({ open: false })}>
                取消
              </Button>
              <Button type="submit" loading={createMutation.isPending || updateMutation.isPending}>
                保存
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
