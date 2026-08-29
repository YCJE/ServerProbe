import ReactECharts from 'echarts-for-react'
import { useMemo } from 'react'
import { cssColor, cssColorAlpha } from '@/lib/theme'
import Skeleton from '@/components/Skeleton'

interface MemoryChartProps {
  /** 时间戳数组（秒级） */
  timestamps: number[]
  /** 内存使用率数组（0-100） */
  memData: number[]
  /** 是否深色主题（仅作为重渲染触发器，颜色已全部走 CSS 变量） */
  isDark?: boolean
  /** 图表高度 */
  height?: number
  /** 加载中：渲染骨架占位 */
  loading?: boolean
  /** 加载失败：渲染错误提示 */
  error?: string
}

/** 内存使用率实时折线图 */
export default function MemoryChart({
  timestamps,
  memData,
  isDark = false,
  height = 300,
  loading = false,
  error,
}: MemoryChartProps) {
  const option = useMemo(() => {
    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: cssColorAlpha('--card', 0.95),
        borderColor: cssColor('--border'),
        textStyle: {
          color: cssColor('--foreground'),
        },
        formatter: (params: unknown) => {
          const points = params as Array<{ value?: number | null; axisValue?: number }> | undefined
          const point = points?.[0]
          if (!point || point.value == null) return ''
          const time = new Date((point.axisValue ?? 0) * 1000).toLocaleTimeString('zh-CN')
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
          lineStyle: { color: cssColor('--border') },
        },
        axisLabel: {
          color: cssColor('--muted-foreground'),
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
          color: cssColor('--muted-foreground'),
          fontSize: 11,
          formatter: '{value}%',
        },
        splitLine: {
          lineStyle: {
            color: cssColorAlpha('--border', 0.5),
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
            color: cssColor('--metric-mem'),
          },
          areaStyle: {
            color: {
              type: 'linear',
              x: 0,
              y: 0,
              x2: 0,
              y2: 1,
              colorStops: [
                { offset: 0, color: cssColorAlpha('--metric-mem', 0.3) },
                { offset: 1, color: cssColorAlpha('--metric-mem', 0.02) },
              ],
            },
          },
          markLine: {
            silent: true,
            symbol: 'none',
            data: [
              {
                yAxis: 85,
                lineStyle: { color: cssColor('--warning'), type: 'dashed', width: 1 },
                label: { show: false },
              },
              {
                yAxis: 95,
                lineStyle: { color: cssColor('--destructive'), type: 'dashed', width: 1 },
                label: { show: false },
              },
            ],
          },
        },
      ],
    }
    // isDark 仅作主题切换时的重算触发器（颜色实时读取 CSS 变量）
  }, [timestamps, memData, isDark])

  if (loading) {
    return <Skeleton variant="chart" height={height} />
  }

  if (error) {
    return (
      <div
        className="flex flex-col items-center justify-center gap-2 rounded-md border border-dashed border-destructive/50 text-sm text-destructive"
        style={{ height: `${height}px` }}
      >
        <span>{error}</span>
      </div>
    )
  }

  return (
    <ReactECharts
      option={option}
      style={{ height: `${height}px`, width: '100%' }}
      opts={{ renderer: 'canvas' }}
      notMerge={true}
      lazyUpdate={true}
    />
  )
}
