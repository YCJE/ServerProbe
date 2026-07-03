import { useMemo } from 'react'

interface MiniBarChartProps {
  /** 延迟数值数组，如 [45.2, 43.1, 50.5, 48.3, ...] */
  data: number[]
  /** Y轴最大值，默认取data最大值 */
  max?: number
  /** 柱子颜色，默认 'var(--apple-blue)' */
  color?: string
  /** 高度px，默认36 */
  height?: number
  /** 单柱宽度px，默认3 */
  barWidth?: number
  /** 柱间距px，默认2 */
  gap?: number
}

/**
 * 纯 CSS 迷你柱状图
 * 用于在服务器卡片中展示三网延迟（电信/联通/移动）的历史数据
 */
export default function MiniBarChart({
  data,
  max,
  color = 'var(--apple-blue)',
  height = 36,
  barWidth = 3,
  gap = 2,
}: MiniBarChartProps) {
  // 计算 Y 轴最大值，避免除零
  const maxValue = useMemo(() => {
    if (max !== undefined && max > 0) return max
    if (data.length === 0) return 1
    const m = Math.max(...data)
    return m > 0 ? m : 1
  }, [data, max])

  if (data.length === 0) {
    return <div className="flex items-end" style={{ height }} aria-hidden />
  }

  return (
    <div
      className="flex items-end"
      style={{ height, gap: `${gap}px` }}
      role="img"
      aria-label="延迟历史柱状图"
    >
      {data.map((value, i) => {
        // 每个柱子高度按 value/max 比例计算，最小高度1px避免空柱
        const ratio = value > 0 ? value / maxValue : 0
        const barHeight = Math.max(1, ratio * height)
        return (
          <div
            key={i}
            style={{
              width: `${barWidth}px`,
              height: `${barHeight}px`,
              backgroundColor: color,
              borderRadius: `${barWidth / 2}px ${barWidth / 2}px 0 0`,
              transition: 'height 0.3s ease',
              flexShrink: 0,
            }}
          />
        )
      })}
    </div>
  )
}
