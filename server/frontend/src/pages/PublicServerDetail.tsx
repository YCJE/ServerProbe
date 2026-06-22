import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { getPublicServers } from '@/lib/api'
import type { ServerData, PingResult } from '@/types'
import CpuChart from '@/components/CpuChart'
import MemoryChart from '@/components/MemoryChart'
import PingChart from '@/components/PingChart'
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

/** 实时历史数据点（公开详情页本地维护） */
interface LocalRealtimePoint {
  timestamp: number
  cpu: number
  mem: number
  ping_data: PingResult[]
}

/** 实时历史数据最大保留点数 */
const MAX_POINTS = 1200

/** 公开服务器详情页（无需登录，仅显示基本监控信息） */
export default function PublicServerDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const serverId = parseInt(id || '0', 10)

  const dashboardData = useServerStore((s) => s.dashboardData)
  const servers = useServerStore((s) => s.servers)
  const connectPublicDashboardWS = useServerStore((s) => s.connectPublicDashboardWS)
  const disconnectPublicDashboardWS = useServerStore((s) => s.disconnectPublicDashboardWS)
  const theme = useServerStore((s) => s.theme)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)

  // 本地维护的实时历史数据
  const [history, setHistory] = useState<LocalRealtimePoint[]>([])
  const [loading, setLoading] = useState(true)

  const isDark = useMemo(() => {
    if (theme === 'dark') return true
    if (theme === 'light') return false
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  }, [theme])

  // 连接公开 WebSocket
  useEffect(() => {
    connectPublicDashboardWS()
    return () => disconnectPublicDashboardWS()
  }, [connectPublicDashboardWS, disconnectPublicDashboardWS])

  // 首次加载时通过公开 API 获取服务器列表（用于在没有 WS 数据时也能展示）
  useEffect(() => {
    if (servers.length === 0) {
      setLoading(true)
      getPublicServers()
        .then((res) => {
          if (res.servers.length === 0) {
            setLoading(false)
          }
          // 如果有数据，等待 WS 推送更新 store
        })
        .catch(() => {
          setLoading(false)
        })
        .finally(() => {
          // 安全兜底: 5 秒后强制关闭 loading
          setTimeout(() => setLoading(false), 5000)
        })
    } else {
      setLoading(false)
    }
  }, [servers.length])

  // 从 store 中的 servers 列表查找当前服务器基本信息
  const baseServer = useMemo(() => {
    return servers.find((s) => s.id === serverId) || null
  }, [servers, serverId])

  // 实时数据
  const liveData = dashboardData.get(serverId)

  // 合并基本信息和实时数据
  const displayServer = useMemo<ServerData | null>(() => {
    if (!baseServer && !liveData) return null
    if (baseServer && liveData) {
      return {
        ...baseServer,
        online: liveData.online,
        cpu: liveData.cpu,
        mem: liveData.mem,
        mem_total: liveData.mem_total,
        mem_used: liveData.mem_used,
        swap_total: liveData.swap_total || 0,
        swap_used: liveData.swap_used || 0,
        net_rx: liveData.net_rx,
        net_tx: liveData.net_tx,
        uptime: liveData.uptime,
        load_1: liveData.load_1,
        load_5: liveData.load_5 || 0,
        load_15: liveData.load_15 || 0,
        disk_usage: liveData.disk_usage ?? baseServer.disk_usage ?? 0,
        disks: liveData.disks || [],
        tcp_connections: liveData.tcp_connections || 0,
        udp_connections: liveData.udp_connections || 0,
        process_count: liveData.process_count || 0,
        ping_data: liveData.ping_data,
      }
    }
    if (liveData) {
      return {
        id: liveData.agent_id,
        hostname: liveData.hostname || `Agent-${liveData.agent_id}`,
        display_name: liveData.display_name || '',
        os: baseServer?.os || '',
        arch: baseServer?.arch || '',
        agent_version: baseServer?.agent_version || '',
        online: liveData.online,
        last_seen: liveData.timestamp,
        cpu: liveData.cpu,
        mem: liveData.mem,
        mem_total: liveData.mem_total,
        mem_used: liveData.mem_used,
        swap_total: liveData.swap_total || 0,
        swap_used: liveData.swap_used || 0,
        net_rx: liveData.net_rx,
        net_tx: liveData.net_tx,
        uptime: liveData.uptime,
        load_1: liveData.load_1,
        load_5: liveData.load_5 || 0,
        load_15: liveData.load_15 || 0,
        disk_usage: liveData.disk_usage ?? 0,
        disks: liveData.disks || [],
        tcp_connections: liveData.tcp_connections || 0,
        udp_connections: liveData.udp_connections || 0,
        process_count: liveData.process_count || 0,
        ping_data: liveData.ping_data || [],
      }
    }
    return baseServer
  }, [baseServer, liveData])

  // 收集实时数据点到本地历史
  useEffect(() => {
    if (liveData) {
      setLoading(false)
      const point: LocalRealtimePoint = {
        timestamp: liveData.timestamp || Date.now() / 1000,
        cpu: liveData.cpu,
        mem: liveData.mem,
        ping_data: liveData.ping_data || [],
      }
      setHistory((prev) => {
        const next = [...prev, point]
        if (next.length > MAX_POINTS) {
          next.splice(0, next.length - MAX_POINTS)
        }
        return next
      })
    }
  }, [liveData])

  // 图表数据（最近 1 小时）
  const chartData = useMemo(() => {
    const cutoffTime = Date.now() / 1000 - 3600
    const filtered = history.filter((p) => p.timestamp >= cutoffTime)
    return {
      timestamps: filtered.map((p) => p.timestamp),
      cpuData: filtered.map((p) => p.cpu),
      memData: filtered.map((p) => p.mem),
      pingData: filtered.map((p) => p.ping_data || []),
    }
  }, [history])

  // 加载中
  if (loading && !displayServer) {
    return (
      <div className="flex h-full items-center justify-center py-20">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    )
  }

  if (!displayServer) {
    return (
      <div className="flex flex-col items-center justify-center py-16">
        <p className="text-sm text-muted-foreground">服务器不存在或未上线</p>
        <button
          onClick={() => navigate('/')}
          className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
        >
          返回首页
        </button>
      </div>
    )
  }

  const memUsagePercent = displayServer.mem_total > 0
    ? (displayServer.mem_used / displayServer.mem_total) * 100
    : displayServer.mem

  return (
    <div className="space-y-4">
      {/* 页面头部 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <button
            onClick={() => navigate('/')}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-border bg-card text-foreground transition-colors hover:bg-accent"
          >
            <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
            </svg>
          </button>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold text-foreground">
                {displayServer.display_name || displayServer.hostname}
              </h1>
              <span
                className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${
                  displayServer.online
                    ? 'bg-success/10 text-success'
                    : 'bg-destructive/10 text-destructive'
                }`}
              >
                <span
                  className={`inline-block h-1.5 w-1.5 rounded-full ${
                    displayServer.online ? 'bg-success' : 'bg-destructive'
                  }`}
                />
                {displayServer.online ? '在线' : '离线'}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {displayServer.hostname}
              {displayServer.os ? ` · ${displayServer.os}` : ''}
            </p>
          </div>
        </div>

        {/* 时间范围选择器 + 实时连接状态 */}
        <div className="flex items-center gap-3">
          {/* 实时连接状态 */}
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                publicWsConnected ? 'bg-success animate-pulse' : 'bg-destructive'
              }`}
            />
            <span>{publicWsConnected ? '实时数据' : '已断开'}</span>
          </div>
          {/* 时间范围选择器（公开页仅支持最近 1 小时） */}
          <div className="flex items-center gap-1 rounded-lg border border-border bg-card p-1">
            <button
              className="cursor-default rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground"
            >
              最近 1 小时
            </button>
          </div>
        </div>
      </div>

      {/* 实时指标卡片 */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
        {/* CPU */}
        <MetricCard
          label="CPU"
          value={`${displayServer.cpu.toFixed(1)}%`}
          color={getUsageTextColor(displayServer.cpu)}
        />
        {/* 内存 */}
        <MetricCard
          label="内存"
          value={`${memUsagePercent.toFixed(1)}%`}
          subValue={`${formatBytes(displayServer.mem_used)} / ${formatBytes(displayServer.mem_total)}`}
          color={getUsageTextColor(memUsagePercent)}
        />
        {/* 磁盘 */}
        <MetricCard
          label="磁盘"
          value={`${(displayServer.disk_usage || 0).toFixed(1)}%`}
          color={getUsageTextColor(displayServer.disk_usage || 0)}
        />
        {/* 下行 */}
        <MetricCard
          label="下行"
          value={displayServer.online ? formatSpeed(displayServer.net_rx) : '---'}
        />
        {/* 上行 */}
        <MetricCard
          label="上行"
          value={displayServer.online ? formatSpeed(displayServer.net_tx) : '---'}
        />
        {/* 运行时间 */}
        <MetricCard
          label="运行时间"
          value={displayServer.online ? formatUptime(displayServer.uptime) : '---'}
        />
      </div>

      {/* 扩展指标卡片 */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-6">
        {/* TCP 连接数 */}
        <MetricCard
          label="TCP 连接"
          value={displayServer.online ? String(displayServer.tcp_connections || 0) : '---'}
        />
        {/* UDP 连接数 */}
        <MetricCard
          label="UDP 连接"
          value={displayServer.online ? String(displayServer.udp_connections || 0) : '---'}
        />
        {/* 进程数 */}
        <MetricCard
          label="进程数"
          value={displayServer.online ? String(displayServer.process_count || 0) : '---'}
        />
        {/* Swap 使用 */}
        <MetricCard
          label="Swap"
          value={
            displayServer.online
              ? displayServer.swap_total > 0
                ? `${((displayServer.swap_used / displayServer.swap_total) * 100).toFixed(1)}%`
                : '无'
              : '---'
          }
          subValue={
            displayServer.online && displayServer.swap_total > 0
              ? `${formatBytes(displayServer.swap_used)} / ${formatBytes(displayServer.swap_total)}`
              : undefined
          }
        />
        {/* 负载 (1/5/15 分钟) */}
        <MetricCard
          label="系统负载"
          value={
            displayServer.online
              ? `${displayServer.load_1?.toFixed(2) || '0.00'} / ${displayServer.load_5?.toFixed(2) || '0.00'} / ${displayServer.load_15?.toFixed(2) || '0.00'}`
              : '---'
          }
          subValue={displayServer.online ? '1分 / 5分 / 15分' : undefined}
        />
        {/* 1分钟负载 */}
        <MetricCard
          label="1分钟负载"
          value={displayServer.online ? (displayServer.load_1?.toFixed(2) || '0.00') : '---'}
        />
      </div>

      {/* CPU 图表 */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">CPU 使用率</h2>
          <span className="text-xs text-muted-foreground">最近 1 小时</span>
        </div>
        {chartData.timestamps.length > 0 ? (
          <CpuChart
            timestamps={chartData.timestamps}
            cpuData={chartData.cpuData}
            isDark={isDark}
            height={260}
          />
        ) : (
          <EmptyChart />
        )}
      </div>

      {/* 内存图表 */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">内存使用率</h2>
          <span className="text-xs text-muted-foreground">最近 1 小时</span>
        </div>
        {chartData.timestamps.length > 0 ? (
          <MemoryChart
            timestamps={chartData.timestamps}
            memData={chartData.memData}
            isDark={isDark}
            height={260}
          />
        ) : (
          <EmptyChart />
        )}
      </div>

      {/* 延迟与丢包率融合图 */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">三网延迟与丢包率</h2>
          <span className="text-xs text-muted-foreground">最近 1 小时</span>
        </div>
        {chartData.timestamps.length > 0 ? (
          <PingChart
            timestamps={chartData.timestamps}
            pingData={chartData.pingData}
            isDark={isDark}
            height={400}
          />
        ) : (
          <EmptyChart />
        )}
      </div>

      {/* 系统信息 + 三网详情 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* 系统信息（不显示敏感信息） */}
        <div className="rounded-xl border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">系统信息</h2>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <InfoItem label="主机名" value={displayServer.hostname} />
            <InfoItem label="操作系统" value={displayServer.os || '-'} />
            <InfoItem label="运行时间" value={displayServer.online ? formatUptime(displayServer.uptime) : '---'} />
            <InfoItem label="进程数" value={displayServer.online ? String(displayServer.process_count || 0) : '---'} />
            <InfoItem label="负载(1分)" value={displayServer.load_1?.toFixed(2) || '---'} />
            <InfoItem label="负载(5分)" value={displayServer.load_5?.toFixed(2) || '---'} />
            <InfoItem label="负载(15分)" value={displayServer.load_15?.toFixed(2) || '---'} />
            <InfoItem label="TCP 连接" value={displayServer.online ? String(displayServer.tcp_connections || 0) : '---'} />
            <InfoItem label="UDP 连接" value={displayServer.online ? String(displayServer.udp_connections || 0) : '---'} />
            <InfoItem
              label="Swap"
              value={
                displayServer.online
                  ? displayServer.swap_total > 0
                    ? `${formatBytes(displayServer.swap_used)} / ${formatBytes(displayServer.swap_total)}`
                    : '未启用'
                  : '---'
              }
            />
          </div>

          {/* 磁盘详情列表 */}
          {displayServer.disks && displayServer.disks.length > 0 && (
            <div className="mt-4 border-t border-border pt-3">
              <h3 className="mb-2 text-xs font-medium text-muted-foreground">磁盘分区详情</h3>
              <div className="space-y-2">
                {displayServer.disks.map((disk, idx) => {
                  const usage = disk.total > 0 ? (disk.used / disk.total) * 100 : 0
                  return (
                    <div key={idx} className="rounded-lg bg-secondary/50 px-3 py-2">
                      <div className="flex items-center justify-between text-sm">
                        <span className="font-mono text-foreground">{disk.device}</span>
                        <span className={`font-medium ${getUsageTextColor(usage)}`}>
                          {usage.toFixed(1)}%
                        </span>
                      </div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {formatBytes(disk.used)} / {formatBytes(disk.total)}
                      </div>
                      <div className="mt-1.5">
                        <ProgressBar value={usage} color={getUsageColor(usage)} />
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </div>

        {/* 三网延迟详情 */}
        <div className="rounded-xl border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">三网延迟详情</h2>
          {displayServer.ping_data && displayServer.ping_data.length > 0 ? (
            <div className="space-y-2">
              {displayServer.ping_data.map((ping: PingResult, idx: number) => (
                <div
                  key={idx}
                  className="flex items-center justify-between rounded-lg bg-secondary/50 px-3 py-2"
                >
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-foreground">{ping.name}</span>
                    <span className="text-xs text-muted-foreground">{ping.method}</span>
                  </div>
                  <div className="flex items-center gap-4 text-sm">
                    <div className="text-right">
                      <span className="text-xs text-muted-foreground">延迟</span>
                      <div className="font-medium text-foreground">
                        {displayServer.online ? formatLatency(ping.avg_latency) : '---'}
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="text-xs text-muted-foreground">丢包率</span>
                      <div className={`font-medium ${displayServer.online ? getLossColor(ping.loss) : 'text-muted-foreground'}`}>
                        {displayServer.online ? formatLoss(ping.loss) : '---'}
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="text-xs text-muted-foreground">抖动</span>
                      <div className="font-medium text-foreground">
                        {displayServer.online ? `${ping.jitter.toFixed(1)} ms` : '---'}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-6 text-center text-sm text-muted-foreground">
              暂无 Ping 探测数据
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

/** 指标卡片 */
function MetricCard({
  label,
  value,
  subValue,
  color,
}: {
  label: string
  value: string
  subValue?: string
  color?: string
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={`mt-1 text-lg font-bold ${color || 'text-foreground'}`}>
        {value}
      </div>
      {subValue && (
        <div className="mt-0.5 text-xs text-muted-foreground">{subValue}</div>
      )}
    </div>
  )
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

/** 信息项 */
function InfoItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-0.5 font-medium text-foreground">{value}</div>
    </div>
  )
}

/** 空图表占位 */
function EmptyChart() {
  return (
    <div className="flex h-[260px] items-center justify-center">
      <div className="flex flex-col items-center gap-2">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        <span className="text-xs text-muted-foreground">等待实时数据...</span>
      </div>
    </div>
  )
}
