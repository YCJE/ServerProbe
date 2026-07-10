import { memo, useMemo, useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ServerData, PingResult } from '@/types'
import {
  formatSpeed,
  formatTraffic,
  formatUptime,
  formatRelativeTime,
  getRegionFromServer,
  getFlagEmoji,
} from '@/lib/utils'
import ResourceRing from '@/components/ResourceRing'
import DistroIcon from '@/components/DistroIcon'
import StatusDot from '@/components/StatusDot'
import { useAnimatedNumber } from '@/hooks/useAnimatedNumber'
import { useInViewport } from '@/hooks/useInViewport'

interface ServerCardProps {
  server: ServerData
  /**
   * 链接基础路径。
   * - 公开页面传 "" (空字符串)，链接为 `/server/:id`
   * - 管理页面传 "/admin"，链接为 `/admin/server/:id`
   * 默认为 "/admin"（保持向后兼容）
   */
  basePath?: string
}

/** 卡片本地历史数据点 */
interface CardHistoryPoint {
  timestamp: number
  ping_data: PingResult[]
  online: boolean
}

/** 最大缓存历史点数（约 12 分钟，按 3s 上报间隔） */
const MAX_HISTORY_POINTS = 240

/** 紧凑图表桶数 */
const COMPACT_BUCKETS = 30

/** 延迟质量桶颜色（与 LatencyQualityBar 保持一致） */
const BUCKET_COLORS = {
  deepGreen: '#69BE7B',
  lightGreen: '#A7D879',
  lightYellow: '#E8CC68',
  deepYellow: '#EFA85F',
  lightRed: '#E98686',
  deepRed: '#D96B6B',
  empty: 'rgba(148, 163, 184, 0.22)',
} as const

/** 根据平均延迟和丢包率返回桶颜色 */
function getBucketColor(avgLatency: number, avgLoss: number, hasData: boolean): string {
  if (!hasData) return BUCKET_COLORS.empty
  if (avgLoss > 50) return BUCKET_COLORS.deepRed
  if (avgLatency <= 50) return BUCKET_COLORS.deepGreen
  if (avgLatency <= 100) return BUCKET_COLORS.lightGreen
  if (avgLatency <= 180) return BUCKET_COLORS.lightYellow
  if (avgLatency <= 300) return BUCKET_COLORS.deepYellow
  return BUCKET_COLORS.lightRed
}

/** 延迟桶聚合结果 */
interface LatencyBucket {
  index: number
  color: string
  hasData: boolean
  avgLatency: number
  avgLoss: number
  count: number
}

/** 在线状态格子类型 */
type OnlineStatus = 'online' | 'offline' | 'empty'

/** 在线状态格子 */
interface OnlineCell {
  index: number
  status: OnlineStatus
}

/** 在线状态格子颜色（内联样式用） */
function getOnlineCellColor(status: OnlineStatus): string {
  switch (status) {
    case 'online':
      return 'hsl(var(--primary))'
    case 'offline':
      return 'hsl(var(--destructive) / 0.45)'
    case 'empty':
      return 'hsl(var(--muted) / 0.3)'
  }
}

/**
 * 紧凑延迟质量分布条（卡片专用）
 *
 * - 30 个时间桶，自适应历史数据范围
 * - 高度 16px，悬停显示延迟和丢包率
 * - 颜色与 LatencyQualityBar 保持一致
 */
function CompactLatencyBar({
  points,
  online,
  currentAvgLatency,
}: {
  points: CardHistoryPoint[]
  online: boolean
  currentAvgLatency: number | null
}) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null)

  const buckets = useMemo<LatencyBucket[]>(() => {
    const result: LatencyBucket[] = Array.from({ length: COMPACT_BUCKETS }, (_, i) => ({
      index: i,
      color: BUCKET_COLORS.empty,
      hasData: false,
      avgLatency: 0,
      avgLoss: 0,
      count: 0,
    }))

    if (points.length === 0) return result

    const oldestTs = points[0].timestamp
    const newestTs = points[points.length - 1].timestamp
    const range = newestTs - oldestTs || 1

    for (const point of points) {
      const ratio = (point.timestamp - oldestTs) / range
      const bucketIdx = Math.min(
        Math.max(0, Math.floor(ratio * COMPACT_BUCKETS)),
        COMPACT_BUCKETS - 1,
      )
      const bucket = result[bucketIdx]
      const pings = point.ping_data || []
      if (pings.length === 0) continue

      let latencySum = 0
      let lossSum = 0
      let validCount = 0
      for (const ping of pings) {
        if (ping.avg_latency != null && ping.avg_latency >= 0) {
          latencySum += ping.avg_latency
          lossSum += ping.loss || 0
          validCount++
        }
      }

      if (validCount > 0) {
        const pointAvgLatency = latencySum / validCount
        const pointAvgLoss = lossSum / validCount
        bucket.avgLatency =
          (bucket.avgLatency * bucket.count + pointAvgLatency) / (bucket.count + 1)
        bucket.avgLoss =
          (bucket.avgLoss * bucket.count + pointAvgLoss) / (bucket.count + 1)
        bucket.count++
        bucket.hasData = true
      }
    }

    for (const bucket of result) {
      if (bucket.hasData) {
        bucket.color = getBucketColor(bucket.avgLatency, bucket.avgLoss, true)
      }
    }

    return result
  }, [points])

  const hoveredBucket = hoveredIdx !== null ? buckets[hoveredIdx] : null

  return (
    <div>
      {/* 标题行 */}
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">延迟质量分布</span>
        <span className="text-[10px] font-medium tabular-nums text-foreground/70">
          {online && currentAvgLatency !== null
            ? `${currentAvgLatency.toFixed(0)}ms`
            : '---'}
        </span>
      </div>
      {/* 桶条 */}
      <div className="relative">
        <div className="flex items-end gap-[2px]" style={{ height: 16 }}>
          {buckets.map((bucket) => (
            <div
              key={bucket.index}
              className="flex-1 cursor-pointer rounded-[2px] transition-all"
              style={{
                height: '100%',
                backgroundColor: bucket.color,
                opacity: hoveredIdx !== null && hoveredIdx !== bucket.index ? 0.45 : 1,
                transform: hoveredIdx === bucket.index ? 'scaleY(1.18)' : 'scaleY(1)',
                transitionProperty: 'opacity, transform',
                transitionDuration: '150ms',
              }}
              onMouseEnter={() => setHoveredIdx(bucket.index)}
              onMouseLeave={() => setHoveredIdx(null)}
            />
          ))}
        </div>
        {/* 悬停 Tooltip */}
        {hoveredBucket && (
          <div className="pointer-events-none absolute -top-1.5 left-1/2 z-10 -translate-x-1/2 -translate-y-full rounded-md border border-border bg-card px-2.5 py-1.5 text-[10px] shadow-tooltip">
            {hoveredBucket.hasData ? (
              <div className="whitespace-nowrap">
                <span className="font-medium text-foreground">
                  {hoveredBucket.avgLatency.toFixed(0)}ms
                </span>
                <span className="ml-2 text-muted-foreground">
                  丢包 {hoveredBucket.avgLoss.toFixed(0)}%
                </span>
              </div>
            ) : (
              <span className="text-muted-foreground">无数据</span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/**
 * 紧凑在线状态时间线（卡片专用）
 *
 * - 30 个格子，自适应历史数据范围
 * - 高度 8px，右侧显示可用率百分比
 * - 在线格子 primary 色，离线格子 destructive/45%，空格子 muted/30%
 */
function CompactOnlineTimeline({ points }: { points: CardHistoryPoint[] }) {
  const cells = useMemo<OnlineCell[]>(() => {
    const result: OnlineCell[] = Array.from({ length: COMPACT_BUCKETS }, (_, i) => ({
      index: i,
      status: 'empty' as OnlineStatus,
    }))

    if (points.length === 0) return result

    const oldestTs = points[0].timestamp
    const newestTs = points[points.length - 1].timestamp
    const range = newestTs - oldestTs || 1

    for (const point of points) {
      const ratio = (point.timestamp - oldestTs) / range
      const cellIdx = Math.min(
        Math.max(0, Math.floor(ratio * COMPACT_BUCKETS)),
        COMPACT_BUCKETS - 1,
      )
      result[cellIdx].status = point.online ? 'online' : 'offline'
    }

    return result
  }, [points])

  const availability = useMemo(() => {
    const withData = cells.filter((c) => c.status !== 'empty')
    if (withData.length === 0) return 0
    return (withData.filter((c) => c.status === 'online').length / withData.length) * 100
  }, [cells])

  return (
    <div>
      {/* 标题行 */}
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">在线状态</span>
        <span className="text-[10px] font-medium tabular-nums text-primary">
          {availability.toFixed(0)}%
        </span>
      </div>
      {/* 格子条 */}
      <div className="flex items-center gap-2">
        <div className="flex flex-1 gap-[2px]" style={{ height: 8 }}>
          {cells.map((cell) => (
            <div
              key={cell.index}
              className="flex-1 rounded-[1.5px] transition-all"
              style={{
                height: '100%',
                backgroundColor: getOnlineCellColor(cell.status),
              }}
            />
          ))}
        </div>
      </div>
    </div>
  )
}

/** 服务器卡片组件（NodeGet 风格） */
function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()

  // 视口懒加载：仅当卡片进入视口时才渲染延迟和在线状态部分
  const { ref: viewportRef, isInViewport } = useInViewport<HTMLDivElement>(320)

  // 本地历史数据（按 WS 上报间隔累积，最多保留 MAX_HISTORY_POINTS 个点）
  const [history, setHistory] = useState<CardHistoryPoint[]>([])
  const lastSeenRef = useRef<number>(0)

  // 当 last_seen 变化时，追加历史数据点
  useEffect(() => {
    if (server.last_seen && server.last_seen !== lastSeenRef.current) {
      lastSeenRef.current = server.last_seen
      setHistory((prev) =>
        [
          ...prev,
          {
            timestamp: server.last_seen,
            ping_data: server.ping_data || [],
            online: server.online,
          },
        ].slice(-MAX_HISTORY_POINTS),
      )
    }
  }, [server.last_seen, server.ping_data, server.online])

  // 记录上一次网速值，用于判断上升/下降趋势
  const prevNetRxRef = useRef<number>(server.net_rx || 0)
  const prevNetTxRef = useRef<number>(server.net_tx || 0)
  const [rxRising, setRxRising] = useState<boolean>(true)
  const [txRising, setTxRising] = useState<boolean>(true)

  // 网速数值动画
  const { value: animatedRx } = useAnimatedNumber(server.net_rx || 0)
  const { value: animatedTx } = useAnimatedNumber(server.net_tx || 0)

  // 监听网速变化，判断上升/下降
  useEffect(() => {
    const prev = prevNetRxRef.current
    const cur = server.net_rx || 0
    if (cur !== prev) {
      setRxRising(cur >= prev)
      prevNetRxRef.current = cur
    }
  }, [server.net_rx])

  useEffect(() => {
    const prev = prevNetTxRef.current
    const cur = server.net_tx || 0
    if (cur !== prev) {
      setTxRising(cur >= prev)
      prevNetTxRef.current = cur
    }
  }, [server.net_tx])

  const handleClick = () => {
    const selection = window.getSelection()
    if (selection && selection.toString().length > 0) return
    navigate(`${basePath}/server/${server.id}`)
  }

  // 国旗 emoji
  const flag = useMemo(() => {
    const region = getRegionFromServer(server)
    return region ? getFlagEmoji(region) : ''
  }, [server.display_name, server.hostname])

  // 磁盘使用率
  const diskUsage = server.disk_usage || 0

  // 内存使用率
  const memUsagePercent = server.mem_total > 0
    ? ((server.mem_used || 0) / server.mem_total) * 100
    : server.mem || 0

  // 当前平均延迟（所有探测目标的平均值）
  const currentAvgLatency = useMemo(() => {
    const pings = server.ping_data || []
    const valid = pings.filter((p) => p.avg_latency != null && p.avg_latency >= 0)
    if (valid.length === 0) return null
    return valid.reduce((sum, p) => sum + (p.avg_latency || 0), 0) / valid.length
  }, [server.ping_data])

  const displayName = server.display_name || server.hostname

  // 虚拟化 Badge（仅在存在数据时显示）
  const virtualization = server.virtualization
  const showVirtualizationBadge =
    virtualization && virtualization !== 'None' && virtualization !== 'none'

  // OS 显示文本
  const osText = server.os || ''

  return (
    <div
      ref={viewportRef}
      onClick={handleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          navigate(`${basePath}/server/${server.id}`)
        }
      }}
      role="button"
      tabIndex={0}
      className={`group relative flex min-h-[360px] cursor-pointer flex-col rounded-2xl border border-border p-4 card-soft node-card-hover animate-fade-in focus:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:min-h-[420px] sm:p-5 ${
        server.online ? '' : 'opacity-80'
      }`}
    >
      {/* 1. 头部行：StatusDot + DistroIcon + 名称 + 国旗 */}
      <div className="flex items-center gap-2 border-b border-dashed border-border pb-3">
        <StatusDot online={server.online} size="md" />
        <DistroIcon distro={server.distro} os={server.os} size={16} />
        <h3 className="min-w-0 flex-1 truncate text-[14px] font-bold text-foreground sm:text-[15px]">
          {displayName}
        </h3>
        {showVirtualizationBadge && (
          <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[9px] font-medium text-muted-foreground">
            {virtualization}
          </span>
        )}
        {flag && <span className="shrink-0 text-sm leading-none">{flag}</span>}
      </div>

      {/* 2. OS / 虚拟化 信息行 */}
      <div className="mt-2 truncate text-xs font-bold text-muted-foreground">
        {osText}
        {showVirtualizationBadge ? ` · ${virtualization}` : ''}
      </div>

      {/* 3. 资源环形图：CPU / 内存 / 硬盘（三个圆环） */}
      <div className="mt-3 grid grid-cols-3 gap-x-2 gap-y-3">
        <ResourceRing label="CPU" value={server.cpu || 0} size={80} />
        <ResourceRing label="内存" value={memUsagePercent} size={80} />
        <ResourceRing label="硬盘" value={diskUsage} size={80} />
      </div>

      {/* 4. 网络信息面板：实时速率（动画过渡）+ 累计流量 */}
      <div className="mt-3 rounded-xl border border-dashed border-border/80 px-3 py-2.5">
        {/* 实时速率行 - 使用动画数值，上升显示主题色，下降显示橙色 */}
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <span style={{ color: rxRising ? 'hsl(var(--primary))' : '#FF9500' }}>↓</span>
            <span
              className="font-medium tabular-nums"
              style={{ color: rxRising ? 'hsl(var(--primary))' : '#FF9500' }}
            >
              {server.online ? formatSpeed(animatedRx) : '---'}
            </span>
          </span>
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <span style={{ color: txRising ? 'hsl(var(--primary))' : '#FF9500' }}>↑</span>
            <span
              className="font-medium tabular-nums"
              style={{ color: txRising ? 'hsl(var(--primary))' : '#FF9500' }}
            >
              {server.online ? formatSpeed(animatedTx) : '---'}
            </span>
          </span>
        </div>
        {/* 累计流量行 */}
        <div className="mt-1 flex items-center justify-between border-t border-dashed border-border/60 pt-1 text-[10px]">
          <span className="flex items-center gap-1 text-muted-foreground">
            <span style={{ color: 'hsl(var(--primary))' }}>↓</span>
            <span className="font-medium tabular-nums text-foreground/70">
              {server.online ? formatTraffic(server.total_rx || 0) : '---'}
            </span>
          </span>
          <span className="flex items-center gap-1 text-muted-foreground">
            <span style={{ color: 'hsl(var(--accent-foreground))' }}>↑</span>
            <span className="font-medium tabular-nums text-foreground/70">
              {server.online ? formatTraffic(server.total_tx || 0) : '---'}
            </span>
          </span>
        </div>
      </div>

      {/* 5. 延迟质量分布 + 在线状态（仅在卡片进入视口时渲染） */}
      <div className="mt-3 rounded-xl border border-dashed border-border/80 px-3 py-2.5">
        {isInViewport ? (
          <div className="space-y-2.5">
            <CompactLatencyBar
              points={history}
              online={server.online}
              currentAvgLatency={currentAvgLatency}
            />
            <CompactOnlineTimeline points={history} />
          </div>
        ) : (
          // 占位符：不在视口时不渲染，显示占位高度避免布局抖动
          <div className="flex h-16 items-center justify-center">
            <span className="text-[10px] text-muted-foreground/40">加载中...</span>
          </div>
        )}
      </div>

      {/* 6. 底部信息栏：运行时间 · 最后更新 */}
      <div className="mt-auto space-y-1.5 border-t border-dashed border-border pt-3 text-xs tabular-nums text-muted-foreground">
        <div className="flex items-center justify-between gap-1">
          <span className="shrink-0">
            {server.online ? formatUptime(server.uptime) : '---'}
          </span>
          <span className="shrink-0 whitespace-nowrap">
            {formatRelativeTime(server.last_seen)}
          </span>
        </div>
      </div>
    </div>
  )
}

export default memo(ServerCard, (prev, next) => {
  const a = prev.server
  const b = next.server
  return (
    a.id === b.id &&
    a.online === b.online &&
    a.cpu === b.cpu &&
    a.cpu_model === b.cpu_model &&
    a.cpu_cores === b.cpu_cores &&
    a.mem === b.mem &&
    a.mem_total === b.mem_total &&
    a.mem_used === b.mem_used &&
    a.net_rx === b.net_rx &&
    a.net_tx === b.net_tx &&
    a.total_rx === b.total_rx &&
    a.total_tx === b.total_tx &&
    a.disk_usage === b.disk_usage &&
    a.uptime === b.uptime &&
    a.last_seen === b.last_seen &&
    a.ping_data === b.ping_data &&
    a.display_name === b.display_name &&
    a.hostname === b.hostname &&
    a.os === b.os &&
    a.virtualization === b.virtualization &&
    a.distro === b.distro
  )
})
