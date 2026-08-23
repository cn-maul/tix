import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { toast } from 'sonner'
import { CheckCircle2, ListChecks } from 'lucide-react'
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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ticketSchema, type TicketFormValues } from '@/lib/validation'
import { submitTicket } from '@/api/tickets'
import { fetchSubmitCategories } from '@/api/categories'

const fallbackCategories = ['硬件故障', '软件问题', '网络问题', '打印机故障', '其他']

export default function Submit() {
  const [submitted, setSubmitted] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [categories, setCategories] = useState<string[]>(fallbackCategories)

  useEffect(() => {
    let alive = true
    fetchSubmitCategories()
      .then((items) => {
        if (alive && items.length > 0) setCategories(items)
      })
      .catch(() => {
        /* 失败时使用默认分类 */
      })
    return () => {
      alive = false
    }
  }, [])

  const {
    register,
    handleSubmit,
    setValue,
    reset,
    formState: { errors },
  } = useForm<TicketFormValues>({
    resolver: zodResolver(ticketSchema),
    defaultValues: { category: '软件问题' },
  })

  const onFinish = async (values: TicketFormValues) => {
    setSubmitting(true)
    try {
      await submitTicket(values)
      setSubmitted(true)
    } catch (e: any) {
      toast.error(e?.message ?? '提交失败')
    } finally {
      setSubmitting(false)
    }
  }

  if (submitted) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-muted/40 p-4">
        <Card className="w-full max-w-sm shadow-lg">
          <CardContent className="flex flex-col items-center gap-3 pt-8 text-center">
            <CheckCircle2 className="size-12 text-emerald-500" />
            <h2 className="text-lg font-semibold">提交成功</h2>
            <p className="text-sm text-muted-foreground">工单已提交，我们会尽快处理</p>
            <Button
              variant="outline"
              className="mt-2"
              onClick={() => {
                reset()
                setSubmitted(false)
              }}
            >
              再提交一条
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/40 p-4">
      <Card className="w-full max-w-md shadow-lg">
        <CardHeader className="text-center">
          <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
            <ListChecks className="size-6" />
          </div>
          <CardTitle className="text-xl">工单提交</CardTitle>
          <CardDescription>请填写问题信息，我们将尽快处理</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onFinish)} className="space-y-4">
            <div className="space-y-2">
              <Label>分类</Label>
              <Select
                defaultValue="软件问题"
                onValueChange={(v) => setValue('category', v)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="选择分类" />
                </SelectTrigger>
                <SelectContent>
                  {categories.map((c) => (
                    <SelectItem key={c} value={c}>
                      {c}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="creator">发起人</Label>
              <Input id="creator" placeholder="姓名 / 手机号" {...register('creator')} />
              {errors.creator && (
                <p className="text-sm text-destructive">{errors.creator.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="content">问题描述</Label>
              <Textarea id="content" rows={4} placeholder="请描述问题…" {...register('content')} />
              {errors.content && (
                <p className="text-sm text-destructive">{errors.content.message}</p>
              )}
            </div>
            <Button type="submit" className="w-full" size="lg" loading={submitting}>
              提交
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}