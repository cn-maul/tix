import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Search, Pencil, Trash2, Eye, Download, CheckCheck } from 'lucide-react'
import {
  fetchList,
  deleteTicket,
  ticketNumber,
  batchMarkDone,
  batchDeleteTickets,
  exportCsv,
  type Ticket,
} from '../api/tickets'
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
import StatusBadge from '@/components/StatusBadge'

type AssigneeFilter = 'all' | 'me' | 'unassigned'

export default function TicketList({ status }: { status: 0 | 1 | '' }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const urlKeyword = searchParams.get('keyword') ?? ''
  const [keyword, setKeyword] = useState(urlKeyword)
  const [debounced, setDebounced] = useState(urlKeyword)
  const [category, setCategory] = useState('')
  const [from, setFrom] = useState(searchParams.get('from') ?? '')
  const [to, setTo] = useState(searchParams.get('to') ?? '')
  const [assignee, setAssignee] = useState<AssigneeFilter>('all')
  const [selected, setSelected] = useState<(string | number)[]>([])
  const [exporting, setExporting] = useState(false)
  const [batchDoing, setBatchDoing] = useState(false)
  const [page, setPage] = useState(1)
  const size = 20

  // 记录由本页写入 URL 的关键字，用于区分「自己写的」与「外部跳转」
  const syncedKeywordRef = useRef<string | null>(null)

  // 顶栏搜索跳转 /tickets?keyword=… 时同步关键字并回到第一页
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

  // 筛选条件变化后清空多选（选中项可能已不在当前视图）
  useEffect(() => {
    setSelected([])
  }, [status, category, debounced, from, to, assignee, page])

  const onKeywordChange = (v: string) => {
    setKeyword(v)
    setPage(1)
  }

  const setDateRange = (nf: string, nt: string) => {
    setFrom(nf)
    setTo(nt)
    setPage(1)
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev)
        if (nf) next.set('from', nf)
        else next.delete('from')
        if (nt) next.set('to', nt)
        else next.delete('to')
        return next
      },
      { replace: true },
    )
  }

  // 当前筛选条件（列表与导出共用同一口径）
  const listParams = useMemo(
    () => ({
      status,
      category: category || undefined,
      keyword: debounced || undefined,
      from: from || undefined,
      to: to || undefined,
      assignee: assignee === 'me' ? ('me' as const) : undefined,
      unassigned: assignee === 'unassigned',
    }),
    [status, category, debounced, from, to, assignee],
  )

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['tickets', listParams, page],
    queryFn: () => fetchList({ ...listParams, page, size, order: status === 0 ? 'asc' : 'desc' }),
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

  const refreshAfterMutation = () => {
    refetch()
    queryClient.invalidateQueries({ queryKey: ['stats'] })
  }

  const onBatchDone = async () => {
    setBatchDoing(true)
    try {
      const n = await batchMarkDone(selected.map(Number))
      toast.success(`已标记 ${n} 条为已处理`)
      refreshAfterMutation()
    } catch (e: any) {
      toast.error(e?.message ?? '批量操作失败')
    } finally {
      setBatchDoing(false)
    }
  }

  const onExport = async () => {
    setExporting(true)
    try {
      const truncated = await exportCsv(listParams)
      if (truncated) toast.warning('超过 10 万条，导出已截断，请缩小筛选范围')
      else toast.success('已导出 CSV')
    } catch (e: any) {
      toast.error(e?.message ?? '导出失败')
    } finally {
      setExporting(false)
    }
  }

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
      width: 150,
      render: (r: Ticket) => (
        <div className="min-w-0">
          {/* 第一行：姓名；第二行：手机号（旧数据可能为空） */}
          <div className="max-w-[150px] truncate" title={r.creator}>
            {r.creator}
          </div>
          <div
            className="font-mono text-xs tabular-nums text-muted-foreground"
            title={r.phone || undefined}
          >
            {r.phone || '—'}
          </div>
        </div>
      ),
    },
    {
      title: '内容',
      key: 'content',
      render: (r: Ticket) => (
        <span className="block max-w-[280px] truncate text-muted-foreground" title={r.content}>
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
    {
      title: '负责人',
      key: 'assignee',
      width: 100,
      render: (r: Ticket) =>
        r.assignee ? (
          <span className="truncate" title={r.assignee}>{r.assignee}</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
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
              // 删除的是当前页最后一条且不在第一页时，回退一页避免停留在空页码
              if ((data?.items?.length ?? 0) === 1 && page > 1) setPage(page - 1)
              else refetch()
              queryClient.invalidateQueries({ queryKey: ['stats'] })
            }}
          />
        </div>
      ),
    },
  ], [status, catColor, navigate, refetch, queryClient, data, page, setPage])

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
            <SelectTrigger className="w-32">
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
          <div className="flex items-center gap-1.5">
            <Input
              type="date"
              className="w-[9.5rem]"
              aria-label="创建日期起"
              value={from}
              onChange={(e) => setDateRange(e.target.value, to)}
            />
            <span className="text-xs text-muted-foreground">至</span>
            <Input
              type="date"
              className="w-[9.5rem]"
              aria-label="创建日期止"
              value={to}
              onChange={(e) => setDateRange(from, e.target.value)}
            />
          </div>
          <Select
            value={assignee}
            onValueChange={(v) => {
              setAssignee(v as AssigneeFilter)
              setPage(1)
            }}
          >
            <SelectTrigger className="w-32">
              <SelectValue placeholder="负责人" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部负责人</SelectItem>
              <SelectItem value="unassigned">未指派</SelectItem>
              <SelectItem value="me">我负责的</SelectItem>
            </SelectContent>
          </Select>
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="w-48 pl-8"
              placeholder="搜索内容 / 发起人 / 手机号"
              value={keyword}
              onChange={(e) => onKeywordChange(e.target.value)}
            />
          </div>
          <Button variant="outline" onClick={onExport} loading={exporting}>
            {!exporting && <Download />}
            导出 CSV
          </Button>
        </div>
      </div>

      {selected.length > 0 && (
        <div className="flex flex-wrap items-center gap-3 rounded-lg border bg-muted/50 px-4 py-2.5">
          <span className="text-sm">已选 {selected.length} 条</span>
          {status !== 1 && (
            <Button size="sm" onClick={onBatchDone} loading={batchDoing}>
              {!batchDoing && <CheckCheck />}
              标记已处理
            </Button>
          )}
          <DeleteConfirm
            title="批量删除"
            description={`确定要删除选中的 ${selected.length} 条工单吗？其处理记录将一并删除。`}
            trigger={
              <Button
                variant="outline"
                size="sm"
                className="text-destructive hover:text-destructive"
              >
                <Trash2 /> 删除
              </Button>
            }
            onConfirm={async () => {
              try {
                const n = await batchDeleteTickets(selected.map(Number))
                toast.success(`已删除 ${n} 条工单`)
                refreshAfterMutation()
              } catch (e: any) {
                toast.error(e?.message ?? '批量删除失败')
              }
            }}
          />
          <Button variant="ghost" size="sm" onClick={() => setSelected([])}>
            取消选择
          </Button>
        </div>
      )}

      <DataTable<Ticket>
        columns={columns}
        dataSource={data?.items ?? []}
        rowKey={(r) => r.id}
        loading={isLoading}
        empty="没有符合条件的工单"
        selectable
        selectedKeys={selected}
        onSelectedChange={setSelected}
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
