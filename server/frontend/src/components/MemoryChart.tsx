import ReactECharts from 'echarts-for-react'
import { useMemo } from 'react'

interface MemoryChartProps {
  /** 时间戳数组（秒级） */
  timestamps: number[]
  /** 内存使用率数组（0-100） */
  memData: number[]
  /** 是否深色主题 */
  isDark?: boolean
  /** 图表高度 */
  height?: number
}

/** 内存使用率实时折线图 */
export default function MemoryChart({
  timestamps,
  memData,
  isDark = false,
  height = 300,
}: MemoryChartProps) {
  const option = useMemo(() => {
    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(30,30,30,0.95)' : 'rgba(255,255,255,0.95)',
        borderColor: isDark ? '#444' : '#e5e7eb',
        textStyle: {
          color: isDark ? '#e5e7eb' : '#1f2937',
        },
        formatter: (params: any) => {
          const point = params[0]
          const time = new Date(point.axisValue * 1000).toLocaleTimeString('zh-CN')
          return `${time}<br/>内存: <strong>${point.value.toFixed(1)}%</strong>`
        },
      },
      grid: {
        left: '8%',
        right: '5%',
        top: '10%',
        bottom: '12%',
      },
      xAxis: {
        type: 'category',
        data: timestamps,
        axisLine: {
          lineStyle: { color: isDark ? '#444' : '#e5e7eb' },
        },
        axisLabel: {
          color: isDark ? '#9ca3af' : '#6b7280',
          fontSize: 11,
          formatter: (value: number) => {
            return new Date(value * 1000).toLocaleTimeString('zh-CN', {
              hour: '2-digit',
              minute: '2-digit',
            })
          },
        },
        splitLine: { show: false },
      },
      yAxis: {
        type: 'value',
        min: 0,
        max: 100,
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: {
          color: isDark ? '#9ca3af' : '#6b7280',
          fontSize: 11,
          formatter: '{value}%',
        },
        splitLine: {
          lineStyle: {
            color: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)',
          },
        },
      },
      series: [
        {
          name: '内存',
          type: 'line',
          data: memData,
          smooth: true,
          symbol: 'none',
          lineStyle: {
            width: 2,
            color: '#8b5cf6',
          },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: 'rgba(139,92,246,0.3)' },
                { offset: 1, color: 'rgba(139,92,246,0.02)' },
              ],
            },
          },
          markLine: {
            silent: true,
            symbol: 'none',
            data: [
              {
                yAxis: 85,
                lineStyle: { color: '#f59e0b', type: 'dashed', width: 1 },
                label: { show: false },
              },
              {
                yAxis: 95,
                lineStyle: { color: '#ef4444', type: 'dashed', width: 1 },
                label: { show: false },
              },
            ],
          },
        },
      ],
    }
  }, [timestamps, memData, isDark])

  return (
    <ReactECharts
      option={option}
      style={{ height: `${height}px`, width: '100%' }}
      opts={{ renderer: 'canvas' }}
      notMerge={false}
      lazyUpdate={true}
    />
  )
}
