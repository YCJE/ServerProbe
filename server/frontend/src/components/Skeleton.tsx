interface SkeletonProps {
  /** 骨架形态：统计卡 / 表格 / 图表 / 表单区块 */
  variant: 'card' | 'table' | 'chart' | 'section'
  /** card: 卡片数量（默认 4）；table: 占位行数（默认 5）；chart/section: 高度 px（默认 300/160） */
  count?: number
  height?: number
  className?: string
}

/** 统一骨架屏：card-soft 容器内 animate-pulse 占位，高度对齐真实内容避免 CLS */
export default function Skeleton({
  variant,
  count = variant === 'card' ? 4 : 5,
  height = variant === 'section' ? 160 : 300,
  className = '',
}: SkeletonProps) {
  if (variant === 'section') {
    return (
      <div className={`card-soft animate-pulse p-5 ${className}`} style={{ minHeight: `${height}px` }}>
        <div className="h-4 w-28 rounded bg-muted" />
        <div className="mt-5 h-3 w-full rounded bg-muted" />
        <div className="mt-2.5 h-3 w-4/5 rounded bg-muted" />
        <div className="mt-2.5 h-3 w-3/5 rounded bg-muted" />
      </div>
    )
  }

  if (variant === 'card') {
    return (
      <div className={`grid grid-cols-2 gap-3 md:grid-cols-4 ${className}`}>
        {Array.from({ length: count }, (_, i) => (
          <div key={i} className="card-soft p-4">
            <div className="h-3 w-16 animate-pulse rounded bg-muted" />
            <div className="mt-3 h-7 w-24 animate-pulse rounded bg-muted" />
          </div>
        ))}
      </div>
    )
  }

  if (variant === 'chart') {
    return (
      <div
        className={`flex w-full animate-pulse items-end gap-1 rounded-md bg-muted/60 p-4 ${className}`}
        style={{ height: `${height}px` }}
      >
        {Array.from({ length: 24 }, (_, i) => (
          <div
            key={i}
            className="flex-1 rounded-sm bg-muted"
            style={{ height: `${20 + ((i * 37) % 60)}%` }}
          />
        ))}
      </div>
    )
  }

  return (
    <div className={className}>
      <div className="flex items-center gap-6 border-b border-border px-4 py-3">
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="h-2.5 w-20 animate-pulse rounded bg-muted" />
        ))}
      </div>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="flex items-center gap-6 border-b border-dashed border-border px-4 py-3.5">
          {Array.from({ length: 5 }, (_, j) => (
            <div
              key={j}
              className="h-2.5 animate-pulse rounded bg-muted"
              style={{ width: `${36 + ((i + j * 13) % 40)}px` }}
            />
          ))}
        </div>
      ))}
    </div>
  )
}
