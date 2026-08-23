import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Search, Pencil, Trash2, Eye } from 'lucide-react'
import { fetchList, deleteTicket, ticketNumber, type Ticket } from '../api/tickets'
import { fetchCategories } from '../api/categories'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DataTable, PaginationBar, type Column } from '@/components/Table'
import DeleteConfirm from '@/components/DeleteConfirm'

function StatusBadge({ status }: { status: 0 | 1 }) {
  return (
    <Badge
      variant="secondary"
      className={
        status === 0
          ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-400'
          : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400'
      }
    >
      {status === 0 ? '待处理' : '已处理'}
    </Badge>
  )
}

export default function TicketList({ status }: { status: 0 | 1 | '' }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const urlKeyword = searchParams.get('keyword') ?? ''
  const [keyword, setKeyword] = useState(urlKeyword)
  const [debounced, setDebounced] = useState(urlKeyword)
  const [category, setCategory] = useState('')
  const [page, setPage] = useState(1)
  const size = 20

  // 记录由本页写入 URL 的关键字，用于区分「自己写的」与「外部跳转」
  const syncedKeywordRef = useRef<string | null>(null)

  // 顶栏搜索跳转 /tickets?keyword=… 时同步关键字并回到第一页
  // （跳过本页防抖后自己写入的值，避免回流覆盖正在输入的文字）
  useEffect(() => {
    if (urlKeyword === syncedKeywordRef.current) return
    setKeyword(urlKeyword)
    setPage(1)
  }, [urlKeyword])

  // 关键字防抖（300ms）后再发起请求
  useEffect(() => {
    const t = setTimeout(() => setDebounced(keyword), 300)
    return () => clearTimeout(t)
  }, [keyword])

  // 防抖后的关键字同步到 URL（替换历史，方便分享/刷新保留）
  useEffect(() => {
    syncedKeywordRef.current = debounced
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if ((prev.get('keyword') ?? '') !== debounced) {
          if (debounced.trim()) next.set('keyword', debounced)
          else next.delete('keyword')
        }
        return next
      },
      { replace: true },
    )
  }, [debounced])

  // 切换状态页签时回到第一页
  useEffect(() => {
    setPage(1)
  }, [status])

  const onKeywordChange = (v: string) => {
    setKeyword(v)
    setPage(1)
  }

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['tickets', status, category, debounced, page],
    queryFn: () =>
      fetchList({
        status,
        category: category || undefined,
        keyword: debounced || undefined,
        page,
        size,
        order: status === 0 ? 'asc' : 'desc',
      }),
  })

  const title = status === 0 ? '待处理' : status === 1 ? '已处理' : '全部工单'

  const { data: categories } = useQuery({ queryKey: ['categories'], queryFn: fetchCategories })
  const catColor: Record<string, string> = useMemo(() => {
    const map: Record<string, string> = {}
    for (const c of categories ?? []) {
      if (c.color) map[c.name] = c.color
    }
    return map
  }, [categories])

  const columns: Column<Ticket>[] = useMemo(() => [
    {
      title: '编号',
      key: 'id',
      width: 140,
      render: (r: Ticket) => <span className="font-mono text-sm">{ticketNumber(r)}</span>,
    },
    {
      title: '分类',
      key: 'category',
      width: 110,
      render: (r: Ticket) => (
        <Badge variant="secondary" style={{ backgroundColor: catColor[r.category], color: '#fff' }}>
          {r.category}
        </Badge>
      ),
    },
    {
      title: '发起人',
      key: 'creator',
      width: 170,
      render: (r: Ticket) => (
        <span className="block max-w-[170px] truncate" title={r.creator}>
          {r.creator}
        </span>
      ),
    },
    {
      title: '内容',
      key: 'content',
      render: (r: Ticket) => (
        <span className="block max-w-[320px] truncate text-muted-foreground" title={r.content}>
          {r.content}
        </span>
      ),
    },
    ...(status === ''
      ? [
          {
            title: '状态',
            key: 'status',
            width: 90,
            render: (r: Ticket) => <StatusBadge status={r.status} />,
          },
        ]
      : []),
    { title: '创建时间', key: 'created_at', width: 160 },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (r: Ticket) => (
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => navigate(`/tickets/${r.id}`)}>
            <Eye /> 查看
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate(`/tickets/new?edit=${r.id}`)}
          >
            <Pencil /> 编辑
          </Button>
          <DeleteConfirm
            title="确认删除"
            description="确定要删除这条工单吗？"
            trigger={
              <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive">
                <Trash2 />
              </Button>
            }
            onConfirm={async () => {
              await deleteTicket(r.id)
              refetch()
              queryClient.invalidateQueries({ queryKey: ['stats'] })
            }}
          />
        </div>
      ),
    },
  ], [status, catColor, navigate, refetch, queryClient])

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">{title}</h1>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={category || 'all'}
            onValueChange={(v) => {
              setCategory(v === 'all' ? '' : v)
              setPage(1)
            }}
          >
            <SelectTrigger className="w-36">
              <SelectValue placeholder="全部分类" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部分类</SelectItem>
              {(categories ?? []).map((c) => (
                <SelectItem key={c.id} value={c.name}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="w-56 pl-8"
              placeholder="搜索内容 / 发起人"
              value={keyword}
              onChange={(e) => onKeywordChange(e.target.value)}
            />
          </div>
        </div>
      </div>

      <DataTable<Ticket>
        columns={columns}
        dataSource={data?.items ?? []}
        rowKey={(r) => r.id}
        loading={isLoading}
        empty="没有符合条件的工单"
      />

      <PaginationBar
        page={page}
        pageSize={size}
        total={data?.total ?? 0}
        onChange={setPage}
      />
    </div>
  )
}