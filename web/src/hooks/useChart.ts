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

  useEffect(() => {
    if (!el) return
    let disposed = false

    loadEcharts().then((echarts) => {
      if (disposed) return
      const chart = echarts.init(el)
      chartRef.current = chart
      chart.setOption(option, true)
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
