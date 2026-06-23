import { useEffect, useState, useCallback, useRef } from 'react'
import { getSystemStatus } from '@/lib/api'
import { formatBytes, getUsageColor, getUsageTextColor } from '@/lib/utils'
import type { SystemStatus } from '@/types'

/** 将秒数格式化为 x天x小时x分x秒 */
function formatUptimeFull(seconds: number): string {
  if (!seconds || seconds <= 0) return '0秒'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)

  const parts: string[] = []
  if (days > 0) parts.push(`${days}天`)
  if (hours > 0) parts.push(`${hours}小时`)
  if (minutes > 0) parts.push(`${minutes}分`)
  if (secs > 0 || parts.length === 0) parts.push(`${secs}秒`)
  return parts.join(' ')
}

/** 进度条组件 */
function ProgressBar({ value, color }: { value: number; color: string }) {
  return (
    <div className="h-2 w-full overflow-hidden rounded-full bg-secondary">
      <div
        className={`h-full rounded-full transition-all duration-500 ${color}`}
        style={{ width: `${Math.min(value, 100)}%` }}
      />
    </div>
  )
}

/** 指标卡片 */
function MetricCard({
  label,
  value,
  subValue,
  color,
  icon,
}: {
  label: string
  value: string
  subValue?: string
  color?: string
  icon?: string
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        {icon && <span className="text-sm">{icon}</span>}
        <span>{label}</span>
      </div>
      <div className={`mt-2 text-2xl font-bold ${color || 'text-foreground'}`}>
        {value}
      </div>
      {subValue && (
        <div className="mt-1 text-xs text-muted-foreground">{subValue}</div>
      )}
    </div>
  )
}

/** 系统状态页 */
export default function SystemStatus() {
  const [status, setStatus] = useState<SystemStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [lastUpdate, setLastUpdate] = useState<number>(0)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  const loadStatus = useCallback(async () => {
    if (!mountedRef.current) return
    setLoading(true)
    try {
      const data = await getSystemStatus()
      if (mountedRef.current) {
        setStatus(data)
        setError('')
        setLastUpdate(Date.now())
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(err instanceof Error ? err.message : '加载系统状态失败')
      }
    } finally {
      if (mountedRef.current) {
        setLoading(false)
      }
    }
  }, [])

  useEffect(() => {
    loadStatus()
    const interval = setInterval(loadStatus, 5000)
    return () => clearInterval(interval)
  }, [loadStatus])

  // 计算磁盘使用率
  const diskUsagePercent =
    status && status.disk_total > 0
      ? ((status.disk_total - status.disk_free) / status.disk_total) * 100
      : 0
  const diskUsed = status ? status.disk_total - status.disk_free : 0

  // 计算内存占比
  const memPercent =
    status && status.mem_sys > 0
      ? (status.mem_alloc / status.mem_sys) * 100
      : 0

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">系统状态</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            服务端运行状态监控，每 5 秒自动刷新
          </p>
        </div>
        <div className="flex items-center gap-3">
          {lastUpdate > 0 && (
            <span className="text-xs text-muted-foreground">
              最后更新：{new Date(lastUpdate).toLocaleTimeString('zh-CN')}
            </span>
          )}
          <button
            onClick={loadStatus}
            disabled={loading}
            className="flex h-9 items-center gap-1.5 rounded-lg border border-border bg-card px-3 text-sm text-foreground transition-colors hover:bg-accent disabled:opacity-50"
          >
            {loading ? (
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
            ) : (
              <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            )}
            刷新
          </button>
        </div>
      </div>

      {error && (
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      {loading && !status ? (
        <div className="flex items-center justify-center py-16">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      ) : status ? (
        <>
          {/* 核心指标卡片 */}
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-4">
            <MetricCard
              label="运行时间"
              icon="⏱"
              value={formatUptimeFull(status.uptime)}
              subValue={`启动于 ${new Date(Date.now() - status.uptime * 1000).toLocaleString('zh-CN')}`}
            />
            <MetricCard
              label="在线 Agent"
              icon="🖥"
              value={String(status.online_agents)}
              subValue="已连接的监控节点"
              color="text-success"
            />
            <MetricCard
              label="WebSocket 连接"
              icon="🔌"
              value={String(status.ws_connections)}
              subValue="当前活跃的 WS 连接"
            />
            <MetricCard
              label="Goroutine 数"
              icon="🧵"
              value={String(status.goroutines)}
              subValue="Go 运行时协程数"
            />
            <MetricCard
              label="GC 次数"
              icon="♻"
              value={String(status.mem_num_gc)}
              subValue="垃圾回收次数"
            />
            <MetricCard
              label="数据库大小"
              icon="💾"
              value={formatBytes(status.db_size)}
              subValue="SQLite 数据文件"
            />
            <MetricCard
              label="内存分配 (Alloc)"
              icon="📊"
              value={formatBytes(status.mem_alloc)}
              subValue={`占总系统内存 ${memPercent.toFixed(1)}%`}
              color={getUsageTextColor(memPercent)}
            />
            <MetricCard
              label="系统内存 (Sys)"
              icon="📈"
              value={formatBytes(status.mem_sys)}
              subValue="Go 运行时从系统获取的内存"
            />
          </div>

          {/* 磁盘空间使用情况 */}
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-foreground">磁盘空间使用</h2>
              <span className={`text-sm font-medium ${getUsageTextColor(diskUsagePercent)}`}>
                {diskUsagePercent.toFixed(1)}%
              </span>
            </div>
            <ProgressBar
              value={diskUsagePercent}
              color={getUsageColor(diskUsagePercent)}
            />
            <div className="mt-3 flex items-center justify-between text-sm">
              <div className="text-muted-foreground">
                已用 <span className="font-medium text-foreground">{formatBytes(diskUsed)}</span>
                {' '}/ 总{' '}
                <span className="font-medium text-foreground">{formatBytes(status.disk_total)}</span>
              </div>
              <div className="text-muted-foreground">
                可用 <span className="font-medium text-success">{formatBytes(status.disk_free)}</span>
              </div>
            </div>
          </div>

          {/* 内存使用情况 */}
          <div className="rounded-xl border border-border bg-card p-4">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-foreground">Go 运行时内存使用</h2>
              <span className={`text-sm font-medium ${getUsageTextColor(memPercent)}`}>
                {memPercent.toFixed(1)}%
              </span>
            </div>
            <ProgressBar
              value={memPercent}
              color={getUsageColor(memPercent)}
            />
            <div className="mt-3 flex items-center justify-between text-sm">
              <div className="text-muted-foreground">
                已分配 <span className="font-medium text-foreground">{formatBytes(status.mem_alloc)}</span>
                {' '}/ 系统{' '}
                <span className="font-medium text-foreground">{formatBytes(status.mem_sys)}</span>
              </div>
              <div className="text-muted-foreground">
                GC 次数 <span className="font-medium text-foreground">{status.mem_num_gc}</span>
              </div>
            </div>
          </div>

          {/* 版本信息 */}
          <div className="rounded-xl border border-border bg-card p-4">
            <h2 className="mb-3 text-sm font-semibold text-foreground">版本信息</h2>
            <div className="flex items-center gap-3">
              <span className="rounded-lg bg-primary/10 px-3 py-1.5 text-sm font-medium text-primary">
                v{status.version}
              </span>
              <span className="text-xs text-muted-foreground">
                服务端版本号
              </span>
            </div>
          </div>
        </>
      ) : !error ? (
        <div className="flex flex-col items-center justify-center py-16">
          <p className="text-sm text-muted-foreground">暂无系统状态数据</p>
        </div>
      ) : null}
    </div>
  )
}
