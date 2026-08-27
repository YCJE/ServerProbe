import { useEffect, useMemo, useState, useCallback, useRef } from 'react'
import type { ReactNode } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { useSiteSettings } from '@/store/useSiteSettingsStore'
import { getServerHistory } from '@/lib/api'
import type { TimeRange, HistoryData } from '@/types'
import NetworkQualityChart, { type ChartSeries } from '@/components/NetworkQualityChart'
import Sparkline from '@/components/Sparkline'
import LatencyQualityBar, { type LatencyPoint } from '@/components/LatencyQualityBar'
import OnlineTimeline, { type OnlineTimelinePoint } from '@/components/OnlineTimeline'
import ResourceRing from '@/components/ResourceRing'
import DistroIcon from '@/components/DistroIcon'
import LatencyGrid, { type LatencyGridPoint } from '@/components/LatencyGrid'
import PingStatsTable, { type PingStatRow } from '@/components/PingStatsTable'
import {
  formatBytes,
  formatSpeed,
  formatUptime,
  formatLoss,
  formatRelativeTime,
  getUsageTextColor,
  getLossColor,
  parsePingData,
} from '@/lib/utils'

/** 时间范围选项（管理端含实时模式；≥7d 走小时聚合层） */
const TIME_RANGES: { value: TimeRange; label: string }[] = [
  { value: 'realtime', label: '实时' },
  { value: '1h', label: '1小时' },
  { value: '6h', label: '6小时' },
  { value: '12h', label: '12小时' },
  { value: '1d', label: '24小时' },
  { value: '2d', label: '2天' },
  { value: '3d', label: '3天' },
  { value: '7d', label: '7天' },
  { value: '30d', label: '30天' },
  { value: '90d', label: '90天' },
  { value: '1y', label: '1年' },
]

/** 判断是否为实时范围（使用 WebSocket 数据） */
function isRealtimeRange(range: TimeRange): boolean {
  return range === 'realtime'
}

/** ping 目标线条颜色池 */
const PING_COLORS = ['#5AC8FA', '#34C759', '#FF9500', '#AF52DE', '#FF2D55', '#FFCC00']

/** Sparkline 配色 */
const SPARK_CPU = '#007AFF'
const SPARK_MEM = '#34C759'
const SPARK_RX = '#5AC8FA'
const SPARK_TX = '#AF52DE'

/** Sparkline / 实时图表最多展示的数据点数 */
const MAX_SPARK_POINTS = 60

/** 稳定的空数组引用（cardHistory 缺省值） */
const EMPTY_GRID_HISTORY: LatencyGridPoint[] = []

/** 历史数据定时刷新间隔 */
const HISTORY_REFRESH_INTERVAL = 5 * 60 * 1000

/** 服务器详情页（管理端，NodeGet 风格） */
export default function ServerDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const serverId = parseInt(id || '0', 10)

  // Store
  const currentServer = useServerStore((s) => s.currentServer)
  const currentServerLoading = useServerStore((s) => s.currentServerLoading)
  const fetchServerDetail = useServerStore((s) => s.fetchServerDetail)
  const abortCurrentFetch = useServerStore((s) => s.abortCurrentFetch)
  const realtimeHistory = useServerStore((s) => s.realtimeHistory)
  const clearRealtimeHistory = useServerStore((s) => s.clearRealtimeHistory)
  const liveData = useServerStore((s) => s.dashboardData.get(serverId))
  // 延迟格子图数据源：卡片历史滚动窗口（Layout 挂载起累积，不随时间范围切换变化）
  const cardHistory = useServerStore((s) => s.cardHistory.get(serverId) ?? EMPTY_GRID_HISTORY)
  // 默认历史范围（后台"站点设置"可配置）
  const { defaultHistoryRange } = useSiteSettings()

  // 本地状态
  const [timeRange, setTimeRange] = useState<TimeRange>('1h')
  const [historyData, setHistoryData] = useState<HistoryData | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)

  // 站点设置加载完成后同步默认历史范围（仅初始化时应用，用户手动切换后不再覆盖）
  const rangeAppliedRef = useRef(false)
  useEffect(() => {
    if (!rangeAppliedRef.current && defaultHistoryRange) {
      rangeAppliedRef.current = true
      setTimeRange(defaultHistoryRange as TimeRange)
    }
  }, [defaultHistoryRange])

  // 资源环形图响应式尺寸：移动 112px / 桌面 124px
  const [ringSize, setRingSize] = useState(112)
  useEffect(() => {
    const update = () => setRingSize(window.innerWidth >= 640 ? 124 : 112)
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [])

  // 防止卸载后 setState
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  // 防止快速切换时间范围时旧请求覆盖新数据
  const historyRequestIdRef = useRef(0)

  // 加载服务器详情
  useEffect(() => {
    setHistoryData(null)
    if (serverId > 0) {
      fetchServerDetail(serverId).catch(() => {})
    }
    return () => {
      clearRealtimeHistory()
      abortCurrentFetch()
    }
  }, [serverId, fetchServerDetail, abortCurrentFetch, clearRealtimeHistory])

  // 加载历史数据
  const loadHistory = useCallback(async (range: TimeRange) => {
    const requestId = ++historyRequestIdRef.current
    if (isRealtimeRange(range)) {
      if (mountedRef.current && historyRequestIdRef.current === requestId) {
        setHistoryData(null)
        setHistoryLoading(false)
      }
      return
    }

    if (mountedRef.current && historyRequestIdRef.current === requestId) {
      setHistoryLoading(true)
    }
    try {
      const data = await getServerHistory(serverId, range)
      if (mountedRef.current && historyRequestIdRef.current === requestId) {
        setHistoryData(data)
      }
    } catch (err) {
      console.error('加载历史数据失败:', err)
      if (mountedRef.current && historyRequestIdRef.current === requestId) {
        setHistoryData(null)
      }
    } finally {
      if (mountedRef.current && historyRequestIdRef.current === requestId) {
        setHistoryLoading(false)
      }
    }
  }, [serverId])

  useEffect(() => {
    loadHistory(timeRange)
  }, [timeRange, loadHistory])

  // 定时刷新历史数据（非实时范围时，标签页隐藏时暂停）
  useEffect(() => {
    if (isRealtimeRange(timeRange)) return

    let interval: ReturnType<typeof setInterval> | null = null

    const handleVisibilityChange = () => {
      if (document.hidden) {
        if (interval) {
          clearInterval(interval)
          interval = null
        }
      } else {
        loadHistory(timeRange)
        interval = setInterval(() => loadHistory(timeRange), HISTORY_REFRESH_INTERVAL)
      }
    }

    // 页面初始可见时创建 interval，否则等待变为可见时再创建
    if (!document.hidden) {
      interval = setInterval(() => loadHistory(timeRange), HISTORY_REFRESH_INTERVAL)
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      if (interval) clearInterval(interval)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [timeRange, loadHistory])

  // 合并当前服务器信息和实时数据
  const displayServer = useMemo(() => {
    if (!currentServer) return null
    if (liveData) {
      return {
        ...currentServer,
        online: liveData.online,
        cpu: liveData.cpu,
        cpu_model: liveData.cpu_model || currentServer.cpu_model,
        cpu_cores: liveData.cpu_cores || currentServer.cpu_cores,
        os: liveData.os || currentServer.os,
        arch: liveData.arch || currentServer.arch,
        agent_version: liveData.agent_version || currentServer.agent_version,
        last_seen: liveData.timestamp,
        mem: liveData.mem,
        mem_total: liveData.mem_total,
        mem_used: liveData.mem_used,
        swap_total: liveData.swap_total || 0,
        swap_used: liveData.swap_used || 0,
        net_rx: liveData.net_rx,
        net_tx: liveData.net_tx,
        total_rx: liveData.total_rx ?? currentServer.total_rx ?? 0,
        total_tx: liveData.total_tx ?? currentServer.total_tx ?? 0,
        uptime: liveData.uptime,
        load_1: liveData.load_1 || 0,
        load_5: liveData.load_5 || 0,
        load_15: liveData.load_15 || 0,
        disk_usage: liveData.disk_usage ?? 0,
        disks: liveData.disks || [],
        tcp_connections: liveData.tcp_connections || 0,
        udp_connections: liveData.udp_connections || 0,
        process_count: liveData.process_count || 0,
        ping_data: liveData.ping_data || [],
        // 新增字段
        virtualization: liveData.virtualization || currentServer.virtualization,
        distro: liveData.distro || currentServer.distro,
        processes: liveData.processes || currentServer.processes,
        time_offset: liveData.time_offset ?? currentServer.time_offset,
        temperature: liveData.temperature ?? currentServer.temperature,
        ipv4: liveData.ipv4 ?? currentServer.ipv4,
        ipv6: liveData.ipv6 ?? currentServer.ipv6,
      }
    }
    return currentServer
  }, [currentServer, liveData])

  // 统一提取 ping 数据源（实时/历史），网络质量图表与延迟统计表共用，避免重复 parsePingData
  // 拆分实时/历史分支依赖，避免历史模式下 realtimeHistory 变化触发无意义重计算
  const pingSource = useMemo<{
    timestamps: number[]
    allPings: ReturnType<typeof parsePingData>[]
  }>(() => {
    if (isRealtimeRange(timeRange)) {
      // 实时模式：从 realtimeHistory 提取
      const points = realtimeHistory.slice(-120)
      return {
        timestamps: points.map((p) => p.timestamp),
        allPings: points.map((p) => parsePingData(p.ping_data)),
      }
    }
    // 历史模式：从 historyData 提取
    if (!historyData || !historyData.points || historyData.points.length === 0) {
      return { timestamps: [], allPings: [] }
    }
    return {
      timestamps: historyData.points.map((p) => p.timestamp),
      allPings: historyData.points.map((p) => parsePingData(p.ping_data)),
    }
    // 实时模式依赖 realtimeHistory，历史模式依赖 historyData，避免交叉触发
  }, [timeRange, isRealtimeRange(timeRange) ? realtimeHistory : historyData])

  // 从 pingSource 构建网络质量图表数据（按目标分组成时间序列）
  const networkChartData = useMemo<{
    timestamps: number[]
    series: ChartSeries[]
  }>(() => {
    const { timestamps, allPings } = pingSource
    if (timestamps.length === 0) return { timestamps: [], series: [] }

    // 收集所有唯一的 ping 目标名称（保持出现顺序）
    const targetNames: string[] = []
    const seen = new Set<string>()
    for (const pings of allPings) {
      for (const ping of pings) {
        if (!seen.has(ping.name)) {
          seen.add(ping.name)
          targetNames.push(ping.name)
        }
      }
    }

    const series: ChartSeries[] = targetNames.map((name, i) => {
      const lossData = allPings.map((pings) => {
        const ping = pings.find((pp) => pp.name === name)
        return ping ? (ping.loss ?? null) : null
      })
      const validLosses = lossData.filter((l): l is number => l !== null && l >= 0)
      const avgSeriesLoss = validLosses.length > 0
        ? validLosses.reduce((sum, l) => sum + l, 0) / validLosses.length
        : undefined
      return {
        name,
        color: PING_COLORS[i % PING_COLORS.length],
        data: allPings.map((pings) => {
          const ping = pings.find((pp) => pp.name === name)
          return ping ? (ping.avg_latency ?? null) : null
        }),
        loss: avgSeriesLoss,
        lossData,
      }
    })

    return { timestamps, series }
  }, [pingSource])

  // 延迟统计表（NodeGet 风格）：每个探测目标在所选时间范围内的平均/最低/最高延迟、抖动、丢包率
  const pingStats = useMemo<PingStatRow[]>(() => {
    const { allPings } = pingSource
    if (allPings.length === 0) return []

    const targetNames: string[] = []
    const seen = new Set<string>()
    for (const pings of allPings) {
      for (const ping of pings) {
        if (!seen.has(ping.name)) {
          seen.add(ping.name)
          targetNames.push(ping.name)
        }
      }
    }

    return targetNames.map((name, i) => {
      const samples = allPings
        .map((pings) => pings.find((pp) => pp.name === name))
        .filter((p): p is NonNullable<typeof p> => !!p)

      const mean = (nums: number[]) =>
        nums.length > 0 ? nums.reduce((s, v) => s + v, 0) / nums.length : null

      const avgs = samples.map((p) => p.avg_latency).filter((v): v is number => v != null && v >= 0)
      const mins = samples.map((p) => p.min_latency ?? p.avg_latency).filter((v): v is number => v != null && v >= 0)
      const maxs = samples.map((p) => p.max_latency ?? p.avg_latency).filter((v): v is number => v != null && v >= 0)
      const jitters = samples.map((p) => p.jitter).filter((v): v is number => v != null && v >= 0)
      const losses = samples.map((p) => p.loss).filter((v): v is number => v != null && v >= 0)

      return {
        name,
        color: PING_COLORS[i % PING_COLORS.length],
        samples: samples.length,
        avg: mean(avgs),
        min: mins.length > 0 ? Math.min(...mins) : null,
        max: maxs.length > 0 ? Math.max(...maxs) : null,
        jitter: mean(jitters),
        loss: mean(losses),
      }
    })
  }, [pingSource])

  // Sparkline 数据：实时模式取 realtimeHistory，历史模式取 historyData
  // peak 为所选范围内的峰值：小时聚合层用每点真实 max（5 分钟层 max=均值，即区间最高均值）
  const sparklineData = useMemo(() => {
    const maxOf = (arr: number[]) => (arr.length ? Math.max(...arr) : 0)
    if (isRealtimeRange(timeRange)) {
      const recent = realtimeHistory.slice(-MAX_SPARK_POINTS)
      const cpu = recent.map((p) => p.cpu)
      const mem = recent.map((p) => p.mem)
      const netRx = recent.map((p) => p.net_rx)
      const netTx = recent.map((p) => p.net_tx)
      return {
        cpu, mem, netRx, netTx,
        cpuPeak: maxOf(cpu), memPeak: maxOf(mem), netRxPeak: maxOf(netRx), netTxPeak: maxOf(netTx),
      }
    }
    // 历史模式：从 historyData 提取，均匀采样到最多 MAX_SPARK_POINTS 个点
    if (!historyData || !historyData.points || historyData.points.length === 0) {
      return { cpu: [], mem: [], netRx: [], netTx: [], cpuPeak: 0, memPeak: 0, netRxPeak: 0, netTxPeak: 0 }
    }
    const points = historyData.points
    const step = Math.max(1, Math.ceil(points.length / MAX_SPARK_POINTS))
    const sampled = points.filter((_, i) => i % step === 0)
    return {
      cpu: sampled.map((p) => p.cpu_usage),
      mem: sampled.map((p) => p.mem_usage),
      netRx: sampled.map((p) => p.net_rx),
      netTx: sampled.map((p) => p.net_tx),
      cpuPeak: sampled.reduce((m, p) => Math.max(m, p.cpu_max ?? p.cpu_usage), 0),
      memPeak: sampled.reduce((m, p) => Math.max(m, p.mem_max ?? p.mem_usage), 0),
      netRxPeak: sampled.reduce((m, p) => Math.max(m, p.net_rx_max ?? p.net_rx), 0),
      netTxPeak: sampled.reduce((m, p) => Math.max(m, p.net_tx_max ?? p.net_tx), 0),
    }
  }, [timeRange, isRealtimeRange(timeRange) ? realtimeHistory : historyData])

  // 平均丢包率（从图表数据源计算，与所选时间范围一致）
  const avgLoss = useMemo(() => {
    const allSeries = networkChartData.series
    if (allSeries.length === 0) return 0
    let totalLoss = 0
    let count = 0
    for (const s of allSeries) {
      if (!s.lossData) continue
      for (const l of s.lossData) {
        if (l !== null && l >= 0) {
          totalLoss += l
          count++
        }
      }
    }
    return count > 0 ? totalLoss / count : 0
  }, [networkChartData])

  // 延迟质量条形图数据：实时模式用 realtimeHistory，历史模式用 historyData
  // 历史数据为 5 分钟粒度聚合点，不与 3 秒粒度的实时点混合（避免 WS 每次推送
  // 触发全量重建数组 → LatencyQualityBar 全量重渲染导致的页面卡顿）
  const latencyQualityPoints = useMemo<LatencyPoint[]>(() => {
    // 实时模式：使用 realtimeHistory
    if (isRealtimeRange(timeRange)) {
      return realtimeHistory.map((p) => ({
        timestamp: p.timestamp,
        ping_data: p.ping_data,
      }))
    }
    // 历史模式：仅使用 historyData（随 5 分钟定时刷新更新）
    if (historyData && historyData.points && historyData.points.length > 0) {
      return historyData.points.map((p) => ({
        timestamp: p.timestamp,
        ping_data: p.ping_data,
      }))
    }
    // 回退到实时数据（历史数据尚未加载时）
    return realtimeHistory.map((p) => ({
      timestamp: p.timestamp,
      ping_data: p.ping_data,
    }))
  }, [timeRange, isRealtimeRange(timeRange) ? realtimeHistory : historyData])

  // 在线状态时间线数据：实时模式用 realtimeHistory，历史模式用 historyData
  // （后端聚合记录带 online 字段，离线时段有占位记录，历史时间线完整可查）
  const onlineTimelinePoints = useMemo<OnlineTimelinePoint[]>(() => {
    const result: OnlineTimelinePoint[] = []
    if (isRealtimeRange(timeRange)) {
      for (const p of realtimeHistory) {
        result.push({
          timestamp: p.timestamp,
          online: p.online,
        })
      }
      return result
    }
    if (historyData && historyData.points && historyData.points.length > 0) {
      for (const p of historyData.points) {
        if (p.online !== undefined) {
          result.push({
            timestamp: p.timestamp,
            online: p.online,
          })
        }
      }
    }
    return result
  }, [timeRange, isRealtimeRange(timeRange) ? realtimeHistory : historyData])

  // ==================== 加载 / 错误状态 ====================

  if (currentServerLoading && !currentServer) {
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
        <p className="text-sm text-muted-foreground">服务器不存在</p>
        <button
          onClick={() => navigate('/admin')}
          className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
        >
          返回仪表盘
        </button>
      </div>
    )
  }

  // ==================== 派生数据 ====================

  const memUsagePercent =
    displayServer.mem_total > 0
      ? ((displayServer.mem_used || 0) / displayServer.mem_total) * 100
      : displayServer.mem || 0
  const diskTotal =
    displayServer.disks?.reduce((sum, d) => sum + d.total, 0) || 0
  const diskUsed =
    displayServer.disks?.reduce((sum, d) => sum + d.used, 0) || 0
  const swapUsagePercent =
    displayServer.swap_total > 0
      ? ((displayServer.swap_used || 0) / displayServer.swap_total) * 100
      : null

  // ==================== 渲染 ====================

  return (
    <div className="flex flex-col gap-4 lg:flex-row">
      {/* ============ 左侧边栏 ============ */}
      <aside className="w-full shrink-0 space-y-4 lg:w-[260px]">
        {/* 返回按钮 */}
        <button
          onClick={() => navigate('/admin')}
          className="flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M10 19l-7-7m0 0l7-7m-7 7h18"
            />
          </svg>
          返回仪表盘
        </button>

        {/* 服务器头部 */}
        <div className="card-soft p-5">
          <div className="flex items-center gap-2">
            <span
              className={`inline-block h-2.5 w-2.5 shrink-0 rounded-full ${
                displayServer.online
                  ? 'bg-success animate-pulse'
                  : 'bg-destructive'
              }`}
            />
            <h1 className="min-w-0 flex-1 truncate text-base font-bold text-foreground">
              {displayServer.display_name || displayServer.hostname}
            </h1>
            {/* 发行版图标 */}
            <DistroIcon distro={displayServer.distro} os={displayServer.os} size={18} showLabel />
          </div>
          <div className="mt-2 flex items-center gap-2">
            <span
              className={`badge-pill ${
                displayServer.online ? 'badge-success' : 'badge-destructive'
              }`}
            >
              {displayServer.online ? '在线' : '离线'}
            </span>
            <span className="truncate text-xs text-muted-foreground">
              {displayServer.hostname}
            </span>
            {/* 虚拟化 Badge */}
            {displayServer.virtualization &&
              displayServer.virtualization !== 'None' &&
              displayServer.virtualization !== 'none' && (
                <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                  {displayServer.virtualization}
                </span>
              )}
          </div>
          {/* 出口 IP（NodeGet 风格：IPv4 完整显示，IPv6 折叠） */}
          {(displayServer.ipv4 || displayServer.ipv6) && (
            <div className="mt-2 space-y-1 text-xs text-muted-foreground">
              {displayServer.ipv4 && (
                <div className="flex items-center gap-1.5">
                  <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[9px] font-semibold">IPv4</span>
                  <span className="truncate font-mono" title={displayServer.ipv4}>
                    {displayServer.ipv4}
                  </span>
                </div>
              )}
              {displayServer.ipv6 && (
                <div className="flex items-center gap-1.5">
                  <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[9px] font-semibold">IPv6</span>
                  <span className="truncate font-mono" title={displayServer.ipv6}>
                    {displayServer.ipv6.length > 24
                      ? `${displayServer.ipv6.slice(0, 24)}…`
                      : displayServer.ipv6}
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* 硬件信息卡片 */}
        <div className="card-soft p-5">
          <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">
            硬件信息
          </h3>
          <div>
            <InfoRow label="CPU 型号" value={displayServer.cpu_model || '-'} />
            <InfoRow
              label="核心数"
              value={displayServer.cpu_cores != null ? `${displayServer.cpu_cores} 核` : '-'}
            />
            <InfoRow label="内存" value={formatBytes(displayServer.mem_total)} />
            <InfoRow label="硬盘" value={diskTotal > 0 ? formatBytes(diskTotal) : '-'} />
            {/* CPU 温度（>0 表示有传感器读数） */}
            {displayServer.temperature != null && displayServer.temperature > 0 && (
              <InfoRow
                label="CPU 温度"
                value={`${displayServer.temperature.toFixed(1)} °C`}
                valueClassName={
                  displayServer.temperature >= 85
                    ? 'text-destructive'
                    : displayServer.temperature >= 70
                      ? 'text-warning'
                      : 'text-foreground'
                }
              />
            )}
            <InfoRow label="系统" value={displayServer.os || '-'} />
            {/* 发行版信息（如果有） */}
            {displayServer.distro && (
              <InfoRow label="发行版" value={displayServer.distro} />
            )}
            {/* 虚拟化信息（如果有） */}
            {displayServer.virtualization &&
              displayServer.virtualization !== 'None' &&
              displayServer.virtualization !== 'none' && (
                <InfoRow label="虚拟化" value={displayServer.virtualization} />
              )}
            {/* NTP 时间偏移（如果有） */}
            {displayServer.time_offset !== undefined && displayServer.time_offset !== null && (
              <InfoRow
                label="NTP 偏移"
                value={`${displayServer.time_offset > 0 ? '+' : ''}${displayServer.time_offset.toFixed(2)} ms`}
              />
            )}
            <InfoRow label="架构" value={displayServer.arch || '-'} />
            <InfoRow label="Agent 版本" value={displayServer.agent_version || '-'} />
          </div>
        </div>

        {/* 系统信息卡片 */}
        <div className="card-soft p-5">
          <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">
            系统信息
          </h3>
          <div>
            <InfoRow
              label="运行时间"
              value={displayServer.online ? formatUptime(displayServer.uptime) : '---'}
            />
            <InfoRow
              label="负载 (1/5/15分)"
              value={
                displayServer.online
                  ? `${(displayServer.load_1 || 0).toFixed(2)} / ${(displayServer.load_5 || 0).toFixed(2)} / ${(displayServer.load_15 || 0).toFixed(2)}`
                  : '---'
              }
            />
            <InfoRow
              label="TCP 连接"
              value={displayServer.online ? String(displayServer.tcp_connections || 0) : '---'}
            />
            <InfoRow
              label="UDP 连接"
              value={displayServer.online ? String(displayServer.udp_connections || 0) : '---'}
            />
            <InfoRow
              label="进程数"
              value={displayServer.online ? String(displayServer.process_count || 0) : '---'}
            />
            <InfoRow
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
        </div>

        {/* 磁盘使用详情（仅有数据时显示） */}
        {displayServer.disks && displayServer.disks.length > 0 && (
          <div className="card-soft p-5">
            <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">
              磁盘使用
            </h3>
            <div className="space-y-2">
              {displayServer.disks.map((disk, i) => {
                const usage = disk.total > 0 ? (disk.used / disk.total) * 100 : 0
                return (
                  <div key={disk.device || `disk-${i}`} className="space-y-1">
                    <div className="flex items-center justify-between text-xs">
                      <span className="truncate text-muted-foreground">{disk.device || `磁盘 ${i + 1}`}</span>
                      <span className={`font-bold tabular-nums ${getUsageTextColor(usage)}`}>
                        {usage.toFixed(1)}%
                      </span>
                    </div>
                    <div className="h-1 w-full overflow-hidden rounded-full bg-secondary">
                      <div
                        className="h-full rounded-full transition-all duration-500"
                        style={{
                          width: `${Math.min(usage, 100)}%`,
                          backgroundColor: usage > 90 ? '#f56565' : usage > 70 ? '#f6ad55' : '#42b983',
                        }}
                      />
                    </div>
                    <div className="text-[10px] text-muted-foreground">
                      {formatBytes(disk.used)} / {formatBytes(disk.total)}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </aside>

      {/* ============ 右侧主内容区（NodeGet NodeDetail 结构） ============ */}
      <div className="min-w-0 flex-1 space-y-4">
        {/* 标题行 + 时间范围选择器（filter-pill） */}
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="text-lg font-semibold text-foreground">监控详情</h2>
          <div className="flex items-center gap-1.5 overflow-x-auto scrollbar-thin">
            {TIME_RANGES.map((range) => (
              <button
                key={range.value}
                onClick={() => setTimeRange(range.value)}
                className={`shrink-0 filter-pill ${
                  timeRange === range.value
                    ? 'filter-pill-active'
                    : 'filter-pill-inactive'
                }`}
              >
                {range.label}
              </button>
            ))}
          </div>
        </div>

        {/* 资源使用（NodeGet Resources：环形图 + 子标签，grid 横排） */}
        <div className="card-soft p-5">
          <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-4">资源</h3>
          <div className="grid grid-cols-2 gap-x-4 gap-y-6 sm:grid-cols-4 sm:gap-8">
            <ResourceRing
              label="CPU"
              value={displayServer.cpu || 0}
              size={ringSize}
              detail
              sub={
                displayServer.load_1 != null
                  ? `${(displayServer.load_1 || 0).toFixed(2)} / ${(displayServer.load_5 || 0).toFixed(2)} / ${(displayServer.load_15 || 0).toFixed(2)}`
                  : undefined
              }
            />
            <ResourceRing
              label="内存"
              value={memUsagePercent}
              size={ringSize}
              detail
              sub={
                displayServer.mem_total > 0
                  ? `${formatBytes(displayServer.mem_used, 1)} / ${formatBytes(displayServer.mem_total, 1)}`
                  : undefined
              }
            />
            <ResourceRing
              label="硬盘"
              value={displayServer.disk_usage || 0}
              size={ringSize}
              detail
              sub={
                diskTotal > 0
                  ? `${formatBytes(diskUsed, 1)} / ${formatBytes(diskTotal, 1)}`
                  : undefined
              }
            />
            <ResourceRing
              label="Swap"
              value={swapUsagePercent}
              size={ringSize}
              detail
              sub={
                displayServer.swap_total > 0
                  ? `${formatBytes(displayServer.swap_used || 0, 1)} / ${formatBytes(displayServer.swap_total, 1)}`
                  : '未启用'
              }
            />
          </div>
        </div>

        {/* 趋势（NodeGet N-Second Trend：4 张 Sparkline 卡片，历史模式附带峰值） */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <TrendCard
            label="CPU"
            value={`${(displayServer.cpu || 0).toFixed(1)}%`}
            peak={sparklineData.cpu.length > 0 ? `${sparklineData.cpuPeak.toFixed(1)}%` : undefined}
            color={SPARK_CPU}
          >
            <Sparkline data={sparklineData.cpu} color={SPARK_CPU} height={40} />
          </TrendCard>

          <TrendCard
            label="内存"
            value={`${memUsagePercent.toFixed(1)}%`}
            subValue={`${formatBytes(displayServer.mem_used)}`}
            peak={sparklineData.mem.length > 0 ? `${sparklineData.memPeak.toFixed(1)}%` : undefined}
            color={SPARK_MEM}
          >
            <Sparkline data={sparklineData.mem} color={SPARK_MEM} height={40} />
          </TrendCard>

          <TrendCard
            label="下行"
            value={displayServer.net_rx != null ? formatSpeed(displayServer.net_rx) : '---'}
            peak={sparklineData.netRx.length > 0 ? formatSpeed(sparklineData.netRxPeak) : undefined}
            color={SPARK_RX}
          >
            <Sparkline data={sparklineData.netRx} color={SPARK_RX} height={40} />
          </TrendCard>

          <TrendCard
            label="上行"
            value={displayServer.net_tx != null ? formatSpeed(displayServer.net_tx) : '---'}
            peak={sparklineData.netTx.length > 0 ? formatSpeed(sparklineData.netTxPeak) : undefined}
            color={SPARK_TX}
          >
            <Sparkline data={sparklineData.netTx} color={SPARK_TX} height={40} />
          </TrendCard>
        </div>

        {/* 延迟格子图（VPS 用户最常看的数据，IPv4/IPv6 分组，每目标一行） */}
        <div className="card-soft p-5">
          <LatencyGrid points={cardHistory} ipVersion={4} maxCells={24} />
          <div className="mt-3 border-t border-dashed border-border/60 pt-3">
            <LatencyGrid points={cardHistory} ipVersion={6} maxCells={24} />
          </div>
        </div>

        {/* 延迟图表 + 延迟统计表（NodeGet Ping/TCP Ping：折线图 + 目标统计表格） */}
        <div className="card-soft p-5">
          {historyLoading && networkChartData.timestamps.length === 0 ? (
            <div
              style={{ height: 360 }}
              className="flex items-center justify-center"
            >
              <div className="flex flex-col items-center gap-2">
                <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                <span className="text-xs text-muted-foreground">加载中...</span>
              </div>
            </div>
          ) : (
            <NetworkQualityChart
              timestamps={networkChartData.timestamps}
              series={networkChartData.series}
              height={360}
              timeRange={isRealtimeRange(timeRange) ? '实时数据' : undefined}
            />
          )}
          <PingStatsTable stats={pingStats} />
        </div>

        {/* 延迟质量分桶条形图（自包含虚线容器） */}
        <LatencyQualityBar points={latencyQualityPoints} />

        {/* 在线状态时间线（自包含虚线容器） */}
        <OnlineTimeline points={onlineTimelinePoints} />

        {/* 网络与负载（NodeGet Network & Load：双列 KV） */}
        <div className="card-soft p-5">
          <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">网络与负载</h3>
          <div className="grid gap-x-10 sm:grid-cols-2">
            <div>
              <InfoRow
                label="累计下行"
                value={formatBytes(displayServer.total_rx || 0)}
              />
              <InfoRow
                label="累计上行"
                value={formatBytes(displayServer.total_tx || 0)}
              />
              <InfoRow
                label="下行速率"
                value={displayServer.net_rx != null ? formatSpeed(displayServer.net_rx) : '---'}
              />
              <InfoRow
                label="上行速率"
                value={displayServer.net_tx != null ? formatSpeed(displayServer.net_tx) : '---'}
              />
              <InfoRow
                label="平均丢包"
                value={formatLoss(avgLoss)}
                valueClassName={getLossColor(avgLoss)}
              />
            </div>
            <div>
              <InfoRow
                label="运行时间"
                value={displayServer.uptime != null ? formatUptime(displayServer.uptime) : '---'}
              />
              <InfoRow
                label="进程数"
                value={displayServer.process_count != null ? String(displayServer.process_count || 0) : '---'}
              />
              <InfoRow
                label="TCP / UDP 连接"
                value={
                  displayServer.tcp_connections != null
                    ? `${displayServer.tcp_connections || 0} / ${displayServer.udp_connections || 0}`
                    : '---'
                }
              />
              <InfoRow
                label="负载 (1/5/15分)"
                value={
                  displayServer.load_1 != null
                    ? `${(displayServer.load_1 || 0).toFixed(2)} / ${(displayServer.load_5 || 0).toFixed(2)} / ${(displayServer.load_15 || 0).toFixed(2)}`
                    : '---'
                }
              />
              <InfoRow
                label="数据更新"
                value={formatRelativeTime(displayServer.last_seen || 0)}
              />
            </div>
          </div>
        </div>

        {/* 进程列表（表格样式） */}
        {displayServer.processes && displayServer.processes.length > 0 && (
          <div className="card-soft overflow-hidden">
            <div className="p-5 pb-3">
              <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">
                进程 (Top {displayServer.processes.length})
              </h3>
            </div>
            <div className="overflow-x-auto scrollbar-thin">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-y border-border">
                    <th className="px-5 py-2 text-left text-xs font-medium text-muted-foreground">PID</th>
                    <th className="px-5 py-2 text-left text-xs font-medium text-muted-foreground">名称</th>
                    <th className="px-5 py-2 text-right text-xs font-medium text-muted-foreground">CPU%</th>
                    <th className="px-5 py-2 text-right text-xs font-medium text-muted-foreground">内存%</th>
                    <th className="px-5 py-2 text-right text-xs font-medium text-muted-foreground">RSS</th>
                  </tr>
                </thead>
                <tbody>
                  {displayServer.processes.map((proc, i) => (
                    <tr
                      key={proc.pid}
                      className={`border-b border-border/50 ${i % 2 === 0 ? '' : 'bg-secondary/10'}`}
                    >
                      <td className="px-5 py-2 tabular-nums text-muted-foreground">{proc.pid}</td>
                      <td className="max-w-[200px] truncate px-5 py-2 text-foreground">{proc.name}</td>
                      <td className="px-5 py-2 text-right font-bold tabular-nums">{proc.cpu.toFixed(1)}%</td>
                      <td className="px-5 py-2 text-right font-bold tabular-nums">{proc.memory.toFixed(1)}%</td>
                      <td className="px-5 py-2 text-right font-bold tabular-nums">{formatBytes(proc.rss)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ============================================================
//  子组件
// ============================================================

/** KV 信息行（NodeGet 风格：label 左 + value 右，font-bold tabular-nums） */
function InfoRow({
  label,
  value,
  valueClassName,
}: {
  label: string
  value: string
  valueClassName?: string
}) {
  return (
    <div className="flex justify-between gap-3 text-sm py-1">
      <span className="text-muted-foreground">{label}</span>
      <span className={`min-w-0 truncate text-right font-bold tabular-nums ${valueClassName || 'text-foreground'}`}>
        {value}
      </span>
    </div>
  )
}

/** 趋势图卡片（Sparkline + 标签 + 当前值，rounded-md border bg-card/50） */
function TrendCard({
  label,
  value,
  subValue,
  peak,
  color,
  children,
}: {
  label: string
  value: string
  subValue?: string
  /** 所选范围内的峰值（小时聚合层为真实极值） */
  peak?: string
  color: string
  children?: ReactNode
}) {
  return (
    <div className="rounded-md border bg-card/50 p-3">
      <div className="mb-1 flex items-center justify-between">
        <span className="text-[10px] font-semibold text-muted-foreground">{label}</span>
        <span className="flex items-center gap-1.5">
          {peak && (
            <span className="text-[10px] text-muted-foreground/70">峰值 {peak}</span>
          )}
          <span
            className="h-1.5 w-1.5 rounded-full"
            style={{ backgroundColor: color }}
          />
        </span>
      </div>
      <div className="mb-1">
        <span className="text-sm font-bold text-foreground tabular-nums">{value}</span>
        {subValue && (
          <span className="ml-1 text-[10px] text-muted-foreground">{subValue}</span>
        )}
      </div>
      {children}
    </div>
  )
}
