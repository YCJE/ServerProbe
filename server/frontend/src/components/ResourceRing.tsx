import { memo } from 'react'
import type { CSSProperties } from 'react'

interface ResourceRingProps {
  /** 资源使用率（0-100），null 表示无数据 */
  value: number | null
  /** 标签（如 "CPU"、"内存"、"硬盘"） */
  label: string
  /** 自定义尺寸（px），默认 80 */
  size?: number
  /** 自定义描边宽度（px），默认 6 */
  strokeWidth?: number
  /** 详情页模式：使用更大的中心文字（text-[18px]） */
  detail?: boolean
}

/** NodeGet 色彩 + glow 配置 */
interface MetricStyle {
  color: string
  glow: string
}

/** 根据使用率返回 NodeGet 色彩和 glow（< 70% 绿，70-90% 橙，>= 90% 红，null 灰） */
function getMetricStyle(value: number | null): MetricStyle {
  if (value === null || value === undefined) {
    return {
      color: 'hsl(var(--muted-foreground) / 0.45)',
      glow: 'transparent',
    }
  }
  if (value > 90) {
    return { color: '#f56565', glow: 'rgba(245, 101, 101, 0.20)' }
  }
  if (value >= 70) {
    return { color: '#f6ad55', glow: 'rgba(246, 173, 85, 0.18)' }
  }
  return { color: '#42b983', glow: 'rgba(66, 185, 131, 0.18)' }
}

/**
 * 资源环形图组件
 *
 * SVG 圆环组件，用于展示 CPU/内存/磁盘等使用率：
 * - 颜色分级（NodeGet 色系）：<70% 绿(#42b983)、70-90% 橙(#f6ad55)、>90% 红(#f56565)、null 灰
 * - 中心显示百分比数值
 * - 进度变化使用 CSS transition 动画（stroke-dashoffset，0.5s ease）
 * - 外圈 glow 效果（boxShadow: 0 0 18px var(--metric-glow)）
 */
function ResourceRing({
  value,
  label,
  size = 80,
  strokeWidth = 6,
  detail = false,
}: ResourceRingProps) {
  // 限制在 0-100
  const clampedValue = value !== null ? Math.min(Math.max(value, 0), 100) : 0
  const { color, glow } = getMetricStyle(value)

  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  // 进度偏移量：值越大，偏移越小
  const offset = circumference - (clampedValue / 100) * circumference

  // 中心文本坐标
  const center = size / 2

  // 中心显示的百分比文本（null 时显示 '--'）
  const displayText = value !== null ? `${clampedValue.toFixed(0)}%` : '--'

  return (
    <div
      className="flex flex-col items-center gap-1"
      style={
        {
          width: size,
          '--metric-color': color,
          '--metric-glow': glow,
        } as CSSProperties
      }
    >
      <div
        className="relative"
        style={{
          width: size,
          height: size,
          borderRadius: '50%',
          boxShadow: glow !== 'transparent' ? '0 0 18px var(--metric-glow)' : 'none',
        }}
      >
        <svg
          width={size}
          height={size}
          viewBox={`0 0 ${size} ${size}`}
          role="img"
          aria-label={`${label} 使用率 ${value !== null ? clampedValue.toFixed(0) + '%' : '无数据'}`}
        >
          {/* 背景圆环 */}
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            stroke="hsl(var(--border))"
            strokeWidth={strokeWidth}
            opacity={0.95}
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
            className={`font-bold tabular-nums text-foreground ${
              detail ? 'text-[18px]' : 'text-[15px]'
            }`}
          >
            {displayText}
          </span>
        </div>
      </div>
      {/* 标签 */}
      <span className="mt-1 text-[10px] font-semibold tracking-wide text-muted-foreground">
        {label}
      </span>
    </div>
  )
}

export default memo(ResourceRing)
