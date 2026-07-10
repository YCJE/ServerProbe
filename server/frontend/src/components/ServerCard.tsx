import { memo, useMemo, useRef, useState, useEffect } from 'react'
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

/**
 * 根据延迟值返回颜色（NodeGet 延迟分桶色）
 * - <= 50ms: 深绿  #69BE7B
 * - <= 100ms: 浅绿 #A7D879
 * - <= 180ms: 浅黄 #E8CC68
 * - <= 300ms: 深黄 #EFA85F
 * - > 300ms:  浅红 #E98686
 * - 丢包:     深红 #D96B6B
 * - 无数据:   灰   rgba(148,163,184,0.28)
 */
function getLatencyColor(latency: number, loss: number): string {
  if (loss > 0) return '#D96B6B'
  if (latency <= 0) return 'rgba(148, 163, 184, 0.28)'
  if (latency <= 50) return '#69BE7B'
  if (latency <= 100) return '#A7D879'
  if (latency <= 180) return '#E8CC68'
  if (latency <= 300) return '#EFA85F'
  return '#E98686'
}

/** 横向延迟条形图 - 每个探测目标一行：名称 + 横条 + 数值，有丢包时标红 */
function LatencyBars({
  targets,
  online,
}: {
  targets: Array<{ name: string; latency: number; loss: number }>
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
    <div className="space-y-1.5">
      {targets.map((t, i) => {
        const ratio = t.latency > 0 ? t.latency / maxLatency : 0
        const barWidth = Math.max(8, ratio * 100)
        const hasLoss = t.loss > 0
        const color = getLatencyColor(t.latency, t.loss)
        return (
          <div key={i} className="flex items-center gap-2">
            {/* 左侧：颜色点 + 名称 */}
            <span className="flex w-16 shrink-0 items-center gap-1">
              <span
                className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
                style={{ backgroundColor: color }}
              />
              <span className="truncate text-[9px] text-muted-foreground">
                {t.name || '--'}
              </span>
            </span>
            {/* 中间：横向进度条（flex-1 自适应填充剩余空间） */}
            <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-secondary">
              <div
                className="h-full rounded-full transition-all duration-500"
                style={{ width: `${barWidth}%`, backgroundColor: hasLoss ? '#D96B6B' : color }}
              />
            </div>
            {/* 右侧：延迟数值（固定） */}
            <span
              className={`shrink-0 text-[9px] font-medium tabular-nums ${
                hasLoss ? 'text-amber-500' : 'text-foreground/80'
              }`}
            >
              {t.latency > 0 ? `${t.latency.toFixed(0)}ms` : '--'}
            </span>
            {/* 丢包率（固定显示，0% 也展示） */}
            <span
              className={`shrink-0 text-[9px] font-medium tabular-nums ${
                hasLoss ? 'text-amber-500' : 'text-muted-foreground/60'
              }`}
            >
              {t.loss.toFixed(1)}%
            </span>
          </div>
        )
      })}
    </div>
  )
}

/** 服务器卡片组件（NodeGet 风格） */
function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()
  const [showAllPings, setShowAllPings] = useState(false)

  // 视口懒加载：仅当卡片进入视口时才渲染延迟探测部分
  const { ref: viewportRef, isInViewport } = useInViewport<HTMLDivElement>(320)

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
      loss: p.loss ?? 0,
    }))
  }, [server.ping_data, showAllPings])

  const hasPingData = (server.ping_data || []).length > 0
  const totalPingCount = (server.ping_data || []).length

  const displayName = server.display_name || server.hostname

  // 虚拟化 Badge（仅在存在数据时显示）
  const virtualization = server.virtualization
  const showVirtualizationBadge = virtualization && virtualization !== 'None' && virtualization !== 'none'

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
      className={`group relative flex min-h-[360px] cursor-pointer flex-col rounded-2xl border border-border p-4 card-soft node-card-hover animate-fade-in focus:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:min-h-[430px] sm:p-5 ${
        server.online ? '' : 'opacity-75'
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
            <span style={{ color: '#5AC8FA' }}>↓</span>
            <span className="font-medium tabular-nums text-foreground/70">
              {server.online ? formatTraffic(server.total_rx || 0) : '---'}
            </span>
          </span>
          <span className="flex items-center gap-1 text-muted-foreground">
            <span style={{ color: '#AF52DE' }}>↑</span>
            <span className="font-medium tabular-nums text-foreground/70">
              {server.online ? formatTraffic(server.total_tx || 0) : '---'}
            </span>
          </span>
        </div>
      </div>

      {/* 5. 延迟探测面板 - 仅在卡片进入视口时渲染 */}
      <div className="mt-3 rounded-xl border border-dashed border-border/80 px-3 py-3">
        {isInViewport && hasPingData ? (
          <>
            <div className="mb-1.5 flex items-center justify-between">
              <span className="text-[10px] font-medium text-muted-foreground">延迟探测</span>
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
        ) : isInViewport ? (
          <div className="flex h-10 items-center justify-center text-[10px] text-muted-foreground/60">
            暂无延迟数据
          </div>
        ) : (
          // 占位符：不在视口时不渲染延迟数据，显示占位高度避免布局抖动
          <div className="flex h-10 items-center justify-center">
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
    a.distro === b.distro &&
    a.virtualization === b.virtualization &&
    a.processes === b.processes &&
    a.time_offset === b.time_offset &&
    prev.basePath === next.basePath
  )
})
