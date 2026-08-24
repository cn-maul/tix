import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowLeft } from 'lucide-react'
import { createTicket, updateTicket, fetchOne } from '../api/tickets'
import { fetchCategories } from '../api/categories'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useFormState, validateTicketInput, type TicketInputValues } from '@/lib/validation'

const fallbackCats = ['硬件故障', '软件问题', '网络问题', '打印机故障', '其他']

export default function TicketNew() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const editId = searchParams.get('edit') ? Number(searchParams.get('edit')) : null
  const queryClient = useQueryClient()
  const [saving, setSaving] = useState(false)
  const [categories, setCategories] = useState<string[]>([])

  const { values, errors, set, reset, submit } = useFormState<TicketInputValues>(
    { category: '', name: '', phone: '', content: '' },
    validateTicketInput,
  )

  useEffect(() => {
    let alive = true
    fetchCategories()
      .then((items) => {
        if (alive) setCategories(items.filter((c) => c.enabled === 1).map((c) => c.name))
      })
      .catch(() => {
        /* 分类加载失败时回退到默认列表 */
      })
    if (editId) {
      fetchOne(editId)
        .then((t) => reset({ category: t.category, name: t.creator, phone: t.phone, content: t.content }))
        .catch(() => toast.error('加载工单失败'))
    }
    return () => {
      alive = false
    }
  }, [editId, reset])

  const onSubmit = async (values: TicketInputValues) => {
    setSaving(true)
    try {
      const payload = {
        category: values.category,
        content: values.content,
        name: values.name.trim(),
        phone: values.phone.trim(),
      }
      if (editId) {
        await updateTicket(editId, payload)
        toast.success('已更新')
        navigate(`/tickets/${editId}`)
      } else {
        const t = await createTicket(payload)
        toast.success('已提交')
        navigate(`/tickets/${t.id}`)
      }
      queryClient.invalidateQueries({ queryKey: ['tickets'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setSaving(false)
    }
  }

  const catList = categories.length > 0 ? categories : fallbackCats

  return (
    <div className="mx-auto max-w-xl space-y-4">
      <Button variant="ghost" size="sm" onClick={() => navigate(-1)}>
        <ArrowLeft /> 返回
      </Button>
      <h1 className="text-xl font-semibold">{editId ? '编辑工单' : '新建工单'}</h1>

      <form onSubmit={submit(onSubmit)} className="space-y-4 rounded-xl border bg-card p-6 shadow-sm">
        <div className="space-y-2">
          <Label>分类</Label>
          <Select value={values.category || undefined} onValueChange={(v) => set('category', v)}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="选择分类" />
            </SelectTrigger>
            <SelectContent>
              {catList.map((c) => (
                <SelectItem key={c} value={c}>
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors.category && <p className="text-sm text-destructive">{errors.category}</p>}
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="name">姓名</Label>
            <Input
              id="name"
              placeholder="发起人姓名"
              value={values.name}
              onChange={(e) => set('name', e.target.value)}
            />
            {errors.name && <p className="text-sm text-destructive">{errors.name}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="phone">手机号</Label>
            <Input
              id="phone"
              type="tel"
              inputMode="numeric"
              maxLength={11}
              placeholder="11 位手机号"
              value={values.phone}
              onChange={(e) => set('phone', e.target.value.replace(/\D/g, ''))}
            />
            {errors.phone && <p className="text-sm text-destructive">{errors.phone}</p>}
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="content">内容</Label>
          <Textarea
            id="content"
            rows={4}
            placeholder="请描述问题…"
            value={values.content}
            onChange={(e) => set('content', e.target.value)}
          />
          {errors.content && <p className="text-sm text-destructive">{errors.content}</p>}
        </div>

        <Button type="submit" loading={saving}>
          {editId ? '保存' : '提交'}
        </Button>
      </form>
    </div>
  )
}