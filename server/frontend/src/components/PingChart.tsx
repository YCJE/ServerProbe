import ReactECharts from 'echarts-for-react'
import { useMemo } from 'react'
import type { PingResult } from '@/types'

interface PingChartProps {
  /** 时间戳数组（秒级） */
  timestamps: number[]
  /** 每个时间点的 Ping 数据数组 */
  pingData: PingResult[][]
  /** 是否深色主题 */
  isDark?: boolean
  /** 图表高度 */
  height?: number
}

/** 三网颜色配置 */
const NETWORK_COLORS: Record<string, string> = {
  '电信': '#3b82f6',
  '联通': '#22c55e',
  '移动': '#f59e0b',
}

/** 默认颜色池 */
const DEFAULT_COLORS = ['#3b82f6', '#22c55e', '#f59e0b', '#8b5cf6', '#ec4899']

/** 转义 HTML 特殊字符，防止 XSS 注入 */
function escapeHtml(str: string): string {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/**
 * 延迟与丢包率融合图
 * 上半部分：延迟折线图（三网三条线，直线段连接，参考 nezha/komari 风格）
 * 下半部分：丢包率折线图（带面积填充）
 * 共享 X 轴
 */
export default function PingChart({
  timestamps,
  pingData,
  isDark = false,
  height = 400,
}: PingChartProps) {
  const option = useMemo(() => {
    // 提取所有唯一的网络名称（保持顺序）
    const networkNames: string[] = []
    const seen = new Set<string>()
    for (const pings of pingData) {
      if (pings) {
        for (const p of pings) {
          if (!seen.has(p.name)) {
            seen.add(p.name)
            networkNames.push(p.name)
          }
        }
      }
    }

    // 为每个网络构建延迟数据序列
    // nezha 风格：直线段连接(smooth:false)，无数据点符号，null 创建间隙
    const latencySeries = networkNames.map((name, idx) => {
      const color = NETWORK_COLORS[name] || DEFAULT_COLORS[idx % DEFAULT_COLORS.length]
      const data = timestamps.map((_, tsIdx) => {
        const pings = pingData[tsIdx]
        if (!pings) return null
        const ping = pings.find((p) => p.name === name)
        return ping ? ping.avg_latency : null
      })

      return {
        name: `${name} 延迟`,
        type: 'line',
        data,
        smooth: false,       // 直线段连接，保留折角细节
        symbol: 'none',      // 不显示数据点
        lineStyle: { width: 1.5, color },
        itemStyle: { color },
        connectNulls: false, // null 创建间隙，更真实
        // 渐变面积填充（参考 CPU 图风格）
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: color + '20' },
              { offset: 1, color: color + '02' },
            ],
          },
        },
      }
    })

    // 丢包率数据序列（直线段，带面积填充）
    const lossSeries = networkNames.map((name, idx) => {
      const color = NETWORK_COLORS[name] || DEFAULT_COLORS[idx % DEFAULT_COLORS.length]
      const data = timestamps.map((_, tsIdx) => {
        const pings = pingData[tsIdx]
        if (!pings) return null
        const ping = pings.find((p) => p.name === name)
        return ping ? ping.loss : null
      })

      return {
        name: `${name} 丢包率`,
        type: 'line',
        xAxisIndex: 1,
        yAxisIndex: 1,
        data,
        smooth: false,       // 直线段
        symbol: 'none',
        lineStyle: { width: 1.5, color },
        itemStyle: { color },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0, y: 0, x2: 0, y2: 1,
            colorStops: [
              { offset: 0, color: color + '30' },
              { offset: 1, color: color + '05' },
            ],
          },
        },
        connectNulls: false,
      }
    })

    const axisLabelColor = isDark ? '#9ca3af' : '#6b7280'
    const splitLineColor = isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)'
    const axisLineColor = isDark ? '#444' : '#e5e7eb'

    return {
      tooltip: {
        trigger: 'axis',
        backgroundColor: isDark ? 'rgba(30,30,30,0.95)' : 'rgba(255,255,255,0.95)',
        borderColor: isDark ? '#444' : '#e5e7eb',
        textStyle: {
          color: isDark ? '#e5e7eb' : '#1f2937',
        },
        formatter: (params: any) => {
          if (!params || params.length === 0) return ''
          const time = new Date(params[0].axisValue * 1000).toLocaleString('zh-CN')
          let html = `<div style="font-weight:600;margin-bottom:4px">${time}</div>`

          // 延迟部分
          const latencyParams = params.filter((p: any) => p.seriesName.includes('延迟'))
          if (latencyParams.length > 0) {
            html += '<div style="margin-bottom:4px">延迟 (ms):</div>'
            for (const p of latencyParams) {
              if (p.value !== null && p.value !== undefined) {
                html += `<div style="display:flex;align-items:center;gap:6px">${p.marker} ${escapeHtml(p.seriesName.replace(' 延迟', ''))}: <strong>${p.value.toFixed(1)} ms</strong></div>`
              }
            }
          }

          // 丢包率部分
          const lossParams = params.filter((p: any) => p.seriesName.includes('丢包率'))
          if (lossParams.length > 0) {
            html += '<div style="margin-top:4px;margin-bottom:4px">丢包率:</div>'
            for (const p of lossParams) {
              if (p.value !== null && p.value !== undefined) {
                const lossColor = p.value > 20 ? '#ef4444' : p.value > 0 ? '#f59e0b' : '#22c55e'
                html += `<div style="display:flex;align-items:center;gap:6px">${p.marker} ${escapeHtml(p.seriesName.replace(' 丢包率', ''))}: <strong style="color:${lossColor}">${p.value.toFixed(1)}%</strong></div>`
              }
            }
          }

          return html
        },
      },
      legend: {
        data: [...networkNames.map((name) => `${name} 延迟`), ...networkNames.map((name) => `${name} 丢包率`)],
        top: 0,
        textStyle: { color: axisLabelColor, fontSize: 11 },
        itemWidth: 16,
        itemHeight: 8,
      },
      axisPointer: {
        link: [{ xAxisIndex: 'all' }],
      },
      grid: [
        // 上半部分：延迟折线图
        {
          left: '8%',
          right: '5%',
          top: '12%',
          height: '45%',
        },
        // 下半部分：丢包率折线图
        {
          left: '8%',
          right: '5%',
          top: '65%',
          height: '25%',
        },
      ],
      xAxis: [
        // 延迟图 X 轴（隐藏标签）
        {
          type: 'category',
          gridIndex: 0,
          data: timestamps,
          axisLine: { lineStyle: { color: axisLineColor } },
          axisLabel: { show: false },
          axisTick: { show: false },
          splitLine: { show: false },
          boundaryGap: false,
        },
        // 丢包率图 X 轴（显示标签）
        {
          type: 'category',
          gridIndex: 1,
          data: timestamps,
          axisLine: { lineStyle: { color: axisLineColor } },
          axisLabel: {
            color: axisLabelColor,
            fontSize: 11,
            formatter: (value: number) => {
              return new Date(value * 1000).toLocaleTimeString('zh-CN', {
                hour: '2-digit',
                minute: '2-digit',
              })
            },
          },
          axisTick: { show: false },
          splitLine: { show: false },
          boundaryGap: false,
        },
      ],
      yAxis: [
        // 延迟图 Y 轴
        {
          type: 'value',
          gridIndex: 0,
          name: '延迟 (ms)',
          nameTextStyle: { color: axisLabelColor, fontSize: 11 },
          axisLine: { show: false },
          axisTick: { show: false },
          axisLabel: {
            color: axisLabelColor,
            fontSize: 11,
            formatter: '{value}',
          },
          splitLine: { lineStyle: { color: splitLineColor } },
          minInterval: 1,
        },
        // 丢包率图 Y 轴
        {
          type: 'value',
          gridIndex: 1,
          name: '丢包率 (%)',
          nameTextStyle: { color: axisLabelColor, fontSize: 11 },
          min: 0,
          max: 100,
          axisLine: { show: false },
          axisTick: { show: false },
          axisLabel: {
            color: axisLabelColor,
            fontSize: 11,
            formatter: '{value}%',
          },
          splitLine: { lineStyle: { color: splitLineColor } },
        },
      ],
      series: [...latencySeries, ...lossSeries],
    }
  }, [timestamps, pingData, isDark])

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
