import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import {
  CheckCircle2,
  ListChecks,
  Search,
  ChevronDown,
  ChevronUp,
  MessageSquare,
  Inbox,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  useFormState,
  validateTicketInput,
  type TicketInputValues,
} from '@/lib/validation'
import {
  submitTicket,
  ticketNumber,
  fetchMyTickets,
  fetchMyTicketDetail,
  type Ticket,
  type GuestTicketDetail,
} from '@/api/tickets'
import { fetchSubmitCategories } from '@/api/categories'
import StatusBadge from '@/components/StatusBadge'

const fallbackCategories = ['硬件故障', '软件问题', '网络问题', '打印机故障', '其他']

type Tab = 'submit' | 'track'

export default function Submit() {
  const [tab, setTab] = useState<Tab>('submit')
  const [submitted, setSubmitted] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [categories, setCategories] = useState<string[]>(fallbackCategories)
  // 记录刚提交的手机号，供「查看我的报修 / 查询历史工单」预填
  const [lastPhone, setLastPhone] = useState('')

  const { values, errors, set, getValues, reset, submit } = useFormState<TicketInputValues>(
    { category: '软件问题', name: '', phone: '', content: '' },
    validateTicketInput,
  )

  useEffect(() => {
    let alive = true
    fetchSubmitCategories()
      .then((items) => {
        if (!alive || items.length === 0) return
        setCategories(items)
        // 默认分类跟随服务端已启用分类：避免「软件问题」被停用后仍以旧分类提交被拒
        if (!items.includes(getValues().category)) set('category', items[0])
      })
      .catch(() => {
        /* 失败时使用默认分类 */
      })
    return () => {
      alive = false
    }
  }, [getValues, set])

  const onFinish = async (values: TicketInputValues) => {
    setSubmitting(true)
    try {
      // 姓名与手机号分别落库；手机号是进度查询的凭据
      await submitTicket({
        category: values.category,
        content: values.content,
        name: values.name.trim(),
        phone: values.phone.trim(),
      })
      setLastPhone(values.phone.trim())
      setSubmitted(true)
    } catch (e: any) {
      toast.error(e?.message ?? '提交失败')
    } finally {
      setSubmitting(false)
    }
  }

  const goTrack = (phone: string) => {
    setSubmitted(false)
    setTab('track')
    setTrackQuery(phone)
    setSearchPhone(phone)
    // 清空上一轮结果；带手机号进入时直接查询
    setTickets(null)
    setOpenId(null)
    setDetail(null)
    if (phone) void queryByPhone(phone)
  }

  // ---------------- 进度查询 ----------------
  const [trackQuery, setTrackQuery] = useState('')
  const [searchPhone, setSearchPhone] = useState('') // 已提交查询的手机号（结果归属）
  const [querying, setQuerying] = useState(false)
  const [tickets, setTickets] = useState<Ticket[] | null>(null)
  const [openId, setOpenId] = useState<number | null>(null)
  const [detail, setDetail] = useState<GuestTicketDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  async function queryByPhone(phone: string) {
    setQuerying(true)
    setOpenId(null)
    setDetail(null)
    try {
      const items = await fetchMyTickets(phone)
      setSearchPhone(phone)
      setTickets(items)
    } catch (e: any) {
      toast.error(e?.message ?? '查询失败')
    } finally {
      setQuerying(false)
    }
  }

  const onQuery = () => {
    const phone = trackQuery.trim()
    if (!/^1[3-9]\d{9}$/.test(phone)) {
      toast.error('请输入正确的 11 位手机号')
      return
    }
    void queryByPhone(phone)
  }

  const toggleDetail = async (id: number) => {
    if (openId === id) {
      setOpenId(null)
      setDetail(null)
      return
    }
    setOpenId(id)
    setDetail(null)
    setDetailLoading(true)
    try {
      setDetail(await fetchMyTicketDetail(id, searchPhone))
    } catch (e: any) {
      toast.error(e?.message ?? '查询失败')
      setOpenId(null)
    } finally {
      setDetailLoading(false)
    }
  }

  if (submitted) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-muted/40 p-4">
        <Card className="w-full max-w-sm shadow-lg">
          <CardContent className="flex flex-col items-center gap-3 pt-8 text-center">
            <CheckCircle2 className="size-12 text-emerald-500" />
            <h2 className="text-lg font-semibold">提交成功</h2>
            <p className="text-sm text-muted-foreground">
              工单已提交，我们会尽快处理。
              <br />
              可随时凭手机号 <span className="font-medium text-foreground">{lastPhone}</span> 查询进度
            </p>
            <div className="mt-2 flex w-full flex-col gap-2">
              <Button onClick={() => goTrack(lastPhone)}>查看我的报修</Button>
              <Button
                variant="outline"
                onClick={() => {
                  reset()
                  setSubmitted(false)
                  setTab('submit')
                }}
              >
                再提交一条
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    )
  }

  const tabBtn = (t: Tab, label: string) => (
    <button
      key={t}
      type="button"
      onClick={() => setTab(t)}
      className={
        tab === t
          ? 'rounded-md bg-background py-1.5 text-sm font-medium shadow-sm'
          : 'rounded-md py-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground'
      }
    >
      {label}
    </button>
  )

  return (
    <div className="flex min-h-screen items-start justify-center bg-muted/40 p-4 sm:items-center">
      <Card className="w-full max-w-md shadow-lg">
        <CardHeader className="text-center">
          <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground">
            <ListChecks className="size-6" />
          </div>
          <CardTitle className="text-xl">工单服务</CardTitle>
          <CardDescription>提交问题或查询处理进度</CardDescription>
          {/* 分栏切换：提交报修 / 进度查询 */}
          <div className="mt-3 grid grid-cols-2 gap-1 rounded-lg bg-muted p-1">
            {tabBtn('submit', '提交报修')}
            {tabBtn('track', '进度查询')}
          </div>
        </CardHeader>
        <CardContent>
          {tab === 'submit' ? (
            <form onSubmit={submit(onFinish)} className="space-y-4">
              <div className="space-y-2">
                <Label>分类</Label>
                <Select value={values.category} onValueChange={(v) => set('category', v)}>
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
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="g-name">姓名</Label>
                  <Input
                    id="g-name"
                    placeholder="您的姓名"
                    value={values.name}
                    onChange={(e) => set('name', e.target.value)}
                  />
                  {errors.name && (
                    <p className="text-sm text-destructive">{errors.name}</p>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="g-phone">手机号</Label>
                  <Input
                    id="g-phone"
                    type="tel"
                    inputMode="numeric"
                    maxLength={11}
                    placeholder="11 位手机号"
                    value={values.phone}
                    onChange={(e) => set('phone', e.target.value.replace(/\D/g, ''))}
                  />
                  {errors.phone && (
                    <p className="text-sm text-destructive">{errors.phone}</p>
                  )}
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="content">问题描述</Label>
                <Textarea
                  id="content"
                  rows={4}
                  placeholder="请描述问题…"
                  value={values.content}
                  onChange={(e) => set('content', e.target.value)}
                />
                {errors.content && (
                  <p className="text-sm text-destructive">{errors.content}</p>
                )}
              </div>
              <Button type="submit" className="w-full" size="lg" loading={submitting}>
                提交
              </Button>
              <p className="text-center text-xs text-muted-foreground">
                提交后可凭手机号在「进度查询」中查看处理进度与处理人留言
              </p>
            </form>
          ) : (
            <div className="space-y-4">
              {/* 查询条件：提交时填写的手机号 */}
              <form
                onSubmit={(e) => {
                  e.preventDefault()
                  onQuery()
                }}
                className="space-y-3"
              >
                <div className="space-y-2">
                  <Label htmlFor="track-phone">手机号</Label>
                  <div className="flex gap-2">
                    <Input
                      id="track-phone"
                      type="tel"
                      inputMode="numeric"
                      maxLength={11}
                      placeholder="填写提交报修时使用的手机号"
                      value={trackQuery}
                      onChange={(e) => setTrackQuery(e.target.value.replace(/\D/g, ''))}
                      enterKeyHint="search"
                    />
                    <Button type="submit" loading={querying}>
                      {!querying && <Search />}
                      查询
                    </Button>
                  </div>
                </div>
              </form>

              {tickets !== null && (
                <>
                  <p className="text-xs text-muted-foreground">
                    共 {tickets.length} 条记录{tickets.length > 0 && `（${searchPhone}）`}
                  </p>

                  {tickets.length === 0 ? (
                    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-8 text-muted-foreground">
                      <Inbox className="size-8" />
                      <p className="text-sm">没有找到相关工单</p>
                      <p className="text-xs">请确认与提交报修时填写的手机号一致</p>
                    </div>
                  ) : (
                    <ul className="space-y-2">
                      {tickets.map((t) => (
                        <li key={t.id} className="rounded-lg border bg-background">
                          {/* 卡片头：编号 + 状态，点击展开处理记录 */}
                          <button
                            type="button"
                            onClick={() => toggleDetail(t.id)}
                            aria-expanded={openId === t.id}
                            className="flex w-full cursor-pointer flex-col gap-2 p-3 text-left"
                          >
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-mono text-sm">{ticketNumber(t)}</span>
                              <StatusBadge status={t.status} />
                            </div>
                            <Badge variant="secondary" className="w-fit">
                              {t.category}
                            </Badge>
                            <p className="line-clamp-2 text-sm text-muted-foreground">{t.content}</p>
                            <div className="flex items-center justify-between text-xs text-muted-foreground">
                              <span>提交于 {t.created_at}</span>
                              {openId === t.id ? (
                                <span className="flex items-center gap-0.5">
                                  收起 <ChevronUp className="size-3.5" />
                                </span>
                              ) : (
                                <span className="flex items-center gap-0.5">
                                  处理记录 <ChevronDown className="size-3.5" />
                                </span>
                              )}
                            </div>
                          </button>

                          {/* 展开区：负责人 + 处理记录时间线 */}
                          {openId === t.id && (
                            <div className="border-t px-3 pb-3 pt-2">
                              {detailLoading ? (
                                <p className="py-3 text-center text-sm text-muted-foreground">加载中…</p>
                              ) : detail ? (
                                <div className="space-y-3">
                                  <p className="text-xs text-muted-foreground">
                                    负责人：
                                    {detail.ticket.assignee ? (
                                      <span className="text-foreground">{detail.ticket.assignee}</span>
                                    ) : detail.ticket.status === 1 ? (
                                      '—'
                                    ) : (
                                      '待指派'
                                    )}
                                  </p>
                                  {detail.comments.length === 0 ? (
                                    <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
                                      <MessageSquare className="size-3.5" />
                                      {detail.ticket.status === 1
                                        ? '该工单已完成处理'
                                        : '暂无处理记录，我们会尽快跟进'}
                                    </p>
                                  ) : (
                                    <ol className="space-y-3 border-l pl-3">
                                      {detail.comments.map((c) => (
                                        <li key={c.id} className="relative">
                                          <span className="absolute -left-[19px] top-1.5 size-2 rounded-full bg-primary ring-4 ring-background" />
                                          <p className="text-xs text-muted-foreground">
                                            {c.author} · {c.created_at}
                                          </p>
                                          <p className="text-sm">{c.content}</p>
                                        </li>
                                      ))}
                                    </ol>
                                  )}
                                </div>
                              ) : null}
                            </div>
                          )}
                        </li>
                      ))}
                    </ul>
                  )}
                </>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
