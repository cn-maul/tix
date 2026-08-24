import type { ReactNode } from 'react'
import {
  Table as UITable,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface Column<T> {
  title: ReactNode
  key: string
  width?: number | string
  render?: (record: T) => ReactNode
}

// 复选框（原生 input + shadcn 观感，不引入额外依赖）
function RowCheckbox({
  checked,
  indeterminate,
  onChange,
  label,
}: {
  checked: boolean
  indeterminate?: boolean
  onChange: () => void
  label: string
}) {
  return (
    <input
      type="checkbox"
      aria-label={label}
      checked={checked}
      ref={(el) => {
        if (el) el.indeterminate = !!indeterminate && !checked
      }}
      onChange={onChange}
      className="size-4 cursor-pointer accent-primary"
    />
  )
}

export function DataTable<T extends object>({
  columns,
  dataSource,
  rowKey,
  loading,
  empty = '暂无数据',
  className,
  selectable = false,
  selectedKeys,
  onSelectedChange,
}: {
  columns: Column<T>[]
  dataSource: T[]
  rowKey: (r: T) => string | number
  loading?: boolean
  empty?: ReactNode
  className?: string
  // 多选：开启后首列渲染复选框；受控用法见 TicketList
  selectable?: boolean
  selectedKeys?: (string | number)[]
  onSelectedChange?: (keys: (string | number)[]) => void
}) {
  const allKeys = dataSource.map(rowKey)
  const selectedSet = new Set(selectedKeys ?? [])
  const allChecked = allKeys.length > 0 && allKeys.every((k) => selectedSet.has(k))
  const someChecked = allKeys.some((k) => selectedSet.has(k))

  const toggleRow = (key: string | number) => {
    if (!onSelectedChange) return
    const next = selectedSet.has(key)
      ? [...selectedSet].filter((k) => k !== key)
      : [...selectedSet, key]
    onSelectedChange(next)
  }
  const toggleAll = () => {
    if (!onSelectedChange) return
    onSelectedChange(allChecked ? [] : allKeys)
  }

  const cols: Column<T>[] = selectable
    ? [
        {
          title: (
            <RowCheckbox
              label="全选本页"
              checked={allChecked}
              indeterminate={someChecked}
              onChange={toggleAll}
            />
          ),
          key: '__select__',
          width: 36,
          render: (r) => {
            const key = rowKey(r)
            return (
              <RowCheckbox
                label={`选择 ${key}`}
                checked={selectedSet.has(key)}
                onChange={() => toggleRow(key)}
              />
            )
          },
        },
        ...columns,
      ]
    : columns

  return (
    <div className={cn('rounded-xl border bg-card shadow-sm', className)}>
      <UITable>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {cols.map((c) => (
              <TableHead key={c.key} style={{ width: c.width }}>
                {c.title}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading
            ? Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={`skeleton-${i}`}>
                  {cols.map((c) => (
                    <TableCell key={c.key}>
                      <Skeleton className="h-4 w-full" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            : dataSource.length === 0
              ? (
                  <TableRow>
                    <TableCell colSpan={cols.length} className="h-24 text-center text-muted-foreground">
                      {empty}
                    </TableCell>
                  </TableRow>
                )
              : dataSource.map((r) => (
                  <TableRow key={rowKey(r)}>
                    {cols.map((c) => (
                      <TableCell key={c.key}>{c.render ? c.render(r) : String((r as any)[c.key])}</TableCell>
                    ))}
                  </TableRow>
                ))}
        </TableBody>
      </UITable>
    </div>
  )
}

export function PaginationBar({
  page,
  pageSize,
  total,
  onChange,
}: {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
}) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const disabled = total === 0
  return (
    <div className="flex items-center justify-between gap-4 pt-4 text-sm text-muted-foreground">
      <span>共 {total} 条</span>
      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={disabled || page <= 1}
          onClick={() => onChange(page - 1)}
        >
          上一页
        </Button>
        <span className="tabular-nums">
          {page} / {totalPages}
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={disabled || page >= totalPages}
          onClick={() => onChange(page + 1)}
        >
          下一页
        </Button>
      </div>
    </div>
  )
}
