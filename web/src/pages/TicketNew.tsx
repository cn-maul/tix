import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
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
import { ticketSchema, type TicketFormValues } from '@/lib/validation'

const fallbackCats = ['硬件故障', '软件问题', '网络问题', '打印机故障', '其他']

export default function TicketNew() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const editId = searchParams.get('edit') ? Number(searchParams.get('edit')) : null
  const queryClient = useQueryClient()
  const [saving, setSaving] = useState(false)
  const [categories, setCategories] = useState<string[]>([])

  const {
    register,
    handleSubmit,
    setValue,
    reset,
    watch,
    formState: { errors },
  } = useForm<TicketFormValues>({ resolver: zodResolver(ticketSchema) })

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
        .then((t) => reset({ category: t.category, creator: t.creator, content: t.content }))
        .catch(() => toast.error('加载工单失败'))
    }
    return () => {
      alive = false
    }
  }, [editId, reset])

  const onSubmit = async (values: TicketFormValues) => {
    setSaving(true)
    try {
      if (editId) {
        await updateTicket(editId, values)
        toast.success('已更新')
        navigate(`/tickets/${editId}`)
      } else {
        const t = await createTicket(values)
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

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4 rounded-xl border bg-card p-6 shadow-sm">
        <div className="space-y-2">
          <Label>分类</Label>
          <Select value={watch('category') || undefined} onValueChange={(v) => setValue('category', v)}>
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
          {errors.category && <p className="text-sm text-destructive">{errors.category.message}</p>}
        </div>

        <div className="space-y-2">
          <Label htmlFor="creator">发起人</Label>
          <Input id="creator" placeholder="姓名 / 手机号" {...register('creator')} />
          {errors.creator && <p className="text-sm text-destructive">{errors.creator.message}</p>}
        </div>

        <div className="space-y-2">
          <Label htmlFor="content">内容</Label>
          <Textarea id="content" rows={4} placeholder="请描述问题…" {...register('content')} />
          {errors.content && <p className="text-sm text-destructive">{errors.content.message}</p>}
        </div>

        <Button type="submit" loading={saving}>
          {editId ? '保存' : '提交'}
        </Button>
      </form>
    </div>
  )
}