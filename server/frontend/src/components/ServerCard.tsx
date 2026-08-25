import { memo, useMemo, useState, useRef, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ServerData } from '@/types'
import {
  formatSpeed,
  formatTraffic,
  formatUptime,
  formatRelativeTime,
  getFlagEmoji,
  getCountryCode,
  getTagStyle,
  parseTags,
  formatPrice,
  formatExpireDate,
  getMonthlyTrafficPercent,
  getTrafficColor,
  getExpireColor,
} from '@/lib/utils'
import { useServerStore, type CardHistoryPoint } from '@/store/useServerStore'
import ResourceRing from '@/components/ResourceRing'
import DistroIcon from '@/components/DistroIcon'
import StatusDot from '@/components/StatusDot'
import { useAnimatedNumber } from '@/hooks/useAnimatedNumber'
import { useInViewport } from '@/hooks/useInViewport'

/** 稳定的空数组引用，避免 || [] 每次创建新引用导致 Zustand 不必要重渲染 */
const EMPTY_HISTORY: CardHistoryPoint[] = []

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

/** 丢包率桶颜色 */
const LOSS_COLORS = {
  perfect: '#69BE7B',     // 0%
  good: '#A7D879',        // 0-5%
  fair: '#E8CC68',        // 5-15%
  warning: '#EFA85F',     // 15-30%
  bad: '#E98686',         // 30-50%
  critical: '#D96B6B',    // >50%
  empty: 'rgba(148, 163, 184, 0.22)',
} as const

/** 根据丢包率返回颜色 */
function getLossColor(avgLoss: number, hasData: boolean): string {
  if (!hasData) return LOSS_COLORS.empty
  if (avgLoss <= 0) return LOSS_COLORS.perfect
  if (avgLoss <= 5) return LOSS_COLORS.good
  if (avgLoss <= 15) return LOSS_COLORS.fair
  if (avgLoss <= 30) return LOSS_COLORS.warning
  if (avgLoss <= 50) return LOSS_COLORS.bad
  return LOSS_COLORS.critical
}

/** 丢包率桶聚合结果 */
interface LossBucket {
  index: number
  color: string
  hasData: boolean
  avgLoss: number
  count: number
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

    // 过滤掉 timestamp 无效的数据点，防止 bucketIdx 计算为 NaN 导致数组越界
    const validPoints = points.filter(
      (p) => typeof p.timestamp === 'number' && !isNaN(p.timestamp) && p.ping_data,
    )
    if (validPoints.length === 0) return result

    const oldestTs = validPoints[0].timestamp
    const newestTs = validPoints[validPoints.length - 1].timestamp
    const range = newestTs - oldestTs || 1

    for (const point of validPoints) {
      const ratio = (point.timestamp - oldestTs) / range
      // 确保比值在 [0, 1] 范围内，防止 NaN 或 Infinity
      if (!isFinite(ratio)) continue
      const bucketIdx = Math.min(
        Math.max(0, Math.floor(ratio * COMPACT_BUCKETS)),
        COMPACT_BUCKETS - 1,
      )
      const bucket = result[bucketIdx]
      if (!bucket) continue
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
 * 紧凑丢包率分布条（卡片专用）
 *
 * - 30 个时间桶，自适应历史数据范围
 * - 高度 8px，右侧显示当前平均丢包率
 * - 0% 绿色，随丢包率升高渐变到深红
 */
function CompactPacketLossBar({
  points,
  online,
  currentAvgLoss,
}: {
  points: CardHistoryPoint[]
  online: boolean
  currentAvgLoss: number | null
}) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null)

  const buckets = useMemo<LossBucket[]>(() => {
    const result: LossBucket[] = Array.from({ length: COMPACT_BUCKETS }, (_, i) => ({
      index: i,
      color: LOSS_COLORS.empty,
      hasData: false,
      avgLoss: 0,
      count: 0,
    }))

    if (points.length === 0) return result

    const validPoints = points.filter(
      (p) => typeof p.timestamp === 'number' && !isNaN(p.timestamp) && p.ping_data,
    )
    if (validPoints.length === 0) return result

    const oldestTs = validPoints[0].timestamp
    const newestTs = validPoints[validPoints.length - 1].timestamp
    const range = newestTs - oldestTs || 1

    for (const point of validPoints) {
      const ratio = (point.timestamp - oldestTs) / range
      if (!isFinite(ratio)) continue
      const bucketIdx = Math.min(
        Math.max(0, Math.floor(ratio * COMPACT_BUCKETS)),
        COMPACT_BUCKETS - 1,
      )
      const bucket = result[bucketIdx]
      if (!bucket) continue
      const pings = point.ping_data || []
      if (pings.length === 0) continue

      let lossSum = 0
      let validCount = 0
      for (const ping of pings) {
        if (ping.avg_latency != null && ping.avg_latency >= 0) {
          lossSum += ping.loss || 0
          validCount++
        }
      }

      if (validCount > 0) {
        const pointAvgLoss = lossSum / validCount
        bucket.avgLoss = (bucket.avgLoss * bucket.count + pointAvgLoss) / (bucket.count + 1)
        bucket.count++
        bucket.hasData = true
      }
    }

    for (const bucket of result) {
      if (bucket.hasData) {
        bucket.color = getLossColor(bucket.avgLoss, true)
      }
    }

    return result
  }, [points])

  const hoveredBucket = hoveredIdx !== null ? buckets[hoveredIdx] : null

  return (
    <div>
      {/* 标题行 */}
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">丢包率</span>
        <span
          className="text-[10px] font-medium tabular-nums"
          style={{
            color:
              online && currentAvgLoss !== null
                ? getLossColor(currentAvgLoss, true)
                : 'hsl(var(--muted-foreground))',
          }}
        >
          {online && currentAvgLoss !== null
            ? `${currentAvgLoss.toFixed(1)}%`
            : '---'}
        </span>
      </div>
      {/* 桶条 */}
      <div className="relative">
        <div className="flex items-end gap-[2px]" style={{ height: 8 }}>
          {buckets.map((bucket) => (
            <div
              key={bucket.index}
              className="flex-1 cursor-pointer rounded-[1.5px] transition-all"
              style={{
                height: '100%',
                backgroundColor: bucket.color,
                opacity: hoveredIdx !== null && hoveredIdx !== bucket.index ? 0.45 : 1,
                transform: hoveredIdx === bucket.index ? 'scaleY(1.25)' : 'scaleY(1)',
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
              <span className="whitespace-nowrap font-medium text-foreground">
                丢包 {hoveredBucket.avgLoss.toFixed(1)}%
              </span>
            ) : (
              <span className="text-muted-foreground">无数据</span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/** 服务器卡片组件（NodeGet 风格） */
function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()

  // 视口懒加载：仅当卡片进入视口时才渲染延迟和丢包率部分
  const { ref: viewportRef, isInViewport } = useInViewport<HTMLDivElement>(320)

  // P2: 从 Store 读取卡片历史滚动窗口（统一管理，避免每张卡片各自维护状态）
  const history = useServerStore(
    (s) => s.cardHistory.get(server.id) ?? EMPTY_HISTORY,
  )

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

  // 国旗 emoji（管理员设置的 country_code 优先，其次从名称推断）
  const flag = useMemo(() => {
    const cc = getCountryCode(server)
    return cc ? getFlagEmoji(cc) : ''
  }, [server.country_code, server.display_name, server.hostname])

  // 标签徽章（最多展示 3 个）
  const tagBadges = useMemo(() => parseTags(server.tags).slice(0, 3), [server.tags])

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

  // 当前平均丢包率（所有探测目标的平均值）
  const currentAvgLoss = useMemo(() => {
    const pings = server.ping_data || []
    const valid = pings.filter((p) => p.avg_latency != null && p.avg_latency >= 0)
    if (valid.length === 0) return null
    return valid.reduce((sum, p) => sum + (p.loss || 0), 0) / valid.length
  }, [server.ping_data])

  const displayName = server.display_name || server.hostname

  // 月流量使用量与配额进度（NodeGet 风格）
  const monthlyUsed = (server.monthly_rx || 0) + (server.monthly_tx || 0)
  const monthlyQuota = server.traffic_quota_bytes || 0
  const monthlyPercent = getMonthlyTrafficPercent(monthlyUsed, monthlyQuota)

  // 到期信息（null=永不过期）
  const expiresInDays = server.expires_in_days
  const expireColor = expiresInDays != null ? getExpireColor(expiresInDays) : ''

  // 费用展示（如 "$49.99/年"）
  const priceText =
    server.price_amount && server.price_amount > 0
      ? formatPrice(server.price_amount, server.price_currency || '', server.price_cycle || '')
      : ''

  // 虚拟化 Badge（仅在存在数据时显示）
  const virtualization = server.virtualization
  const showVirtualizationBadge =
    virtualization && virtualization !== 'None' && virtualization !== 'none'

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
      {/* 1. 头部行：国旗 + 名称 + 标签徽章 */}
      <div className="flex items-center gap-1.5 border-b border-dashed border-border pb-2.5">
        {flag && <span className="shrink-0 text-base leading-none">{flag}</span>}
        <h3 className="min-w-0 flex-1 truncate text-[14px] font-bold text-foreground sm:text-[15px]">
          {displayName}
        </h3>
        {tagBadges.map((tag) => {
          const style = getTagStyle(tag)
          return (
            <span
              key={tag}
              className="shrink-0 rounded-md px-1.5 py-0.5 text-[9px] font-semibold leading-none"
              style={style}
            >
              {tag}
            </span>
          )
        })}
        {showVirtualizationBadge && (
          <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[9px] font-medium text-muted-foreground">
            {virtualization}
          </span>
        )}
      </div>

      {/* 2. 状态行：在线状态 · 运行时间 · 位置 · 供应商 */}
      <div className="mt-1.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <StatusDot online={server.online} size="sm" />
        <span className="shrink-0 font-medium">
          {server.online ? formatUptime(server.uptime) : '离线'}
        </span>
        {(server.region || server.isp) && (
          <>
            <span className="text-muted-foreground/40">·</span>
            <span className="truncate">
              {[server.region, server.isp].filter(Boolean).join(' · ')}
            </span>
          </>
        )}
        <span className="ml-auto shrink-0">
          <DistroIcon distro={server.distro} os={server.os} size={14} />
        </span>
      </div>

      {/* 3. 资源环形图：CPU / 内存 / 硬盘（三个圆环） */}
      <div className="mt-3 grid grid-cols-3 gap-x-2 gap-y-3">
        <ResourceRing label="CPU" value={server.cpu || 0} size={80} />
        <ResourceRing label="内存" value={memUsagePercent} size={80} />
        <ResourceRing label="硬盘" value={diskUsage} size={80} />
      </div>

      {/* 4. 网络信息面板：实时速率 + 月流量进度条 */}
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
        {/* 月流量进度条（当月累计/配额，超 80% 橙、100% 红；无配额只显示数值） */}
        <div className="mt-2 border-t border-dashed border-border/60 pt-1.5">
          <div className="mb-1 flex items-center justify-between text-[10px]">
            <span className="text-muted-foreground">月流量</span>
            <span className="font-medium tabular-nums text-foreground/70">
              {formatTraffic(monthlyUsed)}
              {monthlyQuota > 0 ? ` / ${formatTraffic(monthlyQuota)}` : ''}
            </span>
          </div>
          {monthlyPercent !== null && (
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
              <div
                className="h-full rounded-full transition-all duration-500"
                style={{
                  width: `${Math.min(monthlyPercent, 100)}%`,
                  backgroundColor: getTrafficColor(monthlyPercent),
                }}
              />
            </div>
          )}
        </div>
      </div>

      {/* 5. 延迟质量分布 + 丢包率（仅在卡片进入视口时渲染） */}
      <div className="mt-3 rounded-xl border border-dashed border-border/80 px-3 py-2.5">
        {isInViewport ? (
          <div className="space-y-2.5">
            <CompactLatencyBar
              points={history}
              online={server.online}
              currentAvgLatency={currentAvgLatency}
            />
            <CompactPacketLossBar
              points={history}
              online={server.online}
              currentAvgLoss={currentAvgLoss}
            />
          </div>
        ) : (
          // 占位符：不在视口时不渲染，显示占位高度避免布局抖动
          <div className="flex h-16 items-center justify-center">
            <span className="text-[10px] text-muted-foreground/40">加载中...</span>
          </div>
        )}
      </div>

      {/* 6. 底部信息栏：到期 · 费用 · 最后更新 */}
      <div className="mt-auto space-y-1.5 border-t border-dashed border-border pt-2.5 text-xs tabular-nums text-muted-foreground">
        {(expiresInDays != null || priceText) && (
          <div className="flex items-center justify-between gap-1">
            {expiresInDays != null && (
              <span className="shrink-0">
                到期 {formatExpireDate(server.expires_at)}
                <span
                  className="ml-1 font-medium"
                  style={expireColor ? { color: expireColor } : undefined}
                >
                  {expiresInDays < 0 ? '已过期' : `余${expiresInDays}天`}
                  {expiresInDays < 7 ? ' ⚠' : ''}
                </span>
              </span>
            )}
            {priceText && (
              <span className="shrink-0 whitespace-nowrap font-medium text-foreground/70">
                {priceText}
              </span>
            )}
          </div>
        )}
        <div className="flex items-center justify-between gap-1">
          <span className="shrink-0">{server.online ? '在线' : '---'}</span>
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
    a.distro === b.distro &&
    a.tags === b.tags &&
    a.region === b.region &&
    a.country_code === b.country_code &&
    a.isp === b.isp &&
    a.expires_at === b.expires_at &&
    a.expires_in_days === b.expires_in_days &&
    a.price_amount === b.price_amount &&
    a.price_currency === b.price_currency &&
    a.price_cycle === b.price_cycle &&
    a.traffic_quota_bytes === b.traffic_quota_bytes &&
    a.monthly_rx === b.monthly_rx &&
    a.monthly_tx === b.monthly_tx
  )
})
