import { memo, useMemo, useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ServerCard from '@/components/ServerCard'
import DistroIcon from '@/components/DistroIcon'
import StatusDot from '@/components/StatusDot'
import { formatSpeed, formatUptime } from '@/lib/utils'
import type { ServerData } from '@/types'

/** 视图模式 */
type ViewMode = 'card' | 'table'

/** 排序选项 */
type SortOption = 'default' | 'name' | 'cpu' | 'mem' | 'disk' | 'net' | 'uptime'

/** 排序选项列表 */
const SORT_OPTIONS: { value: SortOption; label: string }[] = [
  { value: 'default', label: '默认' },
  { value: 'name', label: '名称' },
  { value: 'cpu', label: 'CPU' },
  { value: 'mem', label: '内存' },
  { value: 'disk', label: '磁盘' },
  { value: 'net', label: '网速' },
  { value: 'uptime', label: '运行时长' },
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
          <DistroIcon distro={server.distro} os={server.os} size={14} />
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
    s.virtualization === n.virtualization
  )
})

/** 仪表盘页（服务器卡片网格 + 表格视图，NodeGet 风格） */
export default function Dashboard() {
  const servers = useServerStore((s) => s.servers)
  const fetchServers = useServerStore((s) => s.fetchServers)
  const wsConnected = useServerStore((s) => s.wsConnected)

  // 视图模式（持久化）
  const [viewMode, setViewMode] = useState<ViewMode>(() =>
    loadLS<ViewMode>(LS_VIEW_MODE, ['card', 'table'], 'card'),
  )
  // 排序选项（持久化）
  const [sortOption, setSortOption] = useState<SortOption>(() =>
    loadLS<SortOption>(LS_SORT, SORT_OPTIONS.map((o) => o.value), 'default'),
  )
  // 搜索关键字
  const [searchInput, setSearchInput] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')

  // 视图模式 / 排序选项持久化
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

  // 搜索防抖 400ms
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput.trim().toLowerCase())
    }, 400)
    return () => clearTimeout(timer)
  }, [searchInput])

  // 统计信息
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

    return { total, online, offline, avgCpu, avgMem, totalRx, totalTx }
  }, [servers])

  // 搜索 + 排序
  const processedServers = useMemo(() => {
    // 1. 搜索筛选
    let result = servers
    if (debouncedSearch) {
      result = servers.filter((s) => {
        const name = (s.display_name || '').toLowerCase()
        const hostname = (s.hostname || '').toLowerCase()
        return name.includes(debouncedSearch) || hostname.includes(debouncedSearch)
      })
    }

    // 2. 排序（在线节点排在离线节点之前）
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
        }
        // 降序排列（值大的在前），名称升序
        return sortOption === 'name' ? cmp : -cmp
      })
    }

    return result
  }, [servers, debouncedSearch, sortOption])

  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setSearchInput(e.target.value)
  }, [])

  return (
    <div className="space-y-4">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-bold text-foreground sm:text-xl">仪表盘</h1>
          <p className="mt-0.5 text-xs text-muted-foreground sm:text-sm">
            实时监控所有服务器状态
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchServers().catch(() => {})}
            className="flex h-11 items-center gap-1.5 rounded-xl border border-border bg-secondary px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent"
          >
            <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            刷新
          </button>
        </div>
      </div>

      {/* 统计卡片（card-soft） */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        {/* 在线/离线 */}
        <div className="card-soft p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">服务器</span>
            <span className="text-xs font-medium text-success">
              {stats.online} 在线
            </span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-bold tabular-nums text-foreground">{stats.total}</span>
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
            <span className="text-2xl font-bold tabular-nums text-foreground">
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
            <span className="text-2xl font-bold tabular-nums text-foreground">
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
            <span className="text-sm font-bold tabular-nums text-foreground">
              ↓{formatSpeed(stats.totalRx)}
            </span>
            <span className="text-sm text-muted-foreground">/</span>
            <span className="text-sm font-bold tabular-nums text-foreground">
              ↑{formatSpeed(stats.totalTx)}
            </span>
          </div>
        </div>
      </div>

      {/* 工具栏：搜索框 + 排序下拉 + 视图切换（分段控制器） */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        {/* 左侧：搜索框 + 排序 */}
        <div className="flex items-center gap-2">
          {/* 搜索框 */}
          <div className="relative">
            <svg
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
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
              placeholder="搜索名称/主机名"
              className="h-11 w-44 rounded-xl border border-border bg-secondary pl-9 pr-3 text-sm font-semibold text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary sm:w-56"
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

          {/* 排序下拉菜单 */}
          <div className="relative">
            <select
              value={sortOption}
              onChange={(e) => setSortOption(e.target.value as SortOption)}
              className="h-11 cursor-pointer appearance-none rounded-xl border border-border bg-secondary px-3 pr-8 text-sm font-medium text-foreground transition-colors hover:bg-accent focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
            >
              {SORT_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  排序: {opt.label}
                </option>
              ))}
            </select>
            <svg
              className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </div>
        </div>

        {/* 右侧：视图切换 - 分段控制器 */}
        <div className="flex items-center rounded-xl border border-border bg-muted p-1">
          <button
            onClick={() => setViewMode('card')}
            className={`flex h-9 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition-all ${
              viewMode === 'card'
                ? 'bg-background text-foreground shadow-sm'
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
            className={`flex h-9 items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition-all ${
              viewMode === 'table'
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground'
            }`}
          >
            <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
            </svg>
            表格
          </button>
        </div>
      </div>

      {/* 服务器列表 */}
      {servers.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16">
          <svg className="mb-3 h-12 w-12 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
          </svg>
          <p className="text-sm font-medium text-foreground">暂无服务器</p>
          <p className="mt-1 text-xs text-muted-foreground">
            请在服务器上安装 Agent 并注册
          </p>
        </div>
      ) : processedServers.length === 0 ? (
        // 搜索无结果
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16">
          <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <p className="text-sm font-medium text-foreground">未找到匹配的服务器</p>
          <p className="mt-1 text-xs text-muted-foreground">
            尝试使用其他关键字搜索
          </p>
        </div>
      ) : viewMode === 'card' ? (
        // 卡片视图
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {processedServers.map((server) => (
            <ServerCard key={server.id} server={server} />
          ))}
        </div>
      ) : (
        // 表格视图（card-soft overflow-hidden）
        <div className="card-soft overflow-hidden">
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full min-w-[640px]">
              <thead>
                <tr className="border-b border-border">
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">状态</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">名称</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">CPU</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">内存</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">磁盘</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">下行</th>
                  <th className="h-10 px-3 text-left text-xs font-medium text-muted-foreground">上行</th>
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
        <div className="glass fixed bottom-4 right-4 rounded-xl border border-warning/30 px-4 py-2 text-sm text-warning shadow-lg">
          实时连接已断开，正在重连...
        </div>
      )}
    </div>
  )
}
