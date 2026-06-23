import { useEffect, useMemo } from 'react'
import { useServerStore } from '@/store/useServerStore'
import ServerCard from '@/components/ServerCard'

/** 公开仪表盘页（无需登录，显示服务器监控信息） */
export default function PublicDashboard() {
  const servers = useServerStore((s) => s.servers)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)
  const connectPublicDashboardWS = useServerStore((s) => s.connectPublicDashboardWS)
  const disconnectPublicDashboardWS = useServerStore((s) => s.disconnectPublicDashboardWS)

  // 连接公开 WebSocket
  useEffect(() => {
    connectPublicDashboardWS()
    return () => disconnectPublicDashboardWS()
  }, [connectPublicDashboardWS, disconnectPublicDashboardWS])

  // 统计信息
  const stats = useMemo(() => {
    const total = servers.length
    const online = servers.filter((s) => s.online).length
    const offline = total - online
    const avgCpu = online > 0
      ? servers.filter((s) => s.online).reduce((sum, s) => sum + s.cpu, 0) / online
      : 0
    const avgMem = online > 0
      ? servers.filter((s) => s.online).reduce((sum, s) => sum + s.mem, 0) / online
      : 0
    const totalRx = servers.filter((s) => s.online).reduce((sum, s) => sum + (s.net_rx || 0), 0)
    const totalTx = servers.filter((s) => s.online).reduce((sum, s) => sum + (s.net_tx || 0), 0)

    return { total, online, offline, avgCpu, avgMem, totalRx, totalTx }
  }, [servers])

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-xl font-bold text-foreground">服务器监控</h1>
        <p className="mt-0.5 text-sm text-muted-foreground">
          实时监控所有服务器状态
        </p>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {/* 在线/离线 */}
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">服务器</span>
            <span className="text-xs font-medium text-success">
              {stats.online} 在线
            </span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-bold text-foreground">{stats.total}</span>
            <span className="text-sm text-muted-foreground">台</span>
            {stats.offline > 0 && (
              <span className="ml-auto text-xs text-destructive">{stats.offline} 离线</span>
            )}
          </div>
        </div>

        {/* 平均 CPU */}
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">平均 CPU</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-bold text-foreground">
              {stats.avgCpu.toFixed(1)}
            </span>
            <span className="text-sm text-muted-foreground">%</span>
          </div>
        </div>

        {/* 平均内存 */}
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">平均内存</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-2xl font-bold text-foreground">
              {stats.avgMem.toFixed(1)}
            </span>
            <span className="text-sm text-muted-foreground">%</span>
          </div>
        </div>

        {/* 总流量 */}
        <div className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">总流量</span>
          </div>
          <div className="mt-2 flex items-baseline gap-1">
            <span className="text-sm font-bold text-foreground">
              ↓{formatSpeedShort(stats.totalRx)}
            </span>
            <span className="text-sm text-muted-foreground">/</span>
            <span className="text-sm font-bold text-foreground">
              ↑{formatSpeedShort(stats.totalTx)}
            </span>
          </div>
        </div>
      </div>

      {/* 服务器卡片网格 */}
      {servers.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16">
          <svg className="mb-3 h-12 w-12 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
          </svg>
          <p className="text-sm font-medium text-foreground">暂无服务器数据</p>
          <p className="mt-1 text-xs text-muted-foreground">
            等待管理员添加服务器
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          {servers.map((server) => (
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

/** 格式化速率（简短版） */
function formatSpeedShort(bytesPerSec: number): string {
  if (bytesPerSec === 0 || bytesPerSec == null) return '0 B/s'
  const k = 1024
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  const i = Math.min(Math.floor(Math.log(bytesPerSec) / Math.log(k)), sizes.length - 1)
  return `${parseFloat((bytesPerSec / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}
