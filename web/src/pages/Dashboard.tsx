import { useMemo, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchStats } from '../api/stats'
import { Clock, CheckCircle2, ListPlus, Flame } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import useChart from '@/hooks/useChart'
import { useTheme } from '@/lib/theme'
import type * as echarts from 'echarts/core'

const PALETTE = ['#2563eb', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899']

const fmtDay = (d: string) => d.slice(5)

function StatCard({ icon: Icon, label, value, color }: {
  icon: typeof Clock
  label: string
  value: number
  color: string
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 pt-6">
        <div
          className="flex size-11 shrink-0 items-center justify-center rounded-lg text-white"
          style={{ backgroundColor: color }}
        >
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
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent className="h-72">{children}</CardContent>
    </Card>
  )
}

function Empty() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      暂无数据
    </div>
  )
}

export default function Dashboard() {
  const { data, isLoading } = useQuery({ queryKey: ['stats'], queryFn: fetchStats })
  const { dark } = useTheme()

  // 图表配色随主题切换：浅色/暗色下文字与网格线使用不同对比度
  const axisColor = dark ? '#94a3b8' : '#475569'
  const splitColor = dark ? 'rgba(148,163,184,0.18)' : 'rgba(100,116,139,0.18)'
  const baseChart = useMemo(
    () => ({
      textStyle: { color: axisColor },
      legend: { bottom: 0, textStyle: { color: axisColor } },
      tooltip: {
        backgroundColor: dark ? 'rgba(30,41,59,0.95)' : 'rgba(255,255,255,0.95)',
        borderColor: splitColor,
        textStyle: { color: dark ? '#e2e8f0' : '#0f172a' },
      },
    }),
    [dark, axisColor, splitColor],
  )

  const dayData = useMemo(() => (data?.by_day ?? []).map((d) => ({ day: fmtDay(d.category), count: d.count })), [data])
  const dayCatData = useMemo(
    () => (data?.by_day_cat ?? []).map((d) => ({ day: fmtDay(d.day), category: d.category, count: d.count })),
    [data],
  )
  const monthData = useMemo(() => (data?.month_cat ?? []).filter((m) => m.count > 0), [data])

  const stackedOption = useMemo(() => {
    const cats = [...new Set(dayCatData.map((d) => d.category))]
    const days = [...new Set(dayCatData.map((d) => d.day))]
    return {
      ...baseChart,
      color: PALETTE,
      tooltip: { ...baseChart.tooltip, trigger: 'axis' },
      grid: { left: 8, right: 8, top: 8, bottom: 32, containLabel: true },
      xAxis: {
        type: 'category', data: days,
        axisLine: { lineStyle: { color: splitColor } },
        axisLabel: { color: axisColor },
      },
      yAxis: {
        type: 'value', minInterval: 1,
        splitLine: { lineStyle: { color: splitColor } },
        axisLabel: { color: axisColor },
      },
      series: cats.map((c) => ({
        name: c,
        type: 'bar',
        stack: 'total',
        barMaxWidth: 28,
        data: days.map((day) => dayCatData.find((x) => x.day === day && x.category === c)?.count ?? 0),
      })),
    } as echarts.EChartsCoreOption
  }, [dayCatData, baseChart, axisColor, splitColor])

  const dayOption = useMemo(
    () =>
      ({
        ...baseChart,
        color: ['#2563eb'],
        tooltip: { ...baseChart.tooltip, trigger: 'axis' },
        grid: { left: 8, right: 8, top: 16, bottom: 24, containLabel: true },
        xAxis: {
          type: 'category', data: dayData.map((d) => d.day),
          axisLine: { lineStyle: { color: splitColor } },
          axisLabel: { color: axisColor },
        },
        yAxis: {
          type: 'value', minInterval: 1,
          splitLine: { lineStyle: { color: splitColor } },
          axisLabel: { color: axisColor },
        },
        series: [{ type: 'bar', barMaxWidth: 28, data: dayData.map((d) => d.count) }],
      }) as echarts.EChartsCoreOption,
    [dayData, baseChart, axisColor, splitColor],
  )

  const pieOption = useMemo(
    () =>
      ({
        ...baseChart,
        color: PALETTE,
        tooltip: { ...baseChart.tooltip, trigger: 'item', formatter: '{b}: {c} ({d}%)' },
        legend: { ...baseChart.legend, type: 'scroll' },
        series: [
          {
            type: 'pie',
            radius: ['42%', '68%'],
            center: ['50%', '45%'],
            data: monthData.map((m) => ({ name: m.category, value: m.count })),
          },
        ],
      }) as echarts.EChartsCoreOption,
    [monthData, baseChart],
  )

  const stackedRef = useChart(stackedOption)
  const dayRef = useChart(dayOption)
  const pieRef = useChart(pieOption)

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
      </div>
    )
  }

  const total = (data?.pending ?? 0) + (data?.done ?? 0)

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">仪表盘</h1>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard icon={Clock} label="待处理" value={data?.pending ?? 0} color="#f59e0b" />
        <StatCard icon={CheckCircle2} label="已处理" value={data?.done ?? 0} color="#10b981" />
        <StatCard icon={ListPlus} label="工单总数" value={total} color="#2563eb" />
        <StatCard icon={Flame} label="今日新增" value={data?.today_new ?? 0} color="#ef4444" />
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <ChartCard title="分类分布（按天）">
          {dayCatData.length > 0 ? <div ref={stackedRef} className="h-full w-full" /> : <Empty />}
        </ChartCard>
        <ChartCard title="工单数量（按天）">
          {dayData.length > 0 ? <div ref={dayRef} className="h-full w-full" /> : <Empty />}
        </ChartCard>
      </div>

      <ChartCard title="本月分类分布">
        {monthData.length > 0 ? (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <div ref={pieRef} className="h-72 w-full" />
            <div className="flex flex-col justify-center gap-2">
              {monthData.map((m) => (
                <div key={m.category} className="flex items-center justify-between rounded-lg border px-4 py-2">
                  <span className="flex items-center gap-2 text-sm">
                    <span
                      className="size-3 rounded-full"
                      style={{ backgroundColor: PALETTE[monthData.indexOf(m) % PALETTE.length] }}
                    />
                    {m.category}
                  </span>
                  <span className="font-medium tabular-nums">{m.count}</span>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <Empty />
        )}
      </ChartCard>
    </div>
  )
}