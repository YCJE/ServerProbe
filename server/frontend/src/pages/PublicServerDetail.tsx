import { useEffect, useMemo, useState, useCallback, useRef } from 'react'
import type { ReactNode } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { getPublicServers, getPublicServerHistory } from '@/lib/api'
import type {
  ServerData,
  DashboardItem,
  PingResult,
  TimeRange,
  HistoryData,
} from '@/types'
import NetworkQualityChart, { type ChartSeries } from '@/components/NetworkQualityChart'
import Sparkline from '@/components/Sparkline'
import {
  formatBytes,
  formatSpeed,
  formatUptime,
  formatLoss,
  getUsageTextColor,
  getLossColor,
} from '@/lib/utils'

/** 扩展类型：访问可能由后端附加但尚未在 ServerData 中声明的字段 */
type ServerDataExt = ServerData & {
  cpu_model?: string
  cpu_cores?: number
  monthly_fee?: number
  expires_at?: string
  country_code?: string
}

/** 时间范围选项（仅 1h / 6h / 24h） */
const TIME_RANGES: { value: TimeRange; label: string }[] = [
  { value: '1h', label: '1小时' },
  { value: '6h', label: '6小时' },
  { value: '1d', label: '24小时' },
]

/** ping 目标线条颜色池（Apple 强调色） */
const PING_COLORS = ['#5AC8FA', '#34C759', '#FF9500', '#AF52DE', '#FF2D55', '#FFCC00']

/** Sparkline 配色 */
const SPARK_CPU = '#007AFF'
const SPARK_MEM = '#34C759'
const SPARK_RX = '#5AC8FA'
const SPARK_TX = '#AF52DE'

/** Sparkline 最多展示的数据点数 */
const MAX_SPARK_POINTS = 60

/** 历史数据定时刷新间隔 */
const HISTORY_REFRESH_INTERVAL = 5 * 60 * 1000

/** 解析 ping_data（兼容字符串与数组两种格式） */
function parsePingData(raw: unknown): PingResult[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw as PingResult[]
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
}

/** 将国家代码转为 emoji 国旗 */
function countryToFlag(code: string): string {
  if (!code || code.length !== 2) return ''
  const cc = code.toUpperCase()
  if (!/^[A-Z]{2}$/.test(cc)) return ''
  return String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65))
}

// ============================================================
//  主组件
// ============================================================

/** 公开服务器详情页（三栏布局：左侧边栏 + 右侧主内容） */
export default function PublicServerDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const serverId = parseInt(id || '0', 10)

  const dashboardData = useServerStore((s) => s.dashboardData)
  const servers = useServerStore((s) => s.servers)
  const realtimeHistory = useServerStore((s) => s.realtimeHistory)
  const clearRealtimeHistory = useServerStore((s) => s.clearRealtimeHistory)

  const [loading, setLoading] = useState(true)
  const [fetchError, setFetchError] = useState('')
  const [timeRange, setTimeRange] = useState<TimeRange>('1h')
  const [historyData, setHistoryData] = useState<HistoryData | null>(null)
  const [historyLoading, setHistoryLoading] = useState(false)

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
  }, [servers.length])

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
        total_rx: liveData.total_rx || 0,
        total_tx: liveData.total_tx || 0,
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
      const requestId = ++historyRequestIdRef.current
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

  // 定时刷新历史数据
  useEffect(() => {
    const interval = setInterval(() => {
      loadHistory(timeRange)
    }, HISTORY_REFRESH_INTERVAL)
    return () => clearInterval(interval)
  }, [timeRange, loadHistory])

  // 从历史数据中提取网络质量图表数据
  const networkChartData = useMemo<{
    timestamps: number[]
    series: ChartSeries[]
  }>(() => {
    if (!historyData || !historyData.points || historyData.points.length === 0) {
      return { timestamps: [], series: [] }
    }

    const timestamps = historyData.points.map((p) => p.timestamp)
    // 一次性解析所有点的 ping_data，避免重复调用 parsePingData
    const allPings = historyData.points.map((p) => parsePingData(p.ping_data))

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
      // 取最新一个有效数据点的丢包率
      let latestLoss: number | undefined
      for (let j = allPings.length - 1; j >= 0; j--) {
        const ping = allPings[j].find((pp) => pp.name === name)
        if (ping && ping.loss >= 0) {
          latestLoss = ping.loss
          break
        }
      }
      return {
        name,
        color: PING_COLORS[i % PING_COLORS.length],
        data: allPings.map((pings) => {
          const ping = pings.find((pp) => pp.name === name)
          return ping ? ping.avg_latency : null
        }),
        loss: latestLoss,
      }
    })

    return { timestamps, series }
  }, [historyData])

  // 从 realtimeHistory 中提取 Sparkline 数据（取最近 N 个点）
  const sparklineData = useMemo(() => {
    const recent = realtimeHistory.slice(-MAX_SPARK_POINTS)
    return {
      cpu: recent.map((p) => p.cpu),
      mem: recent.map((p) => p.mem),
      netRx: recent.map((p) => p.net_rx),
      netTx: recent.map((p) => p.net_tx),
    }
  }, [realtimeHistory])

  // 平均丢包率
  const pingData = displayServer?.ping_data
  const avgLoss = useMemo(() => {
    if (!pingData || pingData.length === 0) return 0
    return pingData.reduce((sum, p) => sum + (p.loss || 0), 0) / pingData.length
  }, [pingData])

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
        <button
          onClick={() => navigate('/')}
          className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
        >
          返回首页
        </button>
      </div>
    )
  }

  // ==================== 派生数据 ====================

  const ext = displayServer as ServerDataExt
  const memUsagePercent =
    displayServer.mem_total > 0
      ? (displayServer.mem_used / displayServer.mem_total) * 100
      : displayServer.mem || 0
  const diskTotal =
    displayServer.disks?.reduce((sum, d) => sum + d.total, 0) || 0
  const hasPrice = ext.monthly_fee != null || ext.expires_at != null
  const flag = ext.country_code ? countryToFlag(ext.country_code) : ''

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
        <div className="rounded-2xl border border-border bg-card p-4">
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
          </div>
        </div>

        {/* 价格信息卡片（仅有数据时显示） */}
        {hasPrice && (
          <div className="rounded-2xl border border-border bg-card p-4">
            <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              价格信息
            </h3>
            <div className="space-y-2.5">
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
        <div className="rounded-2xl border border-border bg-card p-4">
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            硬件信息
          </h3>
          <div className="space-y-2.5">
            <InfoRow label="CPU 型号" value={ext.cpu_model || '-'} />
            <InfoRow
              label="核心数"
              value={ext.cpu_cores != null ? `${ext.cpu_cores} 核` : '-'}
            />
            <InfoRow label="内存" value={formatBytes(displayServer.mem_total)} />
            <InfoRow label="硬盘" value={diskTotal > 0 ? formatBytes(diskTotal) : '-'} />
            <InfoRow label="系统" value={displayServer.os || '-'} />
            <InfoRow label="架构" value={displayServer.arch || '-'} />
            <InfoRow label="Agent 版本" value={displayServer.agent_version || '-'} />
          </div>
        </div>

        {/* 系统信息卡片 */}
        <div className="rounded-2xl border border-border bg-card p-4">
          <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            系统信息
          </h3>
          <div className="space-y-2.5">
            <InfoRow
              label="运行时间"
              value={
                displayServer.online ? formatUptime(displayServer.uptime) : '---'
              }
            />
            <InfoRow
              label="负载 (1/5/15分)"
              value={
                displayServer.online
                  ? `${(displayServer.load_1 || 0).toFixed(2)} / ${(displayServer.load_5 || 0).toFixed(2)} / ${(displayServer.load_15 || 0).toFixed(2)}`
                  : '---'
              }
            />
          </div>
        </div>
      </aside>

      {/* ============ 右侧主内容区 ============ */}
      <div className="min-w-0 flex-1 space-y-4">
        {/* 标题行 + 时间范围选择器 */}
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="text-lg font-semibold text-foreground">网络质量</h2>
          <div className="flex items-center gap-1 rounded-full border border-border bg-card p-1">
            {TIME_RANGES.map((range) => (
              <button
                key={range.value}
                onClick={() => setTimeRange(range.value)}
                className={`rounded-full px-3 py-1 text-xs font-medium transition-colors ${
                  timeRange === range.value
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                {range.label}
              </button>
            ))}
          </div>
        </div>

        {/* 网络质量图表 */}
        <div className="rounded-2xl border border-border bg-card p-4">
          {historyLoading && networkChartData.timestamps.length === 0 ? (
            <div
              style={{ height: 300 }}
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
              height={300}
            />
          )}
        </div>

        {/* 状态标签行 */}
        <div className="flex flex-wrap gap-2">
          <StatusBadge
            label="CPU"
            value={`${(displayServer.cpu || 0).toFixed(1)}%`}
            colorClass={getUsageTextColor(displayServer.cpu || 0)}
          />
          <StatusBadge
            label="内存"
            value={`${memUsagePercent.toFixed(1)}%`}
            colorClass={getUsageTextColor(memUsagePercent)}
          />
          <StatusBadge
            label="磁盘"
            value={`${(displayServer.disk_usage || 0).toFixed(1)}%`}
            colorClass={getUsageTextColor(displayServer.disk_usage || 0)}
          />
          <StatusBadge
            label="丢包率"
            value={formatLoss(avgLoss)}
            colorClass={getLossColor(avgLoss)}
          />
        </div>

        {/* 资源监控卡片网格 */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <ResourceCard
            label="CPU 使用率"
            value={`${(displayServer.cpu || 0).toFixed(1)}%`}
            color={SPARK_CPU}
          >
            <Sparkline data={sparklineData.cpu} color={SPARK_CPU} height={40} />
          </ResourceCard>

          <ResourceCard
            label="内存使用率"
            value={`${memUsagePercent.toFixed(1)}%`}
            subValue={`${formatBytes(displayServer.mem_used)} / ${formatBytes(displayServer.mem_total)}`}
            color={SPARK_MEM}
          >
            <Sparkline data={sparklineData.mem} color={SPARK_MEM} height={40} />
          </ResourceCard>

          <ResourceCard
            label="网络下行"
            value={displayServer.online ? formatSpeed(displayServer.net_rx) : '---'}
            color={SPARK_RX}
          >
            <Sparkline data={sparklineData.netRx} color={SPARK_RX} height={40} />
          </ResourceCard>

          <ResourceCard
            label="网络上行"
            value={displayServer.online ? formatSpeed(displayServer.net_tx) : '---'}
            color={SPARK_TX}
          >
            <Sparkline data={sparklineData.netTx} color={SPARK_TX} height={40} />
          </ResourceCard>
        </div>
      </div>
    </div>
  )
}

// ============================================================
//  子组件
// ============================================================

/** 信息行（label + value 左右对齐） */
function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-2 text-sm">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-medium text-foreground">
        {value}
      </span>
    </div>
  )
}

/** 状态药丸标签 */
function StatusBadge({
  label,
  value,
  colorClass,
}: {
  label: string
  value: string
  colorClass: string
}) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-3 py-1.5 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className={`font-semibold ${colorClass}`}>{value}</span>
    </span>
  )
}

/** 资源监控卡片（当前值 + Sparkline） */
function ResourceCard({
  label,
  value,
  subValue,
  color,
  children,
}: {
  label: string
  value: string
  subValue?: string
  color: string
  children?: ReactNode
}) {
  return (
    <div className="rounded-2xl border border-border bg-card p-4">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span
          className="h-2 w-2 rounded-full"
          style={{ backgroundColor: color }}
        />
      </div>
      <div className="mb-2">
        <span className="text-xl font-bold text-foreground">{value}</span>
        {subValue && (
          <span className="ml-1.5 text-xs text-muted-foreground">{subValue}</span>
        )}
      </div>
      {children}
    </div>
  )
}
