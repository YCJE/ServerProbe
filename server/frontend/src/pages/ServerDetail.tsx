import { useEffect, useState, useMemo, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { getServerHistory } from '@/lib/api'
import type { TimeRange, HistoryData, PingResult } from '@/types'
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

/** 时间范围选项 */
const TIME_RANGES: { value: TimeRange; label: string }[] = [
  { value: '1h', label: '1小时' },
  { value: '6h', label: '6小时' },
  { value: '12h', label: '12小时' },
  { value: '1d', label: '1天' },
  { value: '2d', label: '2天' },
]

/** 判断是否为实时范围（使用 WebSocket 数据） */
function isRealtimeRange(range: TimeRange): boolean {
  return range === '1h' || range === '6h'
}

/** 服务器详情页 */
export default function ServerDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const serverId = parseInt(id || '0', 10)

  // Store
  const currentServer = useServerStore((s) => s.currentServer)
  const currentServerLoading = useServerStore((s) => s.currentServerLoading)
  const fetchServerDetail = useServerStore((s) => s.fetchServerDetail)
  const realtimeHistory = useServerStore((s) => s.realtimeHistory)
  const clearRealtimeHistory = useServerStore((s) => s.clearRealtimeHistory)
  const theme = useServerStore((s) => s.theme)
  const dashboardData = useServerStore((s) => s.dashboardData)

  // 本地状态
  const [timeRange, setTimeRange] = useState<TimeRange>('1h')
  const [historyData, setHistoryData] = useState<HistoryData | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)

  const isDark = useMemo(() => {
    if (theme === 'dark') return true
    if (theme === 'light') return false
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  }, [theme])

  // 加载服务器详情
  useEffect(() => {
    if (serverId > 0) {
      fetchServerDetail(serverId).catch(() => {})
      clearRealtimeHistory()
    }
    return () => {
      clearRealtimeHistory()
    }
  }, [serverId, fetchServerDetail, clearRealtimeHistory])

  // 加载历史数据（非实时范围时）
  const loadHistory = useCallback(async (range: TimeRange) => {
    if (isRealtimeRange(range)) {
      setHistoryData(null)
      return
    }

    setHistoryLoading(true)
    try {
      const data = await getServerHistory(serverId, range)
      setHistoryData(data)
    } catch (err) {
      console.error('加载历史数据失败:', err)
      setHistoryData(null)
    } finally {
      setHistoryLoading(false)
    }
  }, [serverId])

  // 时间范围变化时加载历史数据
  useEffect(() => {
    loadHistory(timeRange)
  }, [timeRange, loadHistory])

  // 定时刷新历史数据（非实时范围时，每 5 分钟刷新）
  useEffect(() => {
    if (isRealtimeRange(timeRange)) return

    const interval = setInterval(() => {
      loadHistory(timeRange)
    }, 5 * 60 * 1000)

    return () => clearInterval(interval)
  }, [timeRange, loadHistory])

  // 实时数据来自 WebSocket 的 dashboardData
  const liveData = dashboardData.get(serverId)

  // 合并当前服务器信息和实时数据
  const displayServer = useMemo(() => {
    if (!currentServer) return null
    if (liveData) {
      return {
        ...currentServer,
        online: liveData.online,
        cpu: liveData.cpu,
        mem: liveData.mem,
        mem_total: liveData.mem_total,
        mem_used: liveData.mem_used,
        net_rx: liveData.net_rx,
        net_tx: liveData.net_tx,
        uptime: liveData.uptime,
        load_1: liveData.load_1,
        disk_usage: liveData.disk_usage,
        ping_data: liveData.ping_data,
      }
    }
    return currentServer
  }, [currentServer, liveData])

  // 图表数据
  const chartData = useMemo(() => {
    if (isRealtimeRange(timeRange)) {
      // 使用实时历史数据
      const points = realtimeHistory
      const cutoffTime = timeRange === '1h'
        ? Date.now() / 1000 - 3600
        : Date.now() / 1000 - 6 * 3600

      const filtered = points.filter((p) => p.timestamp >= cutoffTime)

      return {
        timestamps: filtered.map((p) => p.timestamp),
        cpuData: filtered.map((p) => p.cpu),
        memData: filtered.map((p) => p.mem),
        pingData: filtered.map((p) => p.ping_data || []),
      }
    }

    // 使用历史 API 数据
    if (!historyData || !historyData.points) {
      return { timestamps: [], cpuData: [], memData: [], pingData: [] }
    }

    return {
      timestamps: historyData.points.map((p) => p.timestamp),
      cpuData: historyData.points.map((p) => p.cpu_usage),
      memData: historyData.points.map((p) => p.mem_usage),
      pingData: historyData.points.map((p) => p.ping_data || []),
    }
  }, [timeRange, realtimeHistory, historyData])

  // 加载中
  if (currentServerLoading && !currentServer) {
    return (
      <div className="flex h-full items-center justify-center">
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
        <p className="text-sm text-muted-foreground">服务器不存在</p>
        <button
          onClick={() => navigate('/')}
          className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
        >
          返回仪表盘
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
              <h1 className="text-xl font-bold text-foreground">{displayServer.hostname}</h1>
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
              {displayServer.os} · {displayServer.arch} · Agent v{displayServer.agent_version}
            </p>
          </div>
        </div>

        {/* 时间范围切换 */}
        <div className="flex items-center gap-1 rounded-lg border border-border bg-card p-1">
          {TIME_RANGES.map((range) => (
            <button
              key={range.value}
              onClick={() => setTimeRange(range.value)}
              className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                timeRange === range.value
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:bg-accent hover:text-foreground'
              }`}
            >
              {range.label}
            </button>
          ))}
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

      {/* CPU 图表 */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">CPU 使用率</h2>
          <span className="text-xs text-muted-foreground">
            {isRealtimeRange(timeRange) ? '实时数据' : '历史数据'}
          </span>
        </div>
        {chartData.timestamps.length > 0 ? (
          <CpuChart
            timestamps={chartData.timestamps}
            cpuData={chartData.cpuData}
            isDark={isDark}
            height={260}
          />
        ) : (
          <EmptyChart loading={historyLoading} />
        )}
      </div>

      {/* 内存图表 */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">内存使用率</h2>
          <span className="text-xs text-muted-foreground">
            {isRealtimeRange(timeRange) ? '实时数据' : '历史数据'}
          </span>
        </div>
        {chartData.timestamps.length > 0 ? (
          <MemoryChart
            timestamps={chartData.timestamps}
            memData={chartData.memData}
            isDark={isDark}
            height={260}
          />
        ) : (
          <EmptyChart loading={historyLoading} />
        )}
      </div>

      {/* 延迟与丢包率融合图 */}
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-foreground">三网延迟与丢包率</h2>
          <span className="text-xs text-muted-foreground">
            {isRealtimeRange(timeRange) ? '实时数据' : '历史数据'}
          </span>
        </div>
        {chartData.timestamps.length > 0 ? (
          <PingChart
            timestamps={chartData.timestamps}
            pingData={chartData.pingData}
            isDark={isDark}
            height={400}
          />
        ) : (
          <EmptyChart loading={historyLoading} />
        )}
      </div>

      {/* 系统信息 + 三网详情 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* 系统信息 */}
        <div className="rounded-xl border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">系统信息</h2>
          <div className="grid grid-cols-2 gap-3 text-sm">
            <InfoItem label="主机名" value={displayServer.hostname} />
            <InfoItem label="操作系统" value={displayServer.os} />
            <InfoItem label="架构" value={displayServer.arch} />
            <InfoItem label="Agent 版本" value={displayServer.agent_version} />
            <InfoItem label="运行时间" value={displayServer.online ? formatUptime(displayServer.uptime) : '---'} />
            <InfoItem label="负载(1分)" value={displayServer.load_1?.toFixed(2) || '---'} />
          </div>
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
function EmptyChart({ loading }: { loading: boolean }) {
  return (
    <div className="flex h-[260px] items-center justify-center">
      {loading ? (
        <div className="flex flex-col items-center gap-2">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          <span className="text-xs text-muted-foreground">加载中...</span>
        </div>
      ) : (
        <span className="text-sm text-muted-foreground">暂无数据</span>
      )}
    </div>
  )
}
