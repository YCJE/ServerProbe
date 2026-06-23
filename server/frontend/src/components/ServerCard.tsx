import { useNavigate } from 'react-router-dom'
import type { ServerData, PingResult } from '@/types'
import {
  formatBytes,
  formatSpeed,
  formatUptime,
  formatLatency,
  formatLoss,
  getUsageColor,
  getUsageTextColor,
  getLossColor,
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

/** 进度条组件 */
function ProgressBar({ value, color }: { value: number; color: string }) {
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
      <div
        className={`h-full rounded-full transition-all duration-500 ${color}`}
        style={{ width: `${Math.min(value, 100)}%` }}
      />
    </div>
  )
}

/** Ping 信息展示 */
function PingInfo({ ping, online }: { ping?: PingResult; online: boolean }) {
  if (!online || !ping) {
    return (
      <div className="flex flex-col items-center gap-0.5">
        <span className="text-xs text-muted-foreground">{ping?.name || '---'}</span>
        <span className="text-sm font-medium text-muted-foreground">---</span>
        <span className="text-xs text-muted-foreground">---</span>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center gap-0.5">
      <span className="text-xs text-muted-foreground">{ping.name}</span>
      <span className="text-sm font-medium text-foreground">
        {formatLatency(ping.avg_latency)}
      </span>
      <span className={`text-xs font-medium ${getLossColor(ping.loss)}`}>
        {formatLoss(ping.loss)}
      </span>
    </div>
  )
}

/** 服务器卡片组件 */
export default function ServerCard({ server, basePath = '/admin' }: ServerCardProps) {
  const navigate = useNavigate()

  const handleClick = () => {
    navigate(`${basePath}/server/${server.id}`)
  }

  const memUsagePercent = server.mem_total > 0
    ? (server.mem_used / server.mem_total) * 100
    : server.mem

  const diskUsage = server.disk_usage || 0
  const disks = server.disks || []
  const pingData = server.ping_data || []

  return (
    <div
      onClick={handleClick}
      className="group cursor-pointer rounded-xl border border-border bg-card p-4 shadow-sm transition-all hover:border-primary/50 hover:shadow-md animate-fade-in"
    >
      {/* 头部：主机名 + 状态 */}
      <div className="mb-3 flex items-start justify-between">
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-base font-semibold text-foreground">
            {server.display_name || server.hostname}
          </h3>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {server.hostname} · {server.os} · {server.arch}
          </p>
        </div>
        <div className="ml-2 flex shrink-0 items-center gap-1.5">
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              server.online ? 'bg-success animate-pulse' : 'bg-destructive'
            }`}
          />
          <span
            className={`text-xs font-medium ${
              server.online ? 'text-success' : 'text-destructive'
            }`}
          >
            {server.online ? '在线' : '离线'}
          </span>
        </div>
      </div>

      {/* 运行时间 */}
      <div className="mb-3 flex items-center gap-1.5 text-xs text-muted-foreground">
        <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span>{server.online ? formatUptime(server.uptime) : '已离线'}</span>
      </div>

      {/* CPU / 内存 / 磁盘 进度条 */}
      <div className="mb-3 space-y-2">
        {/* CPU */}
        <div>
          <div className="mb-1 flex items-center justify-between text-xs">
            <span className="text-muted-foreground">CPU</span>
            <span className={`font-medium ${getUsageTextColor(server.cpu)}`}>
              {server.cpu.toFixed(1)}%
            </span>
          </div>
          <ProgressBar value={server.cpu} color={getUsageColor(server.cpu)} />
        </div>

        {/* 内存 */}
        <div>
          <div className="mb-1 flex items-center justify-between text-xs">
            <span className="text-muted-foreground">内存</span>
            <span className={`font-medium ${getUsageTextColor(memUsagePercent)}`}>
              {memUsagePercent.toFixed(1)}%
            </span>
          </div>
          <ProgressBar value={memUsagePercent} color={getUsageColor(memUsagePercent)} />
        </div>

        {/* 磁盘 */}
        <div>
          <div className="mb-1 flex items-center justify-between text-xs">
            <span className="text-muted-foreground">磁盘使用</span>
            <span className={`font-medium ${getUsageTextColor(diskUsage)}`}>
              {diskUsage.toFixed(1)}%
            </span>
          </div>
          <ProgressBar value={diskUsage} color={getUsageColor(diskUsage)} />
          {/* 磁盘总量信息 */}
          {disks.length > 0 && (
            <div className="mt-1.5 text-xs text-muted-foreground/70">
              已用 {formatBytes(disks.reduce((sum, d) => sum + d.used, 0))} / 总{' '}
              {formatBytes(disks.reduce((sum, d) => sum + d.total, 0))}
            </div>
          )}
        </div>
      </div>

      {/* 网络速度 */}
      <div className="mb-3 grid grid-cols-2 gap-2">
        <div className="rounded-lg bg-secondary/50 px-2.5 py-1.5">
          <div className="flex items-center gap-1 text-xs text-muted-foreground">
            <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 14l-7 7m0 0l-7-7m7 7V3" />
            </svg>
            下行
          </div>
          <div className="mt-0.5 text-sm font-medium text-foreground">
            {server.online ? formatSpeed(server.net_rx) : '---'}
          </div>
        </div>
        <div className="rounded-lg bg-secondary/50 px-2.5 py-1.5">
          <div className="flex items-center gap-1 text-xs text-muted-foreground">
            <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 10l7-7m0 0l7 7m-7-7v18" />
            </svg>
            上行
          </div>
          <div className="mt-0.5 text-sm font-medium text-foreground">
            {server.online ? formatSpeed(server.net_tx) : '---'}
          </div>
        </div>
      </div>

      {/* 三网延迟 */}
      <div className="border-t border-border pt-2.5">
        <div className="mb-1.5 text-xs font-medium text-muted-foreground">三网延迟 / 丢包率</div>
        {pingData.length > 0 ? (
          <div className="grid grid-cols-3 gap-1">
            {pingData.slice(0, 3).map((ping) => (
              <PingInfo key={ping.name} ping={ping} online={server.online} />
            ))}
          </div>
        ) : (
          <div className="py-2 text-center text-xs text-muted-foreground/60">
            未配置探测目标
          </div>
        )}
      </div>
    </div>
  )
}
