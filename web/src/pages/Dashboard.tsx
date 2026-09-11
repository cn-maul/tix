import { useMemo, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchStats } from '../api/stats'
import { Clock, CheckCircle2, ListPlus, Flame } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import PageSpinner from '@/components/PageSpinner'
import PageHeader from '@/components/PageHeader'

const PALETTE = ['#2563eb', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899']
const fmtDay = (d: string) => d.slice(5)

type DayCategory = { day: string; category: string; count: number }

function StatCard({ icon: Icon, label, value, color }: {
  icon: typeof Clock
  label: string
  value: number
  color: string
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 pt-6">
        <div className="flex size-11 shrink-0 items-center justify-center rounded-lg text-white" style={{ backgroundColor: color }}>
          <Icon className="size-5" />
        </div>
        <div>
          <div className="text-sm text-muted-foreground">{label}</div>
          <div className="text-2xl font-semibold tabular-nums">{value}</div>
        </div>
      </CardContent>
    </Card>
  )
}

function ChartCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader><CardTitle className="text-base">{title}</CardTitle></CardHeader>
      <CardContent className="h-72">{children}</CardContent>
    </Card>
  )
}

function Empty() {
  return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">暂无数据</div>
}

function StackedBarChart({ data }: { data: DayCategory[] }) {
  const days = [...new Set(data.map((d) => d.day))]
  const categories = [...new Set(data.map((d) => d.category))]
  const values = days.map((day) => categories.map((category) => data.find((d) => d.day === day && d.category === category)?.count ?? 0))
  const max = Math.max(1, ...values.map((items) => items.reduce((sum, value) => sum + value, 0)))
  return (
    <div className="flex h-full flex-col gap-2">
      <svg viewBox="0 0 600 220" className="min-h-0 w-full flex-1" role="img" aria-label="分类分布按天">
        {[0, 0.5, 1].map((ratio) => <line key={ratio} x1="42" x2="590" y1={190 - ratio * 160} y2={190 - ratio * 160} stroke="currentColor" className="text-border" />)}
        {values.map((items, index) => {
          let offset = 0
          const x = 48 + index * (540 / Math.max(1, days.length))
          const width = Math.min(28, 420 / Math.max(1, days.length))
          return <g key={days[index]}>
            {items.map((value, categoryIndex) => {
              const height = (value / max) * 160
              const y = 190 - offset - height
              offset += height
              return value > 0 ? <rect key={categories[categoryIndex]} x={x} y={y} width={width} height={height} fill={PALETTE[categoryIndex % PALETTE.length]} rx="2"><title>{`${days[index]} ${categories[categoryIndex]}：${value}`}</title></rect> : null
            })}
            <text x={x + width / 2} y="208" textAnchor="middle" className="fill-muted-foreground text-[10px]">{days[index]}</text>
          </g>
        })}
      </svg>
      <Legend items={categories} />
    </div>
  )
}

function BarChart({ data }: { data: { day: string; count: number }[] }) {
  const max = Math.max(1, ...data.map((d) => d.count))
  return <svg viewBox="0 0 600 220" className="h-full w-full" role="img" aria-label="工单数量按天">
    {[0, 0.5, 1].map((ratio) => <line key={ratio} x1="42" x2="590" y1={190 - ratio * 160} y2={190 - ratio * 160} stroke="currentColor" className="text-border" />)}
    {data.map((item, index) => {
      const width = Math.min(28, 420 / Math.max(1, data.length))
      const x = 48 + index * (540 / Math.max(1, data.length))
      const height = (item.count / max) * 160
      return <g key={item.day}><rect x={x} y={190 - height} width={width} height={height} fill="#2563eb" rx="2"><title>{`${item.day}：${item.count}`}</title></rect><text x={x + width / 2} y="208" textAnchor="middle" className="fill-muted-foreground text-[10px]">{item.day}</text></g>
    })}
  </svg>
}

function Legend({ items }: { items: string[] }) {
  return <div className="flex flex-wrap justify-center gap-x-4 gap-y-1 text-xs text-muted-foreground">{items.map((item, index) => <span key={item} className="flex items-center gap-1.5"><span className="size-2.5 rounded-full" style={{ backgroundColor: PALETTE[index % PALETTE.length] }} />{item}</span>)}</div>
}

function DonutChart({ data }: { data: { category: string; count: number }[] }) {
  const total = data.reduce((sum, item) => sum + item.count, 0)
  let start = 0
  const segments = data.map((item, index) => {
    const end = start + (item.count / total) * 360
    const segment = `${PALETTE[index % PALETTE.length]} ${start}deg ${end}deg`
    start = end
    return segment
  })
  return <div className="flex h-full flex-col items-center justify-center gap-3">
    <div className="relative size-44 rounded-full" style={{ background: `conic-gradient(${segments.join(', ')})` }}>
      <div className="absolute inset-8 flex items-center justify-center rounded-full bg-card text-sm font-semibold tabular-nums">{total}</div>
    </div>
    <Legend items={data.map((item) => item.category)} />
  </div>
}

export default function Dashboard() {
  const { data, isLoading } = useQuery({ queryKey: ['stats'], queryFn: fetchStats })
  const dayData = useMemo(() => (data?.by_day ?? []).map((d) => ({ day: fmtDay(d.category), count: d.count })), [data])
  const dayCatData = useMemo(() => (data?.by_day_cat ?? []).map((d) => ({ day: fmtDay(d.day), category: d.category, count: d.count })), [data])
  const monthData = useMemo(() => (data?.month_cat ?? []).filter((m) => m.count > 0), [data])

  if (isLoading) return <PageSpinner />
  const total = (data?.pending ?? 0) + (data?.done ?? 0)

  return <div className="space-y-4">
    <PageHeader title="仪表盘" />
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <StatCard icon={Clock} label="待处理" value={data?.pending ?? 0} color="#f59e0b" />
      <StatCard icon={CheckCircle2} label="已处理" value={data?.done ?? 0} color="#10b981" />
      <StatCard icon={ListPlus} label="工单总数" value={total} color="#2563eb" />
      <StatCard icon={Flame} label="今日新增" value={data?.today_new ?? 0} color="#ef4444" />
    </div>
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      <ChartCard title="分类分布（按天）">{dayCatData.length > 0 ? <StackedBarChart data={dayCatData} /> : <Empty />}</ChartCard>
      <ChartCard title="工单数量（按天）">{dayData.length > 0 ? <BarChart data={dayData} /> : <Empty />}</ChartCard>
    </div>
    <ChartCard title="本月分类分布">
      {monthData.length > 0 ? <div className="grid h-full grid-cols-1 gap-4 md:grid-cols-2"><DonutChart data={monthData} /><div className="flex flex-col justify-center gap-2">{monthData.map((m, index) => <div key={m.category} className="flex items-center justify-between rounded-lg border px-4 py-2"><span className="flex items-center gap-2 text-sm"><span className="size-3 rounded-full" style={{ backgroundColor: PALETTE[index % PALETTE.length] }} />{m.category}</span><span className="font-medium tabular-nums">{m.count}</span></div>)}</div></div> : <Empty />}
    </ChartCard>
  </div>
}
