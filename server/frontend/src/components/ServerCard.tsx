import { memo, useMemo, useState } from 'react'
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

/** 三网类别 */
type PingCategory = '电信' | '联通' | '移动' | '其他'

/** 将 ping target 归类为三网类别 */
function categorizePing(ping: PingResult): PingCategory {
  const text = `${ping.target || ''} ${ping.name || ''}`.toLowerCase()
  // 使用词边界匹配短代码，避免子串误判（如 "connect" 含 "ct"）
  const wordTest = (t: string, kw: string) =>
    new RegExp(`(^|[^a-z])${kw}([^a-z]|$)`).test(t)
  if (text.includes('电信') || text.includes('telecom') || wordTest(text, 'ct')) return '电信'
  if (text.includes('联通') || text.includes('unicom') || wordTest(text, 'cu')) return '联通'
  if (text.includes('移动') || text.includes('mobile') || text.includes('cmcc')) return '移动'
  return '其他'
}

/** 根据三网类别返回颜色 */
function categoryColor(cat: PingCategory): string {
  switch (cat) {
    case '电信': return '#007AFF'
    case '联通': return '#34C759'
    case '移动': return '#FF9500'
    default: return '#AF52DE'
  }
}

/** 指标进度条格子 */
function MetricCell({
  label,
  value,
  color,
  suffix = '%',
}: {
  label: string
  value: number
  color: string
  suffix?: string
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <span className="text-[10px] text-muted-foreground">{label}</span>
        <span className="text-xs font-medium text-foreground">
          {Math.min(Math.max(value, 0), 100).toFixed(1)}
          {suffix}
        </span>
      </div>
      <div className="h-1 w-full overflow-hidden rounded-full bg-secondary">
        <div
          className="h-full rounded-full transition-all duration-500"
          style={{ width: `${Math.min(value, 100)}%`, backgroundColor: color }}
        />
      </div>
    </div>
  )
}

/** 迷你延迟柱形图 - 每个柱子代表一个探测目标的延迟，有丢包时标红 */
function LatencyBars({
  targets,
  online,
}: {
  targets: Array<{ name: string; latency: number; color: string; loss: number }>
  online: boolean
}) {
  if (!online || targets.length === 0) {
    return (
      <div className="flex h-10 items-center justify-center">
        <span className="text-[10px] text-muted-foreground/60">---</span>
      </div>
    )
  }

  const maxLatency = targets.reduce((max, t) => (t.latency > max ? t.latency : max), 0) || 100

  return (
    <div className="flex h-10 items-end justify-center gap-2">
      {targets.map((t, i) => {
        const ratio = t.latency > 0 ? t.latency / maxLatency : 0
        const barHeight = Math.max(3, ratio * 28)
        const hasLoss = t.loss > 0
        return (
          <div key={i} className="flex min-w-0 flex-1 flex-col items-center gap-0.5">
            <span className={`text-[9px] font-medium ${hasLoss ? 'text-amber-500' : 'text-foreground/80'}`}>
              {t.latency > 0 ? `${t.latency.toFixed(0)}` : '--'}
            </span>
            <div
              className="w-full max-w-[20px] rounded-t-sm transition-all duration-500"
              style={{ height: `${barHeight}px`, backgroundColor: hasLoss ? '#FF3B30' : t.color }}
            />
            <span className="w-full truncate text-center text-[8px] text-muted-foreground">
              {t.name || '--'}
            </span>
            {hasLoss && (
              <span className="text-[8px] font-medium text-amber-500">{t.loss.toFixed(0)}%</span>
            )}
          </div>
        )
      })}
    </div>
  )
}

/** 服务器卡片组件 */
function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()
  const [showAllPings, setShowAllPings] = useState(false)

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

  // 三网延迟目标列表（最多展示3个）
  const pingTargets = useMemo(() => {
    const pings = server.ping_data || []
    // 按类别排序：电信 > 联通 > 移动 > 其他
    const categoryOrder: PingCategory[] = ['电信', '联通', '移动', '其他']
    const sorted = [...pings].sort((a, b) => {
      const ca = categorizePing(a)
      const cb = categorizePing(b)
      return categoryOrder.indexOf(ca) - categoryOrder.indexOf(cb)
    })
    // 默认只展示前3个，点击可展开全部
    const display = showAllPings ? sorted : sorted.slice(0, 3)
    return display.map((p) => ({
      name: p.name || categorizePing(p),
      latency: p.avg_latency ?? 0,
      color: categoryColor(categorizePing(p)),
      loss: p.loss ?? 0,
    }))
  }, [server.ping_data, showAllPings])

  const hasPingData = (server.ping_data || []).length > 0
  const totalPingCount = (server.ping_data || []).length

  const displayName = server.display_name || server.hostname

  return (
    <div
      onClick={handleClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          navigate(`${basePath}/server/${server.id}`)
        }
      }}
      role="button"
      tabIndex={0}
      className="group cursor-pointer rounded-2xl border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-lg animate-fade-in focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    >
      {/* 1. 头部行：状态圆点 + 名称 + 国旗 + 在线/离线标签 */}
      <div className="mb-3 flex items-center gap-2">
        <span className="relative flex h-2 w-2 shrink-0">
          {server.online && (
            <span
              className="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
              style={{ backgroundColor: '#34C759' }}
            />
          )}
          <span
            className="relative inline-flex h-2 w-2 rounded-full"
            style={{ backgroundColor: server.online ? '#34C759' : '#6b7280' }}
          />
        </span>
        <h3 className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
          {displayName}
        </h3>
        {flag && <span className="shrink-0 text-sm leading-none">{flag}</span>}
        <span
          className={`shrink-0 text-xs font-medium ${
            server.online ? 'text-success' : 'text-muted-foreground'
          }`}
        >
          {server.online ? '在线' : '离线'}
        </span>
      </div>

      {/* 2. 指标网格：CPU / 内存 / 硬盘（三列，参考 dstatus 紧凑布局） */}
      <div className="mb-3 grid grid-cols-3 gap-2.5">
        <MetricCell label="CPU" value={server.cpu || 0} color="#007AFF" />
        <MetricCell label="内存" value={memUsagePercent} color="#AF52DE" />
        <MetricCell label="硬盘" value={diskUsage} color="#FF9500" />
      </div>

      {/* 3. 网络信息：实时速率 + 累计流量（参考 dstatus 单行四项紧凑布局） */}
      <div className="mb-3 rounded-lg bg-secondary/30 px-3 py-2">
        {/* 实时速率行 */}
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <span style={{ color: '#5AC8FA' }}>↓</span>
            <span className="font-medium" style={{ color: '#5AC8FA' }}>
              {server.online ? formatSpeed(server.net_rx) : '---'}
            </span>
          </span>
          <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
            <span style={{ color: '#AF52DE' }}>↑</span>
            <span className="font-medium" style={{ color: '#AF52DE' }}>
              {server.online ? formatSpeed(server.net_tx) : '---'}
            </span>
          </span>
        </div>
        {/* 累计流量行 */}
        <div className="mt-1 flex items-center justify-between border-t border-border/50 pt-1 text-[10px]">
          <span className="flex items-center gap-1 text-muted-foreground">
            <span style={{ color: '#5AC8FA' }}>↓</span>
            <span className="font-medium text-foreground/70">
              {server.online ? formatTraffic(server.total_rx || 0) : '---'}
            </span>
          </span>
          <span className="flex items-center gap-1 text-muted-foreground">
            <span style={{ color: '#AF52DE' }}>↑</span>
            <span className="font-medium text-foreground/70">
              {server.online ? formatTraffic(server.total_tx || 0) : '---'}
            </span>
          </span>
        </div>
      </div>

      {/* 4. 三网延迟行 */}
      <div className="mb-3 border-t border-border pt-3">
        {hasPingData ? (
          <>
            <div className="mb-1 flex items-center justify-between">
              <span className="text-[10px] text-muted-foreground">延迟探测</span>
              {totalPingCount > 3 && (
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    setShowAllPings(!showAllPings)
                  }}
                  className="text-[9px] text-primary hover:text-primary/80"
                >
                  {showAllPings ? '收起' : `查看全部 ${totalPingCount}`}
                </button>
              )}
            </div>
            <LatencyBars targets={pingTargets} online={server.online} />
          </>
        ) : (
          <div className="py-2 text-center text-[10px] text-muted-foreground/60">
            暂无延迟数据
          </div>
        )}
      </div>

      {/* 5. 底部信息栏：运行时间 · 最后更新 */}
      <div className="flex items-center justify-between gap-1 text-[10px] text-muted-foreground">
        <span className="shrink-0">
          {server.online ? formatUptime(server.uptime) : '---'}
        </span>
        <span className="shrink-0">·</span>
        <span className="shrink-0 whitespace-nowrap">
          {formatRelativeTime(server.last_seen)}
        </span>
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
    prev.basePath === next.basePath
  )
})
