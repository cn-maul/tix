import { Download, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

type AssigneeFilter = 'all' | 'me' | 'unassigned'

type Category = { id: string | number; name: string }

type TicketFiltersProps = {
  keyword: string
  onKeywordChange: (value: string) => void
  categories?: Category[]
  category: string
  onCategoryChange: (value: string) => void
  from: string
  to: string
  onDateRangeChange: (from: string, to: string) => void
  assignee: AssigneeFilter
  onAssigneeChange: (value: AssigneeFilter) => void
  onExport: () => void
  exporting: boolean
}

export default function TicketFilters({
  keyword, onKeywordChange, categories, category, onCategoryChange,
  from, to, onDateRangeChange, assignee, onAssigneeChange, onExport, exporting,
}: TicketFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted/30 p-3">
      <div className="relative min-w-[220px] flex-1 sm:flex-none">
        <Search className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input className="w-full pl-8 sm:w-56" placeholder="搜索内容 / 发起人 / 手机号" value={keyword} onChange={(e) => onKeywordChange(e.target.value)} />
      </div>
      <Select value={category || 'all'} onValueChange={onCategoryChange}>
        <SelectTrigger className="w-32"><SelectValue placeholder="全部分类" /></SelectTrigger>
        <SelectContent>
          <SelectItem value="all">全部分类</SelectItem>
          {(categories ?? []).map((c) => <SelectItem key={c.id} value={c.name}>{c.name}</SelectItem>)}
        </SelectContent>
      </Select>
      <div className="flex items-center gap-1.5">
        <Input type="date" className="w-[9.5rem]" aria-label="创建日期起" value={from} onChange={(e) => onDateRangeChange(e.target.value, to)} />
        <span className="text-xs text-muted-foreground">至</span>
        <Input type="date" className="w-[9.5rem]" aria-label="创建日期止" value={to} onChange={(e) => onDateRangeChange(from, e.target.value)} />
      </div>
      <Select value={assignee} onValueChange={(v) => onAssigneeChange(v as AssigneeFilter)}>
        <SelectTrigger className="w-32"><SelectValue placeholder="负责人" /></SelectTrigger>
        <SelectContent>
          <SelectItem value="all">全部负责人</SelectItem><SelectItem value="unassigned">未指派</SelectItem><SelectItem value="me">我负责的</SelectItem>
        </SelectContent>
      </Select>
      <Button variant="outline" onClick={onExport} loading={exporting}>{!exporting && <Download />}导出 CSV</Button>
    </div>
  )
}
