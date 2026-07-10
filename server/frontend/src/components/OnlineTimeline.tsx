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

/** 格子颜色 */
const CELL_COLORS: Record<CellStatus, string> = {
  online: '#34C759',     // 绿色
  offline: '#FF3B30',    // 红色
  empty: 'hsl(var(--secondary))', // 灰色
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
 * 在线状态时间线组件
 *
 * - 80 格时间线，每格 3 分钟，共 4 小时
 * - 根据历史采样数据填充格子（在线=绿色，离线=红色，无数据=灰色）
 * - 悬停显示时间范围和状态
 * - 底部显示可用性百分比
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
      // 找到该时间格子内或之前最近的数据点
      let lastPoint: OnlineTimelinePoint | null = null
      for (const point of sortedPoints) {
        if (point.timestamp <= cell.endTime) {
          // 数据点在格子结束时间之前
          if (point.timestamp >= cell.startTime) {
            // 数据点在格子内，记录并继续遍历到最后一个落在范围内的点
            // （数据已按时间升序排序，最后一个满足条件的点即为最新点）
            lastPoint = point
          } else {
            // 数据点在格子之前，记录但继续查找更近的
            lastPoint = point
          }
        } else {
          // 数据点在格子之后，停止查找（性能优化）
          break
        }
      }

      // 反向查找更准确：找到格子时间范围内或之前最近的数据点
      if (!lastPoint || lastPoint.timestamp < cell.startTime) {
        // 重新查找：找到 <= cell.endTime 的最后一个数据点
        let found: OnlineTimelinePoint | null = null
        for (const point of sortedPoints) {
          if (point.timestamp <= cell.endTime) {
            found = point
          } else {
            break
          }
        }
        if (found && found.timestamp >= cell.startTime - cellSeconds * 2) {
          // 数据点在格子附近（两个格子内），使用其状态
          lastPoint = found
        } else {
          // 数据点太远或不存在，清除 lastPoint，该格子标记为 empty
          lastPoint = null
        }
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
    <div className={`relative ${className}`}>
      {/* 标题 + 可用性 */}
      <div className="mb-3 flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">
          在线状态时间线（最近 4 小时）
        </span>
        <span className="text-xs">
          <span className="text-muted-foreground">可用性: </span>
          <span
            className={`font-semibold ${
              availability >= 99
                ? 'text-success'
                : availability >= 90
                  ? 'text-warning'
                  : 'text-destructive'
            }`}
          >
            {availability.toFixed(2)}%
          </span>
        </span>
      </div>

      {/* 时间线格子 */}
      <div className="flex flex-nowrap overflow-hidden gap-0.5">
        {cells.map((cell) => (
          <div
            key={cell.index}
            className="relative cursor-pointer transition-all"
            style={{
              width: `calc((100% - ${(totalCells - 1) * 2}px) / ${totalCells})`,
              height: 24,
              backgroundColor: CELL_COLORS[cell.status],
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
          <span className="inline-block h-2 w-2 rounded-sm" style={{ backgroundColor: CELL_COLORS.online }} />
          <span>在线</span>
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-2 w-2 rounded-sm" style={{ backgroundColor: CELL_COLORS.offline }} />
          <span>离线</span>
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-2 w-2 rounded-sm" style={{ backgroundColor: CELL_COLORS.empty }} />
          <span>无数据</span>
        </span>
      </div>

      {/* 悬停 Tooltip */}
      {hoveredCellData && (
        <div className="pointer-events-none absolute -top-2 left-1/2 z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-card px-3 py-2 text-xs shadow-lg">
          <div className="whitespace-nowrap font-medium text-foreground">
            {formatTime(hoveredCellData.startTime)} - {formatTime(hoveredCellData.endTime)}
          </div>
          <div className="mt-1 flex items-center gap-1.5 whitespace-nowrap">
            <span
              className="inline-block h-2 w-2 rounded-sm"
              style={{ backgroundColor: CELL_COLORS[hoveredCellData.status] }}
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
