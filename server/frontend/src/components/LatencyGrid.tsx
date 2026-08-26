import { useMemo, useState } from 'react'
import type { PingResult } from '@/types'

/**
 * 延迟小格子组件（NodeGet 风格核心组件）
 *
 * - 每行 = 一个探测目标，行内格子 = 最近 N 次探测
 * - 格子颜色 = 延迟分级，高度 = 延迟数值映射（行内归一化）
 * - 悬停格子显示该次探测的完整统计（时间 · 平均/最小/最大/抖动 · 丢包率）
 * - 行尾显示当前值（平均延迟 + 丢包率）
 */

/** 延迟格子数据点 */
export interface LatencyGridPoint {
  timestamp: number
  ping_data: PingResult[]
}

/** 格子颜色分级（柔和渐变，不刺眼） */
const CELL_COLORS = {
  great: '#4ade80', // <50ms 绿
  good: '#a3e635', // <100ms 黄绿
  fair: '#facc15', // <200ms 黄
  slow: '#fb923c', // <400ms 橙
  bad: '#f87171', // >=400ms 红
  timeout: '#9ca3af', // 超时/失败 灰
} as const

/** 格子高度范围（px，行内归一化） */
const MIN_CELL_HEIGHT = 4
const MAX_CELL_HEIGHT = 20

/** 根据平均延迟返回格子颜色 */
export function getLatencyCellColor(avgLatency: number): string {
  if (avgLatency < 50) return CELL_COLORS.great
  if (avgLatency < 100) return CELL_COLORS.good
  if (avgLatency < 200) return CELL_COLORS.fair
  if (avgLatency < 400) return CELL_COLORS.slow
  return CELL_COLORS.bad
}

/** 根据平均延迟返回文字颜色（与格子色阶一致） */
export function getLatencyTextColor(avgLatency: number): string {
  return getLatencyCellColor(avgLatency)
}

/**
 * 判断探测目标是否为 IPv6
 * 优先级：目标配置的 ip_version 元数据（面板配置） > 名称/地址启发式（命名约定解析）
 */
export function isIPv6Target(ping: PingResult): boolean {
  if (ping.ip_version === 4 || ping.ip_version === 6) {
    return ping.ip_version === 6
  }
  const target = (ping.target || '').trim()
  if (target.includes(':')) return true
  const name = (ping.name || '').toLowerCase()
  return /(^|[^a-z0-9])v?6([^a-z0-9]|$)/.test(name) || name.includes('ipv6')
}

/** 单次探测的格子数据 */
interface Cell {
  timestamp: number
  ping: PingResult
  timeout: boolean
  color: string
  height: number
}

/** 单行数据（一个探测目标） */
interface Row {
  name: string
  cells: Cell[]
  currentAvg: number | null
  currentLoss: number | null
}

/** 格式化时间戳为 HH:MM:SS */
function formatTime(ts: number): string {
  const d = new Date(ts * 1000)
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  return `${h}:${m}:${s}`
}

/** 从历史点构建行数据（按目标分组，取最近 maxCells 次） */
function buildRows(points: LatencyGridPoint[], ipVersion: 4 | 6, maxCells: number, maxRows?: number): Row[] {
  // 收集目标名（保持出现顺序）并按 IP 版本过滤
  const targetOrder: string[] = []
  const seen = new Set<string>()
  for (const point of points) {
    for (const ping of point.ping_data || []) {
      const isV6 = isIPv6Target(ping)
      if ((ipVersion === 6) !== isV6) continue
      const key = ping.name || ping.target
      if (!seen.has(key)) {
        seen.add(key)
        targetOrder.push(key)
      }
    }
  }

  const rows: Row[] = []
  for (const key of targetOrder) {
    // 取该目标最近 maxCells 次探测
    const samples: { timestamp: number; ping: PingResult }[] = []
    for (let i = points.length - 1; i >= 0 && samples.length < maxCells; i--) {
      const ping = (points[i].ping_data || []).find(
        (p) => (p.name || p.target) === key,
      )
      if (ping) samples.unshift({ timestamp: points[i].timestamp, ping })
    }

    const cells: Cell[] = []
    let rowMax = 0
    for (const s of samples) {
      const avg = s.ping.avg_latency
      const loss = s.ping.loss ?? 0
      const timeout = avg == null || avg < 0 || loss >= 100
      if (!timeout && avg > rowMax) rowMax = avg
      cells.push({
        timestamp: s.timestamp,
        ping: s.ping,
        timeout,
        color: '',
        height: MIN_CELL_HEIGHT,
      })
    }

    // 行内归一化高度映射（延迟越高格子越高）
    for (const cell of cells) {
      if (cell.timeout) {
        cell.color = CELL_COLORS.timeout
        cell.height = MIN_CELL_HEIGHT
      } else {
        const avg = cell.ping.avg_latency || 0
        cell.color = getLatencyCellColor(avg)
        const ratio = rowMax > 0 ? avg / rowMax : 1
        cell.height = Math.round(
          MIN_CELL_HEIGHT + ratio * (MAX_CELL_HEIGHT - MIN_CELL_HEIGHT),
        )
      }
    }

    // 当前值（最后一次有效探测）
    const lastValid = [...samples].reverse().find((s) => s.ping.avg_latency != null && s.ping.avg_latency >= 0)
    rows.push({
      name: key,
      cells,
      currentAvg: lastValid ? (lastValid.ping.avg_latency ?? null) : null,
      currentLoss: lastValid ? (lastValid.ping.loss ?? null) : null,
    })
  }

  return maxRows ? rows.slice(0, maxRows) : rows
}

/** 延迟格子图（每目标一行） */
export default function LatencyGrid({
  points,
  ipVersion = 4,
  maxCells = 24,
  maxRows,
  showTitle = true,
  compact = false,
}: {
  points: LatencyGridPoint[]
  /** 只展示该 IP 版本的目标（4 或 6） */
  ipVersion?: 4 | 6
  /** 每行最多格子数（最近 N 次探测） */
  maxCells?: number
  /** 最多展示行数（超出截断） */
  maxRows?: number
  /** 是否显示标题（"延迟格子图 (IPv4)"） */
  showTitle?: boolean
  /** 紧凑模式（卡片内使用：更小的行高与字号） */
  compact?: boolean
}) {
  const [hovered, setHovered] = useState<{ row: number; cell: number } | null>(null)

  const rows = useMemo(
    () => buildRows(points, ipVersion, maxCells, maxRows),
    [points, ipVersion, maxCells, maxRows],
  )

  if (rows.length === 0) {
    if (!showTitle) return null
    return (
      <div className="flex items-center justify-center py-3">
        <span className="text-[10px] text-muted-foreground/60">
          {ipVersion === 6 ? '无 IPv6 探测目标' : '无探测数据'}
        </span>
      </div>
    )
  }

  const hoveredCell =
    hovered !== null ? rows[hovered.row]?.cells[hovered.cell] : null

  return (
    <div>
      {showTitle && (
        <div className="mb-2 flex items-center justify-between">
          <span className="text-xs font-semibold text-foreground">
            延迟格子图
            <span className="ml-1.5 rounded bg-secondary px-1.5 py-0.5 text-[9px] font-medium text-muted-foreground">
              IPv{ipVersion}
            </span>
          </span>
          <span className="text-[10px] text-muted-foreground/70">
            最近 {maxCells} 次
          </span>
        </div>
      )}
      <div className="relative space-y-1">
        {rows.map((row, rowIndex) => (
          <div
            key={row.name}
            className={`flex items-center gap-2 ${compact ? 'text-[10px]' : 'text-xs'}`}
          >
            {/* 目标名 */}
            <span
              className={`w-[5.5rem] shrink-0 truncate text-muted-foreground ${compact ? 'text-[10px]' : 'text-xs'}`}
              title={row.name}
            >
              {row.name}
            </span>
            {/* 格子序列 */}
            <div className="flex min-w-0 flex-1 items-end justify-end gap-[2px]" style={{ height: MAX_CELL_HEIGHT }}>
              {row.cells.map((cell, cellIndex) => (
                <div
                  key={cellIndex}
                  className="min-w-0 flex-1 rounded-[2px] transition-all duration-300"
                  style={{
                    height: cell.height,
                    backgroundColor: cell.color,
                    opacity:
                      hovered !== null &&
                      !(hovered.row === rowIndex && hovered.cell === cellIndex)
                        ? 0.5
                        : 1,
                    cursor: 'pointer',
                  }}
                  onMouseEnter={() => setHovered({ row: rowIndex, cell: cellIndex })}
                  onMouseLeave={() => setHovered(null)}
                />
              ))}
            </div>
            {/* 当前值 */}
            <span
              className={`w-[4.5rem] shrink-0 text-right tabular-nums ${compact ? 'text-[10px]' : 'text-xs'}`}
            >
              {row.currentAvg !== null ? (
                <>
                  <span className="font-semibold" style={{ color: getLatencyTextColor(row.currentAvg) }}>
                    {row.currentAvg.toFixed(1)}ms
                  </span>
                  {row.currentLoss != null && row.currentLoss > 0 && (
                    <span className="ml-1 text-muted-foreground/70">
                      {row.currentLoss.toFixed(0)}%
                    </span>
                  )}
                </>
              ) : (
                <span className="text-muted-foreground/50">超时</span>
              )}
            </span>
          </div>
        ))}

        {/* 悬停 Tooltip */}
        {hoveredCell && (
          <div className="pointer-events-none absolute -top-2 left-1/2 z-10 -translate-x-1/2 -translate-y-full whitespace-nowrap rounded-md border border-border bg-card px-2.5 py-1.5 text-[10px] shadow-tooltip">
            <div className="mb-0.5 font-semibold text-foreground">{hoveredCell.ping.name || hoveredCell.ping.target}</div>
            {hoveredCell.timeout ? (
              <div className="text-muted-foreground">
                {formatTime(hoveredCell.timestamp)} · 探测超时/失败
              </div>
            ) : (
              <div className="text-muted-foreground">
                {formatTime(hoveredCell.timestamp)} · 均{' '}
                <span className="font-medium text-foreground">
                  {(hoveredCell.ping.avg_latency ?? 0).toFixed(1)}ms
                </span>{' '}
                / 最小 {hoveredCell.ping.min_latency?.toFixed(1) ?? '-'} / 最大{' '}
                {hoveredCell.ping.max_latency?.toFixed(1) ?? '-'} / 抖动{' '}
                {hoveredCell.ping.jitter?.toFixed(1) ?? '-'} · 丢包{' '}
                <span className="font-medium text-foreground">
                  {(hoveredCell.ping.loss ?? 0).toFixed(0)}%
                </span>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
