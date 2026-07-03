import { useEffect, useMemo, useState } from 'react'
import { useServerStore } from '@/store/useServerStore'
import ServerCard from '@/components/ServerCard'
import {
  formatSpeed,
  getRegionFromServer,
  getFlagEmoji,
} from '@/lib/utils'

/** 聚合指标卡片 */
function StatCard({
  label,
  value,
  unit,
}: {
  label: string
  value: string
  unit?: string
}) {
  return (
    <div className="rounded-2xl border border-border bg-card p-4">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-2 flex items-baseline gap-1">
        <span className="truncate text-2xl font-bold text-foreground">{value}</span>
        {unit && <span className="text-xs text-muted-foreground">{unit}</span>}
      </div>
    </div>
  )
}

/** 公开仪表盘页（无需登录，显示服务器监控信息） */
export default function PublicDashboard() {
  const servers = useServerStore((s) => s.servers)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)
  const connectPublicDashboardWS = useServerStore((s) => s.connectPublicDashboardWS)
  const disconnectPublicDashboardWS = useServerStore((s) => s.disconnectPublicDashboardWS)

  const [selectedRegion, setSelectedRegion] = useState<string>('')
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')

  // 搜索框防抖，避免每次按键都触发过滤
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(searchQuery), 200)
    return () => clearTimeout(t)
  }, [searchQuery])

  // 连接公开 WebSocket
  useEffect(() => {
    connectPublicDashboardWS()
    return () => disconnectPublicDashboardWS()
  }, [connectPublicDashboardWS, disconnectPublicDashboardWS])

  // 聚合统计信息
  const stats = useMemo(() => {
    const total = servers.length
    const onlineServers = servers.filter((s) => s.online)
    const online = onlineServers.length
    const avgCpu = online > 0
      ? onlineServers.reduce((sum, s) => sum + (s.cpu || 0), 0) / online
      : 0
    const avgDisk = online > 0
      ? onlineServers.reduce((sum, s) => sum + (s.disk_usage || 0), 0) / online
      : 0
    const totalRx = onlineServers.reduce((sum, s) => sum + (s.net_rx || 0), 0)
    const totalTx = onlineServers.reduce((sum, s) => sum + (s.net_tx || 0), 0)
    const totalTraffic = totalRx + totalTx

    return { total, online, avgCpu, avgDisk, totalRx, totalTx, totalTraffic }
  }, [servers])

  // 从服务器数据中提取地区标签
  const regions = useMemo(() => {
    const set = new Set<string>()
    for (const s of servers) {
      const r = getRegionFromServer(s)
      if (r) set.add(r)
    }
    return Array.from(set)
  }, [servers])

  // 按地区和搜索关键词筛选
  const filteredServers = useMemo(() => {
    return servers.filter((s) => {
      if (selectedRegion && getRegionFromServer(s) !== selectedRegion) return false
      if (debouncedQuery) {
        const name = (s.display_name || s.hostname || '').toLowerCase()
        if (!name.includes(debouncedQuery.toLowerCase())) return false
      }
      return true
    })
  }, [servers, selectedRegion, debouncedQuery])

  return (
    <div className="space-y-6">
      {/* 页面标题 + WebSocket 状态指示器 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">服务器监控</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">实时监控所有服务器状态</p>
        </div>
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <span
            className={`inline-block h-2 w-2 rounded-full ${
              publicWsConnected ? 'bg-success animate-pulse' : 'bg-destructive'
            }`}
          />
          <span>{publicWsConnected ? '实时' : '已断开'}</span>
        </div>
      </div>

      {/* 聚合指标栏：6 个卡片 */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <StatCard label="CPU 使用率" value={stats.avgCpu.toFixed(1)} unit="%" />
        <StatCard label="磁盘使用率" value={stats.avgDisk.toFixed(1)} unit="%" />
        <StatCard
          label="可用服务器"
          value={String(stats.online)}
          unit={`/ ${stats.total}`}
        />
        <StatCard label="总流量" value={formatSpeed(stats.totalTraffic)} />
        <StatCard label="上传速度" value={formatSpeed(stats.totalTx)} />
        <StatCard label="下载速度" value={formatSpeed(stats.totalRx)} />
      </div>

      {/* 筛选栏：地区胶囊 + 搜索框 */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <button
            className={`filter-pill ${
              selectedRegion === '' ? 'filter-pill-active' : 'filter-pill-inactive'
            }`}
            onClick={() => setSelectedRegion('')}
          >
            全部
          </button>
          {regions.map((r) => (
            <button
              key={r}
              className={`filter-pill ${
                selectedRegion === r ? 'filter-pill-active' : 'filter-pill-inactive'
              }`}
              onClick={() => setSelectedRegion(r)}
            >
              {getFlagEmoji(r)} {r}
            </button>
          ))}
        </div>
        <input
          type="text"
          placeholder="搜索服务器..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full shrink-0 rounded-lg border border-border bg-card px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none sm:w-64"
        />
      </div>

      {/* 服务器卡片网格 */}
      {filteredServers.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-border py-16">
          <svg
            className="mb-3 h-12 w-12 text-muted-foreground/50"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01"
            />
          </svg>
          <p className="text-sm font-medium text-foreground">暂无服务器数据</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {servers.length === 0 ? '等待服务器接入' : '没有匹配的服务器'}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filteredServers.map((server) => (
            <ServerCard key={server.id} server={server} basePath="" />
          ))}
        </div>
      )}

      {/* WebSocket 断线提示 */}
      {!publicWsConnected && servers.length > 0 && (
        <div className="fixed bottom-4 right-4 rounded-lg border border-warning/30 bg-warning/10 px-4 py-2 text-sm text-warning shadow-lg">
          实时连接已断开，正在重连...
        </div>
      )}
    </div>
  )
}
