import { memo } from 'react'

interface ResourceRingProps {
  /** 资源使用率（0-100） */
  value: number
  /** 标签（如 "CPU"、"内存"、"硬盘"） */
  label: string
  /** 自定义尺寸（px），默认 80 */
  size?: number
  /** 自定义描边宽度（px），默认 6 */
  strokeWidth?: number
}

/** 根据使用率返回颜色（Apple 系统色） */
function getColorForValue(value: number): string {
  if (value > 90) return '#FF3B30' // 红色
  if (value >= 70) return '#FF9500' // 橙色
  return '#34C759' // 绿色
}

/**
 * 资源环形图组件
 *
 * SVG 圆环组件，用于展示 CPU/内存/磁盘等使用率：
 * - 颜色分级：<70% 绿色(#34C759)、70-90% 橙色(#FF9500)、>90% 红色(#FF3B30)
 * - 中心显示百分比数值
 * - 进度变化使用 CSS transition 动画（stroke-dashoffset，0.5s ease）
 * - 外圈微弱发光效果（使用 CSS filter drop-shadow）
 */
function ResourceRing({
  value,
  label,
  size = 80,
  strokeWidth = 6,
}: ResourceRingProps) {
  // 限制在 0-100
  const clampedValue = Math.min(Math.max(value, 0), 100)
  const color = getColorForValue(clampedValue)

  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  // 进度偏移量：值越大，偏移越小
  const offset = circumference - (clampedValue / 100) * circumference

  // 中心文本坐标
  const center = size / 2

  // 中心显示的百分比文本（简单字符串拼接，无需 useMemo）
  const displayText = `${clampedValue.toFixed(0)}%`

  return (
    <div
      className="flex flex-col items-center gap-1"
      style={{ width: size }}
    >
      <div
        className="relative"
        style={{ width: size, height: size }}
      >
        <svg
          width={size}
          height={size}
          viewBox={`0 0 ${size} ${size}`}
          style={{
            // 外圈微弱发光效果
            filter: `drop-shadow(0 0 4px ${color}40)`,
          }}
        >
          {/* 背景圆环 */}
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke="hsl(var(--secondary))"
            strokeWidth={strokeWidth}
          />
          {/* 进度圆环 - 使用 CSS transition 动画 */}
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke={color}
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={offset}
            // 旋转 -90 度，让进度从顶部开始
            transform={`rotate(-90 ${center} ${center})`}
            style={{
              transition: 'stroke-dashoffset 0.5s ease, stroke 0.5s ease',
            }}
          />
        </svg>
        {/* 中心百分比数值 */}
        <div
          className="absolute inset-0 flex items-center justify-center"
          style={{ pointerEvents: 'none' }}
        >
          <span
            className="font-semibold tabular-nums text-foreground"
            style={{ fontSize: size * 0.22 }}
          >
            {displayText}
          </span>
        </div>
      </div>
      {/* 标签 */}
      <span className="text-[10px] text-muted-foreground">{label}</span>
    </div>
  )
}

export default memo(ResourceRing)
