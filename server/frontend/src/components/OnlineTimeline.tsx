import { memo, useEffect, useMemo, useState } from 'react'

/** 单个时间点的在线状态 */
export interface OnlineTimelinePoint {
  /** 时间戳（秒） */
  timestamp: number
  /** 在线状态（1=在线，0=离线） */
  online: number | boolean
}

interface OnlineTimelineProps {
  /** 历史采样数据点列表 */
  points: OnlineTimelinePoint[]
  /** 时间线总格数，默认 80 */
  totalCells?: number
  /** 每格时间跨度（秒），默认 180（3 分钟） */
  cellSeconds?: number
  /** 自定义类名 */
  className?: string
}

/** 格子状态 */
type CellStatus = 'online' | 'offline' | 'empty'

/** 格子 className（NodeGet 风格） */
const CELL_CLASS: Record<CellStatus, string> = {
  online: 'bg-primary shadow-[0_0_0_1px_hsl(var(--primary)/0.09)]',
  offline: 'bg-border/90',
  empty: 'bg-muted/40',
}

/** 格子状态文本 */
const CELL_LABELS: Record<CellStatus, string> = {
  online: '在线',
  offline: '离线',
  empty: '无数据',
}

/** 时间格子 */
interface TimelineCell {
  /** 格子索引 */
  index: number
  /** 格子起始时间（秒） */
  startTime: number
  /** 格子结束时间（秒） */
  endTime: number
  /** 状态 */
  status: CellStatus
}

/** 格式化时间为 MM/DD HH:MM */
function formatTime(ts: number): string {
  return new Date(ts * 1000).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/**
 * 在线状态时间线组件（NodeGet 风格）
 *
 * - 80 格时间线，每格 3 分钟，共 4 小时
 * - 虚线边框容器，标题使用 primary 色
 * - 在线格子 bg-primary，离线格子 bg-border/90，空格子 bg-muted/40
 * - 悬停显示时间范围和状态
 * - 右侧显示可用性百分比
 */
function OnlineTimeline({
  points,
  totalCells = 80,
  cellSeconds = 180,
  className = '',
}: OnlineTimelineProps) {
  const [hoveredCell, setHoveredCell] = useState<number | null>(null)

  // 每 60 秒刷新一次 "now" 引用，防止长时间挂载后时间范围漂移
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000))
  useEffect(() => {
    const timer = setInterval(() => setNowTick(Math.floor(Date.now() / 1000)), 60000)
    return () => clearInterval(timer)
  }, [])

  // 计算格子数据
  const cells = useMemo<TimelineCell[]>(() => {
    const now = nowTick
    const startTime = now - totalCells * cellSeconds
    const result: TimelineCell[] = []

    // 初始化所有格子
    for (let i = 0; i < totalCells; i++) {
      const cellStart = startTime + i * cellSeconds
      const cellEnd = cellStart + cellSeconds
      result.push({
        index: i,
        startTime: cellStart,
        endTime: cellEnd,
        status: 'empty',
      })
    }

    if (!points || points.length === 0) return result

    // 将数据点分配到格子中
    // 对每个格子，找到落在其时间范围内的数据点，取最后一个数据点的状态
    // 如果没有数据点落在该格子，则根据前后数据点推断状态（向前查找最近的数据点）
    const sortedPoints = [...points].sort((a, b) => a.timestamp - b.timestamp)

    for (const cell of result) {
      if (sortedPoints.length === 0) continue

      // 找到 <= cell.endTime 的最后一个数据点
      let lastPoint: OnlineTimelinePoint | null = null
      for (const point of sortedPoints) {
        if (point.timestamp <= cell.endTime) {
          lastPoint = point
        } else {
          break
        }
      }

      // 数据点太远（超过两个格子之前）则视为无数据
      if (lastPoint && lastPoint.timestamp < cell.startTime - cellSeconds * 2) {
        lastPoint = null
      }

      if (lastPoint) {
        const isOnline = lastPoint.online === 1 || lastPoint.online === true
        cell.status = isOnline ? 'online' : 'offline'
      }
    }

    return result
  }, [points, totalCells, cellSeconds, nowTick])

  // 可用性百分比
  const availability = useMemo(() => {
    const cellsWithData = cells.filter((c) => c.status !== 'empty')
    if (cellsWithData.length === 0) return 0
    const onlineCells = cellsWithData.filter((c) => c.status === 'online').length
    return (onlineCells / cellsWithData.length) * 100
  }, [cells])

  // 悬停的格子
  const hoveredCellData = hoveredCell !== null ? cells[hoveredCell] : null

  return (
    <div className={`relative rounded-md border-dashed border border-border bg-secondary/35 p-4 ${className}`}>
      {/* 标题行: Activity 图标 + "在线状态" + 右侧可用率 */}
      <div className="mb-3 flex items-center justify-between">
        <span className="flex items-center gap-1.5 text-sm font-bold text-primary">
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M22 12h-4l-3 9L9 3l-3 9H2"
            />
          </svg>
          在线状态
        </span>
        <span className="text-sm">
          <span className="text-muted-foreground">可用率 </span>
          <span className="font-bold text-primary">{availability.toFixed(2)}%</span>
        </span>
      </div>

      {/* 时间线格子 - grid 布局 */}
      <div
        className="grid gap-[3px]"
        style={{ gridTemplateColumns: `repeat(${totalCells}, minmax(0, 1fr))` }}
      >
        {cells.map((cell) => (
          <div
            key={cell.index}
            className={`relative cursor-pointer transition-all ${CELL_CLASS[cell.status]}`}
            style={{
              height: 24,
              borderRadius: 2,
              opacity: hoveredCell !== null && hoveredCell !== cell.index ? 0.5 : 1,
              transform: hoveredCell === cell.index ? 'scaleY(1.15)' : 'scaleY(1)',
              transition: 'opacity 0.15s, transform 0.15s',
            }}
            onMouseEnter={() => setHoveredCell(cell.index)}
            onMouseLeave={() => setHoveredCell(null)}
          />
        ))}
      </div>

      {/* 时间轴标签 */}
      <div className="mt-1.5 flex justify-between text-[10px] text-muted-foreground">
        <span>{formatTime(cells[0]?.startTime || 0)}</span>
        <span>{formatTime(cells[Math.floor(totalCells / 2)]?.startTime || 0)}</span>
        <span>现在</span>
      </div>

      {/* 图例 */}
      <div className="mt-2 flex items-center gap-3 text-[10px] text-muted-foreground">
        <span className="flex items-center gap-1">
          <span className={`inline-block h-2 w-2 rounded-sm ${CELL_CLASS.online}`} />
          <span>在线</span>
        </span>
        <span className="flex items-center gap-1">
          <span className={`inline-block h-2 w-2 rounded-sm ${CELL_CLASS.offline}`} />
          <span>离线</span>
        </span>
        <span className="flex items-center gap-1">
          <span className={`inline-block h-2 w-2 rounded-sm ${CELL_CLASS.empty}`} />
          <span>无数据</span>
        </span>
      </div>

      {/* 悬停 Tooltip */}
      {hoveredCellData && (
        <div className="pointer-events-none absolute -top-2 left-1/2 z-10 -translate-x-1/2 -translate-y-full rounded-sm border border-border bg-card px-3 py-2.5 text-xs shadow-tooltip ring-1 ring-black/5">
          <div className="whitespace-nowrap font-medium text-foreground">
            {formatTime(hoveredCellData.startTime)} - {formatTime(hoveredCellData.endTime)}
          </div>
          <div className="mt-1 flex items-center gap-1.5 whitespace-nowrap">
            <span
              className={`inline-block h-2 w-2 rounded-sm ${CELL_CLASS[hoveredCellData.status]}`}
            />
            <span className="text-muted-foreground">状态: </span>
            <span className="font-medium text-foreground">{CELL_LABELS[hoveredCellData.status]}</span>
          </div>
        </div>
      )}
    </div>
  )
}

export default memo(OnlineTimeline)
