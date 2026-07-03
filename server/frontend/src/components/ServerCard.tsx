import { memo, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import type { ServerData, PingResult } from '@/types'
import MiniBarChart from '@/components/MiniBarChart'
import {
  formatBytes,
  formatSpeed,
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
  if (text.includes('电信') || text.includes('telecom') || text.includes('ct')) return '电信'
  if (text.includes('联通') || text.includes('unicom') || text.includes('cu')) return '联通'
  if (text.includes('移动') || text.includes('mobile') || text.includes('cmcc')) return '移动'
  return '其他'
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
          {value.toFixed(1)}
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

/** 三网延迟分组展示 */
function PingGroup({
  label,
  data,
  color,
  online,
}: {
  label: string
  data: number[]
  color: string
  online: boolean
}) {
  return (
    <div className="flex flex-col items-center gap-1">
      <span className="text-[10px] text-muted-foreground">{label}</span>
      {online && data.length > 0 ? (
        <MiniBarChart data={data} color={color} height={24} barWidth={3} gap={1} />
      ) : (
        <div className="flex h-6 items-center">
          <span className="text-[10px] text-muted-foreground/60">---</span>
        </div>
      )}
    </div>
  )
}

/** 服务器卡片组件 */
function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()

  const handleClick = () => {
    const selection = window.getSelection()
    if (selection && selection.toString().length > 0) return
    navigate(`${basePath}/server/${server.id}`)
  }

  // 国旗 emoji
  const flag = useMemo(() => {
    const region = getRegionFromServer(server)
    return region ? getFlagEmoji(region) : ''
  }, [server])

  // 硬件信息（基于现有数据模型：os · arch · 内存大小）
  const hardwareInfo = useMemo(() => {
    const parts: string[] = []
    if (server.os) parts.push(server.os)
    if (server.arch) parts.push(server.arch)
    if (server.mem_total > 0) parts.push(formatBytes(server.mem_total, 0))
    return parts.length > 0 ? parts.join(' · ') : '---'
  }, [server.os, server.arch, server.mem_total])

  // 内存使用率
  const memUsagePercent = server.mem_total > 0
    ? (server.mem_used / server.mem_total) * 100
    : (server.mem || 0)

  // 磁盘使用率
  const diskUsage = server.disk_usage || 0

  // 流量进度（以 100MB/s 为满载基准）
  const trafficValue = Math.min(
    100,
    ((server.net_rx || 0) + (server.net_tx || 0)) / (100 * 1024 * 1024) * 100,
  )

  // 三网延迟分组
  const pingGroups = useMemo(() => {
    const groups: { 电信: number[]; 联通: number[]; 移动: number[] } = {
      电信: [],
      联通: [],
      移动: [],
    }
    for (const ping of server.ping_data || []) {
      const cat = categorizePing(ping)
      if (cat === '电信' || cat === '联通' || cat === '移动') {
        groups[cat].push(ping.avg_latency)
      }
    }
    return groups
  }, [server.ping_data])

  const hasPingData = (server.ping_data || []).length > 0
  const hasAnyPingGroup =
    pingGroups.电信.length > 0 || pingGroups.联通.length > 0 || pingGroups.移动.length > 0

  const displayName = server.display_name || server.hostname

  return (
    <div
      onClick={handleClick}
      className="group cursor-pointer rounded-2xl border border-border bg-card p-4 transition-all hover:border-primary/30 hover:shadow-lg animate-fade-in"
    >
      {/* 1. 头部行：状态圆点 + 名称 + 国旗 + 在线/离线标签 */}
      <div className="mb-2 flex items-center gap-2">
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

      {/* 2. 硬件信息行 */}
      <div className="mb-3 truncate text-xs text-muted-foreground">{hardwareInfo}</div>

      {/* 3. 2×2 指标网格：CPU / RAM / 硬盘 / 流量 */}
      <div className="mb-3 grid grid-cols-2 gap-3">
        <MetricCell label="CPU" value={server.cpu || 0} color="#007AFF" />
        <MetricCell label="RAM" value={memUsagePercent} color="#34C759" />
        <MetricCell label="硬盘" value={diskUsage} color="#FF9500" />
        {/* 流量格子：显示上传+下载速率而非百分比 */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between gap-1">
            <span className="text-[10px] text-muted-foreground">流量</span>
            <span className="truncate text-[10px] font-medium text-foreground">
              {server.online
                ? `↑${formatSpeed(server.net_tx)} ↓${formatSpeed(server.net_rx)}`
                : '---'}
            </span>
          </div>
          <div className="h-1 w-full overflow-hidden rounded-full bg-secondary">
            <div
              className="h-full rounded-full transition-all duration-500"
              style={{
                width: `${server.online ? trafficValue : 0}%`,
                backgroundColor: '#AF52DE',
              }}
            />
          </div>
        </div>
      </div>

      {/* 4. 三网延迟行 */}
      <div className="mb-3 border-t border-border pt-3">
        {hasPingData && hasAnyPingGroup ? (
          <div className="grid grid-cols-3 gap-2">
            <PingGroup
              label="电信"
              data={pingGroups.电信}
              color="#007AFF"
              online={server.online}
            />
            <PingGroup
              label="联通"
              data={pingGroups.联通}
              color="#34C759"
              online={server.online}
            />
            <PingGroup
              label="移动"
              data={pingGroups.移动}
              color="#FF9500"
              online={server.online}
            />
          </div>
        ) : (
          <div className="py-2 text-center text-[10px] text-muted-foreground/60">
            暂无延迟数据
          </div>
        )}
      </div>

      {/* 5. 底部信息栏：上传下载 · 运行时间 · 最后更新 */}
      <div className="flex items-center justify-between gap-1 text-[10px] text-muted-foreground">
        <span className="truncate">
          {server.online
            ? `↑${formatSpeed(server.net_tx)} ↓${formatSpeed(server.net_rx)}`
            : '---'}
        </span>
        <span className="shrink-0">
          · {server.online ? formatUptime(server.uptime) : '---'} ·
        </span>
        <span className="shrink-0 whitespace-nowrap">
          {formatRelativeTime(server.last_seen)}
        </span>
      </div>
    </div>
  )
}

export default memo(ServerCard)
