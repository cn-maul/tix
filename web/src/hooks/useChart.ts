import { useEffect, useRef, useState } from 'react'
import type * as echartsTypes from 'echarts/core'

type EChartsOption = echartsTypes.EChartsCoreOption

let echartsPromise: Promise<typeof echartsTypes> | null = null

function loadEcharts() {
  if (!echartsPromise) {
    echartsPromise = import('@/lib/echarts').then((m) => m.default)
  }
  return echartsPromise
}

export default function useChart(option: EChartsOption) {
  const [el, setEl] = useState<HTMLDivElement | null>(null)
  const chartRef = useRef<echartsTypes.ECharts | null>(null)
  const roRef = useRef<ResizeObserver | null>(null)
  // 始终持有最新 option：echarts 懒加载完成前 option 可能已更新，
  // 初始化时用最新值，避免图表以过期配置渲染
  const optionRef = useRef(option)
  optionRef.current = option

  useEffect(() => {
    if (!el) return
    let disposed = false

    loadEcharts().then((echarts) => {
      if (disposed) return
      const chart = echarts.init(el)
      chartRef.current = chart
      chart.setOption(optionRef.current, true)
      const ro = new ResizeObserver(() => chart.resize())
      ro.observe(el)
      roRef.current = ro
    })

    return () => {
      disposed = true
      roRef.current?.disconnect()
      roRef.current = null
      chartRef.current?.dispose()
      chartRef.current = null
    }
  }, [el])

  useEffect(() => {
    chartRef.current?.setOption(option, true)
  }, [option])

  return setEl
}
