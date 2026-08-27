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
import { useTagColors } from '@/store/useTagStore'
import ResourceRing from '@/components/ResourceRing'
import DistroIcon from '@/components/DistroIcon'
import StatusDot from '@/components/StatusDot'
import LatencyGrid from '@/components/LatencyGrid'
import { useAnimatedNumber } from '@/hooks/useAnimatedNumber'
import { useInViewport } from '@/hooks/useInViewport'

/** 稳定的空数组引用，避免 || [] 每次创建新引用导致 Zustand 不必要重渲染 */
const EMPTY_HISTORY: CardHistoryPoint[] = []

/** 标签徽章样式：优先用管理端标签表设置的颜色，未设置回退 hash 配色 */
function tagBadgeStyle(tag: string, colorMap: Record<string, string>) {
  const color = colorMap[tag]
  if (color) {
    return { background: color, color: '#ffffff' }
  }
  return getTagStyle(tag)
}

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

/** 服务器卡片组件（NodeGet 风格） */
function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()
  const tagColors = useTagColors()

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
      className={`group relative flex min-h-[360px] cursor-pointer flex-col rounded-lg border border-border p-4 card-soft node-card-hover animate-fade-in focus:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:min-h-[420px] sm:p-5 ${
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
          const style = tagBadgeStyle(tag, tagColors)
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
      <div className="mt-3 rounded-md border border-dashed border-border/80 px-3 py-2.5">
        {/* 实时速率行 - 使用动画数值，上升显示主题色，下降显示橙色 */}
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <span style={{ color: rxRising ? 'hsl(var(--primary))' : '#FF9500' }}>↓</span>
            <span
              className="font-medium tabular-nums"
              style={{ color: rxRising ? 'hsl(var(--primary))' : '#FF9500' }}
            >
              {server.net_rx != null ? formatSpeed(animatedRx) : '---'}
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

      {/* 5. 延迟格子图（每目标一行，NodeGet 风格；仅在卡片进入视口时渲染） */}
      <div className="mt-3 rounded-md border border-dashed border-border/80 px-3 py-2.5">
        {isInViewport ? (
          <LatencyGrid points={history} ipVersion={4} maxCells={24} maxRows={4} compact />
        ) : (
          // 占位符：不在视口时不渲染，显示占位高度避免布局抖动
          <div className="flex h-24 items-center justify-center">
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
