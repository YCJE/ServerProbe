import { memo, useMemo, useState, useEffect, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { useTagColors } from '@/store/useTagStore'
import ServerCard from '@/components/ServerCard'
import DistroIcon from '@/components/DistroIcon'
import StatusDot from '@/components/StatusDot'
import MapView from '@/components/MapView'
import EmptyState from '@/components/EmptyState'
import { getLatencyTextColor } from '@/components/LatencyGrid'
import {
  formatSpeed,
  formatUptime,
  formatTraffic,
  getCountryCode,
  getFlagEmoji,
  parseTags,
  getTagStyle,
} from '@/lib/utils'
import type { ServerData } from '@/types'
import { usePageTitle } from '@/hooks/usePageTitle'

/** 视图模式 */
type ViewMode = 'card' | 'table' | 'map'

/** IP 栈筛选 */
type IPFilter = '' | 'v4' | 'v6' | 'dual'

/** IP 栈筛选选项 */
const IP_FILTER_OPTIONS: { value: IPFilter; label: string }[] = [
  { value: '', label: '全部 IP 栈' },
  { value: 'v4', label: '有 IPv4' },
  { value: 'v6', label: '有 IPv6' },
  { value: 'dual', label: '双栈' },
]

/** 排序选项 */
type SortOption =
  | 'default'
  | 'name'
  | 'cpu'
  | 'mem'
  | 'disk'
  | 'net'
  | 'uptime'
  | 'traffic'
  | 'latency'
  | 'expire'

/** 排序选项列表 */
const SORT_OPTIONS: { value: SortOption; label: string }[] = [
  { value: 'default', label: '默认' },
  { value: 'name', label: '名称' },
  { value: 'cpu', label: 'CPU' },
  { value: 'mem', label: '内存' },
  { value: 'disk', label: '磁盘' },
  { value: 'net', label: '网速' },
  { value: 'uptime', label: '运行时长' },
  { value: 'traffic', label: '月流量' },
  { value: 'latency', label: '平均延迟' },
  { value: 'expire', label: '到期时间' },
]

/** localStorage 键名 */
const LS_VIEW_MODE = 'probe_dashboard_view'
const LS_SORT = 'probe_dashboard_sort'

/** 安全读取 localStorage */
function loadLS<T extends string>(key: string, validValues: T[], defaultValue: T): T {
  try {
    const v = localStorage.getItem(key)
    if (v && validValues.includes(v as T)) return v as T
  } catch {
    // localStorage 不可用
  }
  return defaultValue
}

/** 获取内存使用率 */
function getMemUsage(server: ServerData): number {
  return server.mem_total > 0
    ? ((server.mem_used || 0) / server.mem_total) * 100
    : server.mem || 0
}

/** 获取总网速（下行+上行） */
function getTotalNetSpeed(server: ServerData): number {
  return (server.net_rx || 0) + (server.net_tx || 0)
}

/** 获取月流量使用量（当月累计上下行） */
function getMonthlyTraffic(server: ServerData): number {
  return (server.monthly_rx || 0) + (server.monthly_tx || 0)
}

/** 获取平均延迟（无有效数据返回 -1，排序时排在最后） */
function getAvgLatency(server: ServerData): number {
  const pings = server.ping_data || []
  const valid = pings.filter((p) => p.avg_latency != null && p.avg_latency >= 0)
  if (valid.length === 0) return -1
  return valid.reduce((sum, p) => sum + (p.avg_latency || 0), 0) / valid.length
}

/** 获取到期剩余天数（永不过期返回 Infinity，排序时排在最后） */
function getExpireDays(server: ServerData): number {
  if (server.expires_in_days == null) return Infinity
  return server.expires_in_days
}

/** 筛选下拉框（NodeGet 紧凑控件，input-base 风格） */
function FilterSelect({
  value,
  onChange,
  options,
  ariaLabel,
}: {
  value: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  ariaLabel: string
}) {
  return (
    <div className="relative">
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="input-base h-10 cursor-pointer appearance-none pr-8 font-medium"
        aria-label={ariaLabel}
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
      <svg
        className="pointer-events-none absolute right-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
      </svg>
    </div>
  )
}

/** 紧凑进度条单元格（NodeGet 资源环形图色） */
function ProgressCell({ value }: { value: number }) {
  const v = Math.min(Math.max(value, 0), 100)
  // NodeGet 资源环形图色：< 70% 绿、70-90% 橙、>= 90% 红
  const color = v >= 90 ? '#f56565' : v >= 70 ? '#f6ad55' : '#42b983'
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 w-12 shrink-0 overflow-hidden rounded-full bg-secondary">
        <div
          className="h-full rounded-full transition-all duration-500"
          style={{ width: `${v}%`, backgroundColor: color }}
        />
      </div>
      <span className="shrink-0 text-[10px] font-medium tabular-nums text-foreground/80">
        {v.toFixed(1)}%
      </span>
    </div>
  )
}

/** 表格行组件（NodeGet 风格） */
const ServerTableRow = memo(function ServerTableRow({
  server,
  basePath,
}: {
  server: ServerData
  basePath: string
}) {
  const navigate = useNavigate()
  const memUsage = getMemUsage(server)
  const diskUsage = server.disk_usage || 0
  const showVirt = server.virtualization && server.virtualization !== 'None' && server.virtualization !== 'none'
  const cc = getCountryCode(server)
  const flag = cc ? getFlagEmoji(cc) : ''
  const monthlyTraffic = getMonthlyTraffic(server)
  const avgLatency = getAvgLatency(server)
  const expireDays = server.expires_in_days
  const expireColor =
    expireDays == null ? '' : expireDays < 7 ? '#f56565' : expireDays < 30 ? '#f6ad55' : ''

  const handleClick = () => {
    navigate(`${basePath}/server/${server.id}`)
  }

  return (
    <tr
      onClick={handleClick}
      className={`cursor-pointer border-b transition-colors hover:bg-muted/50 ${
        server.online ? '' : 'opacity-60'
      }`}
    >
      {/* 状态灯 */}
      <td className="p-3 align-middle">
        <StatusDot online={server.online} />
      </td>
      {/* 名称 */}
      <td className="p-3 align-middle">
        <div className="flex items-center gap-2">
          {flag && <span className="shrink-0 text-sm leading-none">{flag}</span>}
          <span className="truncate text-sm font-medium text-foreground">
            {server.display_name || server.hostname}
          </span>
          {showVirt && (
            <span className="shrink-0 rounded bg-secondary px-1.5 py-0.5 text-[9px] font-medium text-muted-foreground">
              {server.virtualization}
            </span>
          )}
        </div>
      </td>
      {/* 位置 */}
      <td className="p-3 align-middle">
        <span className="whitespace-nowrap text-xs text-muted-foreground">
          {server.region || cc || '---'}
        </span>
      </td>
      {/* CPU */}
      <td className="p-3 align-middle">
        <ProgressCell value={server.cpu || 0} />
      </td>
      {/* 内存 */}
      <td className="p-3 align-middle">
        <ProgressCell value={memUsage} />
      </td>
      {/* 磁盘 */}
      <td className="p-3 align-middle">
        <ProgressCell value={diskUsage} />
      </td>
      {/* 下行速度 */}
      <td className="p-3 align-middle">
        <span className="text-xs font-medium tabular-nums text-foreground/80">
          {server.online ? formatSpeed(server.net_rx) : '---'}
        </span>
      </td>
      {/* 上行速度 */}
      <td className="p-3 align-middle">
        <span className="text-xs font-medium tabular-nums text-foreground/80">
          {server.online ? formatSpeed(server.net_tx) : '---'}
        </span>
      </td>
      {/* 月流量 */}
      <td className="p-3 align-middle">
        <span className="whitespace-nowrap text-xs tabular-nums text-muted-foreground">
          {formatTraffic(monthlyTraffic)}
          {server.traffic_quota_bytes ? ` / ${formatTraffic(server.traffic_quota_bytes)}` : ''}
        </span>
      </td>
      {/* 平均延迟（带颜色分级圆点） */}
      <td className="p-3 align-middle">
        <span className="flex items-center gap-1.5 whitespace-nowrap text-xs tabular-nums">
          {avgLatency >= 0 ? (
            <>
              <span
                className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
                style={{ backgroundColor: getLatencyTextColor(avgLatency) }}
              />
              <span className="font-medium" style={{ color: getLatencyTextColor(avgLatency) }}>
                {avgLatency.toFixed(1)}ms
              </span>
            </>
          ) : (
            <span className="text-muted-foreground/50">---</span>
          )}
        </span>
      </td>
      {/* 到期 */}
      <td className="p-3 align-middle">
        <span
          className="whitespace-nowrap text-xs tabular-nums text-muted-foreground"
          style={expireColor ? { color: expireColor } : undefined}
        >
          {expireDays == null
            ? '永久'
            : expireDays < 0
              ? '已过期'
              : `${expireDays}天${expireDays < 7 ? ' ⚠' : ''}`}
        </span>
      </td>
      {/* 运行时长 */}
      <td className="p-3 align-middle">
        <span className="text-xs tabular-nums text-muted-foreground">
          {server.online ? formatUptime(server.uptime) : '---'}
        </span>
      </td>
    </tr>
  )
}, (prevProps, nextProps) => {
  const s = prevProps.server
  const n = nextProps.server
  return (
    prevProps.basePath === nextProps.basePath &&
    s.id === n.id &&
    s.online === n.online &&
    s.cpu === n.cpu &&
    s.mem === n.mem &&
    s.mem_total === n.mem_total &&
    s.mem_used === n.mem_used &&
    s.net_rx === n.net_rx &&
    s.net_tx === n.net_tx &&
    s.disk_usage === n.disk_usage &&
    s.uptime === n.uptime &&
    s.display_name === n.display_name &&
    s.hostname === n.hostname &&
    s.os === n.os &&
    s.distro === n.distro &&
    s.virtualization === n.virtualization &&
    s.country_code === n.country_code &&
    s.region === n.region &&
    s.monthly_rx === n.monthly_rx &&
    s.monthly_tx === n.monthly_tx &&
    s.traffic_quota_bytes === n.traffic_quota_bytes &&
    s.expires_in_days === n.expires_in_days &&
    s.ping_data === n.ping_data
  )
})

/** 仪表盘页（服务器卡片网格 + 表格视图，NodeGet 风格） */
export default function Dashboard() {
  usePageTitle('仪表盘')
  const servers = useServerStore((s) => s.servers)
  const fetchServers = useServerStore((s) => s.fetchServers)
  const wsConnected = useServerStore((s) => s.wsConnected)
  const tagColors = useTagColors()
  const [searchParams, setSearchParams] = useSearchParams()

  // 视图模式（URL 参数优先，其次 localStorage）
  const [viewMode, setViewMode] = useState<ViewMode>(() => {
    const fromURL = searchParams.get('view')
    if (fromURL === 'card' || fromURL === 'table' || fromURL === 'map') return fromURL
    return loadLS<ViewMode>(LS_VIEW_MODE, ['card', 'table', 'map'], 'card')
  })
  // 排序选项（URL 参数优先，其次 localStorage）
  const [sortOption, setSortOption] = useState<SortOption>(() => {
    const fromURL = searchParams.get('sort')
    if (fromURL && SORT_OPTIONS.some((o) => o.value === fromURL)) return fromURL as SortOption
    return loadLS<SortOption>(LS_SORT, SORT_OPTIONS.map((o) => o.value), 'default')
  })
  // 搜索关键字（URL 参数 q）
  const [searchInput, setSearchInput] = useState(() => searchParams.get('q') || '')
  const [debouncedSearch, setDebouncedSearch] = useState(() => (searchParams.get('q') || '').trim().toLowerCase())
  // 标签筛选（''=全部）
  const [tagFilter, setTagFilter] = useState(() => searchParams.get('tag') || '')
  // 地区筛选（''=全部，值为 country_code）
  const [regionFilter, setRegionFilter] = useState(() => searchParams.get('region') || '')
  // IP 栈筛选（''=全部）
  const [ipFilter, setIpFilter] = useState<IPFilter>(() => {
    const v = searchParams.get('ip')
    if (v === 'v4' || v === 'v6' || v === 'dual') return v
    return ''
  })

  // 视图模式 / 排序选项持久化（localStorage）
  useEffect(() => {
    try {
      localStorage.setItem(LS_VIEW_MODE, viewMode)
    } catch {
      // 忽略
    }
  }, [viewMode])

  useEffect(() => {
    try {
      localStorage.setItem(LS_SORT, sortOption)
    } catch {
      // 忽略
    }
  }, [sortOption])

  // 筛选条件写入 URL 参数（刷新/分享保持筛选状态）
  useEffect(() => {
    const params = new URLSearchParams()
    if (searchInput.trim()) params.set('q', searchInput.trim())
    if (tagFilter) params.set('tag', tagFilter)
    if (regionFilter) params.set('region', regionFilter)
    if (ipFilter) params.set('ip', ipFilter)
    if (sortOption !== 'default') params.set('sort', sortOption)
    if (viewMode !== 'card') params.set('view', viewMode)
    const qs = params.toString()
    // replace 避免每次筛选都产生一条历史记录
    setSearchParams(qs ? qs : {}, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput, tagFilter, regionFilter, ipFilter, sortOption, viewMode])

  // 搜索防抖 400ms
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput.trim().toLowerCase())
    }, 400)
    return () => clearTimeout(timer)
  }, [searchInput])

  // 统计信息（含月流量与月成本合计）
  const stats = useMemo(() => {
    const total = servers.length
    const onlineServers = servers.filter((s) => s.online)
    const online = onlineServers.length
    const offline = total - online
    const avgCpu = online > 0
      ? onlineServers.reduce((sum, s) => sum + (s.cpu || 0), 0) / online
      : 0
    const avgMem = online > 0
      ? onlineServers.reduce((sum, s) => sum + (s.mem || 0), 0) / online
      : 0
    const totalRx = onlineServers.reduce((sum, s) => sum + (s.net_rx || 0), 0)
    const totalTx = onlineServers.reduce((sum, s) => sum + (s.net_tx || 0), 0)
    // 本月总流量（所有服务器当月累计上下行）
    const monthlyTraffic = servers.reduce((sum, s) => sum + getMonthlyTraffic(s), 0)
    // 月成本合计（按币种分组，yearly 折算为月）
    const costByCurrency = new Map<string, number>()
    for (const s of servers) {
      if (!s.price_amount || s.price_amount <= 0 || !s.price_currency) continue
      const monthly =
        (s.price_cycle || '').toLowerCase() === 'yearly' ? s.price_amount / 12 : s.price_amount
      costByCurrency.set(
        s.price_currency.toUpperCase(),
        (costByCurrency.get(s.price_currency.toUpperCase()) || 0) + monthly,
      )
    }
    const monthlyCost = [...costByCurrency.entries()]
      .map(([currency, amount]) => ({ currency, amount }))

    return { total, online, offline, avgCpu, avgMem, totalRx, totalTx, monthlyTraffic, monthlyCost }
  }, [servers])

  // 可选标签列表（从所有服务器收集去重，按名称排序）
  const allTags = useMemo(() => {
    const set = new Set<string>()
    for (const s of servers) {
      for (const tag of parseTags(s.tags)) set.add(tag)
    }
    return [...set].sort((a, b) => a.localeCompare(b, 'zh-CN'))
  }, [servers])

  // 可选地区列表（按 country_code 分组，含国旗，按数量降序）
  const allRegions = useMemo(() => {
    const map = new Map<string, { code: string; count: number }>()
    for (const s of servers) {
      const cc = getCountryCode(s)
      if (!cc) continue
      const existing = map.get(cc)
      if (existing) {
        existing.count++
      } else {
        map.set(cc, { code: cc, count: 1 })
      }
    }
    return [...map.values()].sort((a, b) => b.count - a.count)
  }, [servers])

  // 搜索 + 标签 + 地区筛选 + 排序
  const processedServers = useMemo(() => {
    // 1. 搜索筛选（名称/主机名/标签/位置/供应商）
    let result = servers
    if (debouncedSearch) {
      result = result.filter((s) => {
        const name = (s.display_name || '').toLowerCase()
        const hostname = (s.hostname || '').toLowerCase()
        const tags = (s.tags || '').toLowerCase()
        const region = (s.region || '').toLowerCase()
        const isp = (s.isp || '').toLowerCase()
        return (
          name.includes(debouncedSearch) ||
          hostname.includes(debouncedSearch) ||
          tags.includes(debouncedSearch) ||
          region.includes(debouncedSearch) ||
          isp.includes(debouncedSearch)
        )
      })
    }

    // 2. 标签筛选
    if (tagFilter) {
      result = result.filter((s) => parseTags(s.tags).includes(tagFilter))
    }

    // 3. 地区筛选（按 country_code）
    if (regionFilter) {
      result = result.filter((s) => getCountryCode(s) === regionFilter)
    }

    // 4. IP 栈筛选（v4=有 IPv4 出口 / v6=有 IPv6 出口 / dual=双栈）
    if (ipFilter) {
      result = result.filter((s) => {
        switch (ipFilter) {
          case 'v4':
            return !!s.ipv4
          case 'v6':
            return !!s.ipv6
          case 'dual':
            return !!s.ipv4 && !!s.ipv6
          default:
            return true
        }
      })
    }

    // 5. 排序（在线节点排在离线节点之前）
    if (sortOption === 'default') {
      // 默认排序：在线优先，然后保持原顺序
      result = [...result].sort((a, b) => {
        if (a.online !== b.online) return a.online ? -1 : 1
        return 0
      })
    } else {
      result = [...result].sort((a, b) => {
        // 在线优先
        if (a.online !== b.online) return a.online ? -1 : 1

        let cmp = 0
        switch (sortOption) {
          case 'name':
            cmp = (a.display_name || a.hostname || '').localeCompare(b.display_name || b.hostname || '')
            break
          case 'cpu':
            cmp = (a.cpu || 0) - (b.cpu || 0)
            break
          case 'mem':
            cmp = getMemUsage(a) - getMemUsage(b)
            break
          case 'disk':
            cmp = (a.disk_usage || 0) - (b.disk_usage || 0)
            break
          case 'net':
            cmp = getTotalNetSpeed(a) - getTotalNetSpeed(b)
            break
          case 'uptime':
            cmp = (a.uptime || 0) - (b.uptime || 0)
            break
          case 'traffic':
            cmp = getMonthlyTraffic(a) - getMonthlyTraffic(b)
            break
          case 'latency':
            cmp = getAvgLatency(a) - getAvgLatency(b)
            break
          case 'expire':
            cmp = getExpireDays(a) - getExpireDays(b)
            break
        }
        // 降序排列（值大的在前），名称/到期升序
        return sortOption === 'name' || sortOption === 'expire' ? cmp : -cmp
      })
    }

    return result
  }, [servers, debouncedSearch, tagFilter, regionFilter, ipFilter, sortOption])

  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchInput(e.target.value)
  }, [])

  return (
    <div className="space-y-4">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-foreground">仪表盘</h1>
          <p className="mt-0.5 text-xs text-muted-foreground sm:text-sm">
            实时监控所有服务器状态
          </p>
        </div>
        <button
          onClick={() => fetchServers().catch(() => {})}
          className="btn-outline h-10"
          title="重新拉取服务器列表"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新
        </button>
      </div>

      {/* 统计卡片（card-soft） */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-6">
        {/* 在线/离线 */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">服务器</span>
            <span className="text-xs font-medium text-success">
              {stats.online} 在线
            </span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-semibold tabular-nums text-foreground">{stats.total}</span>
            <span className="text-sm text-muted-foreground">台</span>
            {stats.offline > 0 && (
              <span className="ml-auto text-xs text-destructive">{stats.offline} 离线</span>
            )}
          </div>
        </div>

        {/* 平均 CPU */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">平均 CPU</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-semibold tabular-nums text-cpu">
              {stats.avgCpu.toFixed(1)}
            </span>
            <span className="text-sm text-muted-foreground">%</span>
          </div>
        </div>

        {/* 平均内存 */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">平均内存</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-semibold tabular-nums text-mem">
              {stats.avgMem.toFixed(1)}
            </span>
            <span className="text-sm text-muted-foreground">%</span>
          </div>
        </div>

        {/* 总流量 */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">总流量</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-sm font-bold tabular-nums text-net">
              ↓{formatSpeed(stats.totalRx)}
            </span>
            <span className="text-sm text-muted-foreground">/</span>
            <span className="text-sm font-bold tabular-nums text-net">
              ↑{formatSpeed(stats.totalTx)}
            </span>
          </div>
        </div>

        {/* 本月总流量 */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">本月总流量</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-semibold tabular-nums text-net">
              {formatTraffic(stats.monthlyTraffic)}
            </span>
          </div>
        </div>

        {/* 月成本合计（按币种分组） */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">月成本合计</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            {stats.monthlyCost.length > 0 ? (
              stats.monthlyCost.map(({ currency, amount }) => (
                <span
                  key={currency}
                  className="text-2xl font-semibold tabular-nums text-foreground"
                  title={`${currency}（年付已折算为月）`}
                >
                  {currency === 'CNY' ? '¥' : currency === 'USD' ? '$' : `${currency} `}
                  {amount.toFixed(2)}
                </span>
              ))
            ) : (
              <span className="text-sm text-muted-foreground">未设置</span>
            )}
          </div>
        </div>
      </div>

      {/* 工具栏：搜索框 + 筛选下拉 + 排序下拉 + 视图切换（分段控制器） */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        {/* 左侧：搜索框 + 标签/地区筛选 + 排序 */}
        <div className="flex flex-wrap items-center gap-2">
          {/* 搜索框（input-base 紧凑高度） */}
          <div className="relative">
            <svg
              className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              value={searchInput}
              onChange={handleSearchChange}
              placeholder="搜索名称/标签/位置"
              className="input-base h-10 w-44 pl-8 pr-8 sm:w-56"
            />
            {searchInput && (
              <button
                onClick={() => setSearchInput('')}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                aria-label="清除搜索"
              >
                <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>

          {/* 标签筛选下拉 */}
          {allTags.length > 0 && (
            <FilterSelect
              value={tagFilter}
              onChange={(v) => setTagFilter(v)}
              ariaLabel="按标签筛选"
              options={[{ value: '', label: '全部标签' }, ...allTags.map((t) => ({ value: t, label: t }))]}
            />
          )}

          {/* 地区筛选下拉（含国旗） */}
          {allRegions.length > 0 && (
            <FilterSelect
              value={regionFilter}
              onChange={(v) => setRegionFilter(v)}
              ariaLabel="按地区筛选"
              options={[
                { value: '', label: '全部地区' },
                ...allRegions.map(({ code, count }) => ({ value: code, label: `${code} (${count})` })),
              ]}
            />
          )}

          {/* IP 栈筛选下拉 */}
          <FilterSelect
            value={ipFilter}
            onChange={(v) => setIpFilter(v as IPFilter)}
            ariaLabel="按 IP 栈筛选"
            options={IP_FILTER_OPTIONS}
          />

          {/* 排序下拉菜单 */}
          <FilterSelect
            value={sortOption}
            onChange={(v) => setSortOption(v as SortOption)}
            ariaLabel="排序方式"
            options={SORT_OPTIONS.map((o) => ({ value: o.value, label: `排序: ${o.label}` }))}
          />

          {/* 激活的筛选标签徽章（可点击移除） */}
          {tagFilter && (
            <button
              onClick={() => setTagFilter('')}
              className="badge-pill font-semibold transition-transform hover:scale-105"
              style={tagColors[tagFilter] ? { background: tagColors[tagFilter], color: '#fff' } : getTagStyle(tagFilter)}
            >
              {tagFilter}
              <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
          {regionFilter && (
            <button
              onClick={() => setRegionFilter('')}
              className="badge-pill border border-border bg-card text-foreground hover:bg-accent"
            >
              {getFlagEmoji(regionFilter)} {regionFilter}
              <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
          {ipFilter && (
            <button
              onClick={() => setIpFilter('')}
              className="badge-pill border border-border bg-card text-foreground hover:bg-accent"
            >
              {ipFilter === 'dual' ? 'IPv4+IPv6' : `IPv${ipFilter === 'v4' ? 4 : 6}`}
              <svg className="h-3 w-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>

        {/* 右侧：视图切换 - 分段控制器（NodeGet 紧凑） */}
        <div className="flex items-center rounded-md border border-border bg-card p-0.5">
          <button
            onClick={() => setViewMode('card')}
            className={`flex h-8 items-center gap-1.5 rounded px-2.5 text-xs font-medium transition-colors ${
              viewMode === 'card'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
            </svg>
            卡片
          </button>
          <button
            onClick={() => setViewMode('table')}
            className={`flex h-8 items-center gap-1.5 rounded px-2.5 text-xs font-medium transition-colors ${
              viewMode === 'table'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
            </svg>
            表格
          </button>
          <button
            onClick={() => setViewMode('map')}
            className={`flex h-8 items-center gap-1.5 rounded px-2.5 text-xs font-medium transition-colors ${
              viewMode === 'map'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3.05 11.5a9 9 0 1012.6-8.27M3.05 11.5a9 9 0 019-9m-9 9H21m0 0a9 9 0 01-9 9m9-9a9 9 0 00-9-9m0 18a9 9 0 01-9-9m9 9a9 9 0 009-9" />
            </svg>
            地图
          </button>
        </div>
      </div>

      {/* 服务器列表 */}
      {servers.length === 0 ? (
        <EmptyState
          className="rounded-lg border border-dashed border-border py-16"
          icon={
            <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
            </svg>
          }
          title="暂无服务器"
          description="请在服务器上安装 Agent 并注册"
        />
      ) : processedServers.length === 0 ? (
        // 搜索无结果
        <EmptyState
          className="rounded-lg border border-dashed border-border py-16"
          icon={
            <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          }
          title="未找到匹配的服务器"
          description="尝试使用其他关键字搜索"
        />
      ) : viewMode === 'card' ? (
        // 卡片视图
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {processedServers.map((server) => (
            <ServerCard key={server.id} server={server} />
          ))}
        </div>
      ) : viewMode === 'map' ? (
        // 地图视图（ECharts 世界地图 + effectScatter 光点）
        <MapView servers={processedServers} basePath="/admin" />
      ) : (
        // 表格视图（card-soft overflow-hidden）
        <div className="card-soft overflow-hidden">
          <div className="table-shell">
            <table className="w-full min-w-[1160px]">
              <thead>
                <tr className="border-b border-border">
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">状态</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">名称</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">位置</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">CPU</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">内存</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">磁盘</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">下行</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">上行</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">月流量</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">延迟</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">到期</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">运行时长</th>
                </tr>
              </thead>
              <tbody>
                {processedServers.map((server) => (
                  <ServerTableRow key={server.id} server={server} basePath="/admin" />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* WebSocket 断线提示 */}
      {!wsConnected && servers.length > 0 && (
        <div className="glass fixed bottom-4 right-4 rounded-md border border-warning/30 px-4 py-2 text-sm text-warning shadow-lg">
          实时连接已断开，正在重连...
        </div>
      )}
    </div>
  )
}
