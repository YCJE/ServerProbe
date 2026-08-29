import { useEffect, useMemo, useState, useCallback, useRef } from 'react'
import type { ReactNode } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { useSiteSettings } from '@/store/useSiteSettingsStore'
import { getPublicServers, getPublicServerHistory } from '@/lib/api'
import type {
  ServerData,
  DashboardItem,
  TimeRange,
  HistoryData,
} from '@/types'
import NetworkQualityChart, { type ChartSeries } from '@/components/NetworkQualityChart'
import Sparkline from '@/components/Sparkline'
import LatencyGrid, { type LatencyGridPoint } from '@/components/LatencyGrid'
import LatencyQualityBar, { type LatencyPoint } from '@/components/LatencyQualityBar'
import OnlineTimeline, { type OnlineTimelinePoint } from '@/components/OnlineTimeline'
import ResourceRing from '@/components/ResourceRing'
import DistroIcon from '@/components/DistroIcon'
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
  getFlagEmoji,
} from '@/lib/utils'
import { usePageTitle } from '@/hooks/usePageTitle'

/** 扩展类型：访问可能由后端附加但尚未在 ServerData 中声明的字段 */
type ServerDataExt = ServerData & {
  monthly_fee?: number
  expires_at?: string
  country_code?: string
}

/** 时间范围选项（含实时模式；≥7d 走小时聚合层） */
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

/** ping 目标线条颜色池 */
const PING_COLORS = ['#5AC8FA', '#34C759', '#FF9500', '#AF52DE', '#FF2D55', '#FFCC00']

/** Sparkline 配色 */
const SPARK_CPU = '#007AFF'
const SPARK_MEM = '#34C759'
const SPARK_RX = '#5AC8FA'
const SPARK_TX = '#AF52DE'

/** Sparkline 最多展示的数据点数 */
const MAX_SPARK_POINTS = 60

/** 稳定的空数组引用（延迟格子图缺省值，避免每次渲染触发 useMemo 重算） */
const EMPTY_GRID_HISTORY: LatencyGridPoint[] = []

/** 历史数据定时刷新间隔 */
const HISTORY_REFRESH_INTERVAL = 5 * 60 * 1000

// ============================================================
//  主组件
// ============================================================

/** 公开服务器详情页（NodeGet 风格，三栏布局：左侧边栏 + 右侧主内容） */
export default function PublicServerDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const serverId = parseInt(id || '0', 10)

  const dashboardData = useServerStore((s) => s.dashboardData)
  const servers = useServerStore((s) => s.servers)
  const realtimeHistory = useServerStore((s) => s.realtimeHistory)
  const clearRealtimeHistory = useServerStore((s) => s.clearRealtimeHistory)
  // 延迟格子图数据源：公开 WS 填充的滚动窗口（固定最近 60 分钟，不随时间范围切换）
  const gridHistory = useServerStore((s) => s.cardHistory.get(serverId) ?? EMPTY_GRID_HISTORY)
  // 默认历史范围（后台"站点设置"可配置）
  const { defaultHistoryRange } = useSiteSettings()

  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState('')
  const [retryCount, setRetryCount] = useState(0)
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

  // 首次加载：获取公开服务器列表
  useEffect(() => {
    if (servers.length === 0) {
      setLoading(true)
      setFetchError('')
      getPublicServers()
        .then((res) => {
          if (!mountedRef.current) return
          if (res.servers.length > 0) {
            const dashboardItems: DashboardItem[] = res.servers.map((s) => ({
              agent_id: s.id,
              hostname: s.hostname,
              display_name: s.display_name,
              os: s.os || '',
              arch: s.arch || '',
              agent_version: s.agent_version || '',
              online: s.online,
              cpu: s.cpu,
              cpu_model: s.cpu_model || '',
              cpu_cores: s.cpu_cores || 0,
              mem: s.mem,
              mem_total: s.mem_total,
              mem_used: s.mem_used,
              swap_total: s.swap_total || 0,
              swap_used: s.swap_used || 0,
              net_rx: s.net_rx,
              net_tx: s.net_tx,
              total_rx: s.total_rx || 0,
              total_tx: s.total_tx || 0,
              load_1: s.load_1 || 0,
              load_5: s.load_5 || 0,
              load_15: s.load_15 || 0,
              uptime: s.uptime,
              disk_usage: s.disk_usage || 0,
              disks: [],
              // 公开 API 已过滤连接/进程信息（安全考虑），公开页不再展示，置 0 占位
              tcp_connections: 0,
              udp_connections: 0,
              process_count: 0,
              ping_data: [],
              timestamp: Math.floor(Date.now() / 1000),
            }))
            // 先设置 currentServer，再处理 WS 数据，确保首条数据能进入 realtimeHistory
            const targetItem = dashboardItems.find((item) => item.agent_id === serverId)
            if (targetItem) {
              useServerStore.setState({
                currentServer: {
                  id: targetItem.agent_id,
                  hostname: targetItem.hostname || `Agent-${targetItem.agent_id}`,
                  display_name: targetItem.display_name || '',
                  os: targetItem.os || '',
                  arch: targetItem.arch || '',
                  agent_version: targetItem.agent_version || '',
                  online: targetItem.online,
                  last_seen: targetItem.timestamp,
                  cpu: targetItem.cpu,
                  cpu_model: targetItem.cpu_model || '',
                  cpu_cores: targetItem.cpu_cores || 0,
                  mem: targetItem.mem,
                  mem_total: targetItem.mem_total,
                  mem_used: targetItem.mem_used,
                  swap_total: targetItem.swap_total || 0,
                  swap_used: targetItem.swap_used || 0,
                  net_rx: targetItem.net_rx,
                  net_tx: targetItem.net_tx,
                  total_rx: targetItem.total_rx || 0,
                  total_tx: targetItem.total_tx || 0,
                  uptime: targetItem.uptime,
                  load_1: targetItem.load_1 || 0,
                  load_5: targetItem.load_5 || 0,
                  load_15: targetItem.load_15 || 0,
                  disk_usage: targetItem.disk_usage || 0,
                  disks: targetItem.disks || [],
                  tcp_connections: targetItem.tcp_connections || 0,
                  udp_connections: targetItem.udp_connections || 0,
                  process_count: targetItem.process_count || 0,
                  ping_data: targetItem.ping_data || [],
                },
              })
            }
            useServerStore.getState().handleDashboardMessage(dashboardItems)
            setLoading(false)
          } else {
            setLoading(false)
          }
        })
        .catch(() => {
          if (mountedRef.current) {
            setFetchError('加载服务器数据失败，请稍后重试')
            setLoading(false)
          }
        })
    } else {
      setLoading(false)
    }
  }, [servers.length, retryCount])

  const baseServer = useMemo(
    () => servers.find((s) => s.id === serverId) || null,
    [servers, serverId],
  )
  const liveData = dashboardData.get(serverId)

  // 合并基础信息与实时数据
  const displayServer = useMemo<ServerData | null>(() => {
    if (!baseServer && !liveData) return null
    if (baseServer && liveData) {
      return {
        ...baseServer,
        online: liveData.online,
        cpu: liveData.cpu,
        cpu_model: liveData.cpu_model || baseServer?.cpu_model || '',
        cpu_cores: liveData.cpu_cores || baseServer?.cpu_cores || 0,
        mem: liveData.mem,
        mem_total: liveData.mem_total,
        mem_used: liveData.mem_used,
        swap_total: liveData.swap_total || 0,
        swap_used: liveData.swap_used || 0,
        net_rx: liveData.net_rx,
        net_tx: liveData.net_tx,
        total_rx: liveData.total_rx ?? baseServer.total_rx ?? 0,
        total_tx: liveData.total_tx ?? baseServer.total_tx ?? 0,
        uptime: liveData.uptime,
        load_1: liveData.load_1 || 0,
        load_5: liveData.load_5 || 0,
        load_15: liveData.load_15 || 0,
        disk_usage: liveData.disk_usage ?? baseServer.disk_usage ?? 0,
        disks: liveData.disks || [],
        tcp_connections: liveData.tcp_connections || 0,
        udp_connections: liveData.udp_connections || 0,
        process_count: liveData.process_count || 0,
        ping_data: liveData.ping_data || [],
        last_seen: liveData.timestamp,
        // 新增字段
        virtualization: liveData.virtualization || baseServer.virtualization,
        distro: liveData.distro || baseServer.distro,
        processes: liveData.processes || baseServer.processes,
        time_offset: liveData.time_offset ?? baseServer.time_offset,
      }
    }
    if (liveData) {
      return {
        id: liveData.agent_id,
        hostname: liveData.hostname || `Agent-${liveData.agent_id}`,
        display_name: liveData.display_name || '',
        os: liveData.os || baseServer?.os || '',
        arch: liveData.arch || baseServer?.arch || '',
        agent_version: liveData.agent_version || baseServer?.agent_version || '',
        online: liveData.online,
        last_seen: liveData.timestamp,
        cpu: liveData.cpu,
        cpu_model: liveData.cpu_model || baseServer?.cpu_model || '',
        cpu_cores: liveData.cpu_cores || baseServer?.cpu_cores || 0,
        mem: liveData.mem,
        mem_total: liveData.mem_total,
        mem_used: liveData.mem_used,
        swap_total: liveData.swap_total || 0,
        swap_used: liveData.swap_used || 0,
        net_rx: liveData.net_rx,
        net_tx: liveData.net_tx,
        total_rx: liveData.total_rx || 0,
        total_tx: liveData.total_tx || 0,
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
        virtualization: liveData.virtualization || baseServer?.virtualization,
        distro: liveData.distro || baseServer?.distro,
        processes: liveData.processes || baseServer?.processes,
        time_offset: liveData.time_offset ?? baseServer?.time_offset,
      }
    }
    return baseServer
  }, [baseServer, liveData])

  // 页面标题：服务器名（路由级 document.title）
  usePageTitle(displayServer ? displayServer.display_name || displayServer.hostname : '服务器详情')

  // 切换服务器时：清除历史数据 & 重置 currentServer
  useEffect(() => {
    clearRealtimeHistory()
    setHistoryData(null)
    useServerStore.setState({ currentServer: null })

    return () => {
      // 卸载或切换时：如果 currentServer 仍指向当前服务器则清除
      if (useServerStore.getState().currentServer?.id === serverId) {
        useServerStore.setState({ currentServer: null })
      }
    }
  }, [serverId, clearRealtimeHistory])

  // 设置 currentServer，使 handleDashboardMessage 自动填充 realtimeHistory
  useEffect(() => {
    if (displayServer && useServerStore.getState().currentServer?.id !== serverId) {
      useServerStore.setState({ currentServer: displayServer })
    }
  }, [displayServer, serverId])

  // 加载历史数据
  const loadHistory = useCallback(
    async (range: TimeRange) => {
      if (serverId <= 0) return
      const requestId = ++historyRequestIdRef.current
      // 实时模式：不请求历史 API，使用 WebSocket 推送的 realtimeHistory
      if (range === 'realtime') {
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
        const data = await getPublicServerHistory(serverId, range)
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
    },
    [serverId],
  )

  useEffect(() => {
    loadHistory(timeRange)
  }, [timeRange, loadHistory])

  // 定时刷新历史数据（标签页隐藏时暂停以节省带宽，实时模式跳过）
  useEffect(() => {
    if (timeRange === 'realtime') return // 实时模式使用 WebSocket，不需要定时刷新

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

  // 统一提取 ping 数据源（实时/历史），网络质量图表与延迟统计表共用，避免重复 parsePingData
  const pingSource = useMemo<{
    timestamps: number[]
    allPings: ReturnType<typeof parsePingData>[]
  }>(() => {
    // 实时模式：从 realtimeHistory 提取
    if (timeRange === 'realtime') {
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
  }, [timeRange, timeRange === 'realtime' ? realtimeHistory : historyData])

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

  // Sparkline 数据：实时模式取 realtimeHistory，历史模式取 historyData（与 ServerDetail 对齐）
  const sparklineData = useMemo(() => {
    const maxOf = (arr: number[]) => (arr.length ? Math.max(...arr) : 0)
    if (timeRange === 'realtime') {
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
  }, [timeRange, timeRange === 'realtime' ? realtimeHistory : historyData])

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

  // 延迟质量条形图数据：实时模式用 realtimeHistory，历史模式用 historyData + 补充最新实时点
  const latencyQualityPoints = useMemo<LatencyPoint[]>(() => {
    // 实时模式：使用 realtimeHistory
    if (timeRange === 'realtime') {
      return realtimeHistory.map((p) => ({
        timestamp: p.timestamp,
        ping_data: p.ping_data,
      }))
    }
    // 历史模式：优先使用 historyData，再补充比历史数据更新的实时点
    if (historyData && historyData.points && historyData.points.length > 0) {
      const historyPoints = historyData.points.map((p) => ({
        timestamp: p.timestamp,
        ping_data: p.ping_data,
      }))
      // 补充比历史数据最后一个点更新的实时数据
      const lastHistoryTs = historyPoints[historyPoints.length - 1].timestamp
      const newerRealtime = realtimeHistory
        .filter((p) => p.timestamp > lastHistoryTs)
        .map((p) => ({
          timestamp: p.timestamp,
          ping_data: p.ping_data,
        }))
      return [...historyPoints, ...newerRealtime]
    }
    // 回退到实时数据
    return realtimeHistory.map((p) => ({
      timestamp: p.timestamp,
      ping_data: p.ping_data,
    }))
  }, [timeRange, realtimeHistory, historyData])

  // 在线状态时间线数据：从历史数据点提取在线状态
  const onlineTimelinePoints = useMemo<OnlineTimelinePoint[]>(() => {
    const result: OnlineTimelinePoint[] = []
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
    if (realtimeHistory.length > 0) {
      for (const p of realtimeHistory) {
        result.push({
          timestamp: p.timestamp,
          online: p.online,
        })
      }
    }
    return result
  }, [historyData, realtimeHistory])

  // ==================== 加载 / 错误状态 ====================

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
        <p className="text-sm text-muted-foreground">
          {fetchError || '服务器不存在或未上线'}
        </p>
        <div className="mt-3 flex gap-2">
          {fetchError && (
            <button
              onClick={() => setRetryCount((c) => c + 1)}
              className="rounded-lg border border-border bg-card px-4 py-2 text-sm text-foreground hover:bg-accent"
            >
              重试
            </button>
          )}
          <button
            onClick={() => navigate('/')}
            className="rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
          >
            返回首页
          </button>
        </div>
      </div>
    )
  }

  // ==================== 派生数据 ====================

  const ext = displayServer as ServerDataExt
  const memUsagePercent =
    displayServer.mem_total > 0
      ? ((displayServer.mem_used || 0) / displayServer.mem_total) * 100
      : displayServer.mem || 0
  const diskTotal =
    displayServer.disks?.reduce((sum, d) => sum + d.total, 0) || 0
  const diskUsed =
    displayServer.disks?.reduce((sum, d) => sum + d.used, 0) || 0
  const hasPrice = ext.monthly_fee != null || ext.expires_at != null
  const flag = ext.country_code ? getFlagEmoji(ext.country_code) : ''
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
          onClick={() => navigate('/')}
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
          返回列表
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
            {flag && <span className="shrink-0 text-lg">{flag}</span>}
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
        </div>

        {/* 价格信息卡片（仅有数据时显示） */}
        {hasPrice && (
          <div className="card-soft p-5">
            <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">
              价格信息
            </h3>
            <div>
              {ext.monthly_fee != null && (
                <InfoRow label="月费" value={`¥${ext.monthly_fee}`} />
              )}
              {ext.expires_at && (
                <InfoRow label="到期时间" value={ext.expires_at} />
              )}
            </div>
          </div>
        )}

        {/* 硬件信息卡片 */}
        <div className="card-soft p-5">
          <h3 className="text-xs uppercase tracking-wide text-muted-foreground mb-3">
            硬件信息
          </h3>
          <div>
            <InfoRow label="CPU 型号" value={ext.cpu_model || '-'} />
            <InfoRow
              label="核心数"
              value={ext.cpu_cores != null ? `${ext.cpu_cores} 核` : '-'}
            />
            <InfoRow label="内存" value={formatBytes(displayServer.mem_total)} />
            <InfoRow label="硬盘" value={diskTotal > 0 ? formatBytes(diskTotal) : '-'} />
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
          <LatencyGrid points={gridHistory} ipVersion={4} maxCells={24} />
          <div className="mt-3 border-t border-dashed border-border/60 pt-3">
            <LatencyGrid points={gridHistory} ipVersion={6} maxCells={24} />
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
              timeRange={timeRange === 'realtime' ? '实时数据' : undefined}
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
                label="负载 (1/5/15分)"
                value={
                  displayServer.load_1 != null
                    ? `${(displayServer.load_1 || 0).toFixed(2)} / ${(displayServer.load_5 || 0).toFixed(2)} / ${(displayServer.load_15 || 0).toFixed(2)}`
                    : '---'
                }
              />
              <InfoRow
                label="Swap"
                value={
                  displayServer.swap_total != null
                    ? displayServer.swap_total > 0
                      ? `${formatBytes(displayServer.swap_used)} / ${formatBytes(displayServer.swap_total)}`
                      : '未启用'
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
