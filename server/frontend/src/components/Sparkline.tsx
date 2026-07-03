import { useId } from 'react'

interface SparklineProps {
  /** 数据点数组 */
  data: number[]
  /** 线条颜色（十六进制或 rgb） */
  color?: string
  /** 图表高度（px） */
  height?: number
  /** 图表最大宽度（px），不传则填满容器 */
  width?: number
}

/** SVG 内部坐标系宽度 */
const VB_W = 100
/** SVG 内部坐标系高度 */
const VB_H = 30

/**
 * 迷你趋势图（Sparkline）
 * 基于 SVG 绘制，使用 viewBox + preserveAspectRatio="none" 实现自适应宽度，
 * 配合 vectorEffect="non-scaling-stroke" 保证线条粗细不随缩放失真。
 */
export default function Sparkline({
  data,
  color = '#5AC8FA',
  height = 40,
  width,
}: SparklineProps) {
  const uid = useId()
  const gradId = `sparkline-grad-${uid.replace(/[^a-zA-Z0-9]/g, '')}`

  // 过滤无效值
  const validValues = data.filter((v) => v != null && isFinite(v))

  if (validValues.length < 2) {
    return (
      <div
        className="flex w-full items-center justify-center"
        style={{ height, maxWidth: width }}
      >
        <span className="text-[10px] text-muted-foreground/40">暂无趋势</span>
      </div>
    )
  }

  const min = Math.min(...validValues)
  const max = Math.max(...validValues)
  const range = max - min || 1

  // 计算每个数据点的坐标，跳过无效值
  const points = data
    .map((v, i) => {
      const x = (i / (data.length - 1)) * VB_W
      const y =
        v != null && isFinite(v)
          ? VB_H - ((v - min) / range) * (VB_H - 4) - 2
          : null
      return { x, y }
    })
    .filter((p): p is { x: number; y: number } => p.y !== null)

  if (points.length < 2) {
    return (
      <div
        className="flex w-full items-center justify-center"
        style={{ height, maxWidth: width }}
      >
        <span className="text-[10px] text-muted-foreground/40">暂无趋势</span>
      </div>
    )
  }

  const linePath = points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x.toFixed(2)} ${p.y.toFixed(2)}`)
    .join(' ')
  const areaPath = `${linePath} L ${VB_W} ${VB_H} L 0 ${VB_H} Z`

  return (
    <svg
      height={height}
      viewBox={`0 0 ${VB_W} ${VB_H}`}
      preserveAspectRatio="none"
      className="w-full"
      style={width != null ? { maxWidth: width } : undefined}
    >
      <defs>
        <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity={0.3} />
          <stop offset="100%" stopColor={color} stopOpacity={0} />
        </linearGradient>
      </defs>
      <path d={areaPath} fill={`url(#${gradId})`} />
      <path
        d={linePath}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}
