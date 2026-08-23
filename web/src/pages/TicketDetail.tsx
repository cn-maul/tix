import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useParams, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeft, CheckCircle2, Pencil, Trash2, MessageSquarePlus } from 'lucide-react'
import {
  fetchOne,
  deleteTicket,
  markDone,
  ticketNumber,
  addComment,
  fetchComments,
} from '../api/tickets'
import { useAuth } from '@/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import DeleteConfirm from '@/components/DeleteConfirm'

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-0.5">{children}</dd>
    </div>
  )
}

export default function TicketDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { user } = useAuth()
  const numId = Number(id)
  const [note, setNote] = useState('')
  const [comment, setComment] = useState('')
  const authorName = user?.display_name || '匿名'

  const { data: ticket, isLoading: loadingT } = useQuery({
    queryKey: ['ticket', numId],
    queryFn: () => fetchOne(numId),
  })
  const { data: comments, isLoading: loadingC, refetch: refetchC } = useQuery({
    queryKey: ['comments', numId],
    queryFn: () => fetchComments(numId),
    enabled: !!numId,
  })

  if (loadingT) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
      </div>
    )
  }
  if (!ticket) return <div className="text-muted-foreground">工单不存在</div>

  const onDone = async () => {
    const noteText = note.trim()
    await markDone(ticket.id, noteText || undefined, noteText ? authorName : undefined)
    setNote('')
    queryClient.invalidateQueries({ queryKey: ['stats'] })
    toast.success('已标记处理')
    navigate('/tickets/done')
  }

  const onComment = async () => {
    if (!comment.trim()) return
    await addComment(ticket.id, authorName, comment.trim())
    setComment('')
    refetchC()
    toast.success('已添加备注')
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="ghost" size="sm" onClick={() => navigate(-1)}>
            <ArrowLeft /> 返回
          </Button>
          <h1 className="font-mono text-xl font-semibold">{ticketNumber(ticket)}</h1>
          <Badge
            variant="secondary"
            className={
              ticket.status === 0
                ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-400'
                : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400'
            }
          >
            {ticket.status === 0 ? '待处理' : '已处理'}
          </Badge>
        </div>
        <div className="flex items-center gap-2">
          {ticket.status === 0 && (
            <Button onClick={onDone}>
              <CheckCircle2 /> 标记已处理
            </Button>
          )}
          <Button variant="outline" onClick={() => navigate(`/tickets/new?edit=${ticket.id}`)}>
            <Pencil /> 编辑
          </Button>
          <DeleteConfirm
            title="确认删除"
            description="确定要删除这条工单吗？"
            trigger={
              <Button variant="outline" className="text-destructive hover:text-destructive">
                <Trash2 />
              </Button>
            }
            onConfirm={async () => {
              await deleteTicket(ticket.id)
              queryClient.invalidateQueries({ queryKey: ['stats'] })
              toast.success('已删除')
              navigate('/tickets')
            }}
          />
        </div>
      </div>

      <Card>
        <CardContent className="pt-6">
          <dl className="grid grid-cols-1 gap-x-8 gap-y-4 sm:grid-cols-2">
            <Field label="分类">
              <Badge variant="secondary">{ticket.category}</Badge>
            </Field>
            <Field label="发起人">{ticket.creator}</Field>
            <Field label="创建时间">{ticket.created_at}</Field>
            <Field label="更新时间">{ticket.updated_at}</Field>
            <div className="sm:col-span-2">
              <Field label="内容">
                <p className="whitespace-pre-wrap text-muted-foreground">{ticket.content}</p>
              </Field>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>处理记录 / 备注</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {ticket.status === 0 && (
            <div className="rounded-lg bg-muted/60 p-4">
              <p className="mb-2 text-sm text-muted-foreground">标记已处理时可附解决备注：</p>
              <div className="flex flex-wrap gap-2">
                <Input
                  className="max-w-xs"
                  placeholder="解决备注（可选）"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                />
                <Button onClick={onDone}>标记已处理</Button>
              </div>
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            <Input
              className="max-w-sm"
              placeholder="添加备注…"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') onComment()
              }}
            />
            <Button variant="outline" onClick={onComment}>
              <MessageSquarePlus /> 添加
            </Button>
          </div>

          <div className="space-y-0">
            {loadingC ? (
              <div className="flex justify-center py-8">
                <div className="size-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
              </div>
            ) : !comments || comments.length === 0 ? (
              <p className="py-4 text-center text-sm text-muted-foreground">暂无处理记录</p>
            ) : (
              <div className="space-y-0">
                {comments.map((c) => (
                  <div key={c.id} className="relative border-l pb-6 pl-5 last:pb-0">
                    <span className="absolute -left-[5px] top-1 size-2.5 rounded-full bg-primary ring-4 ring-background" />
                    <div className="flex flex-wrap items-center gap-2 text-sm">
                      <span className="font-medium">{c.author}</span>
                      <span className="text-xs text-muted-foreground">{c.created_at}</span>
                    </div>
                    <p className="mt-1 text-muted-foreground">{c.content}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}