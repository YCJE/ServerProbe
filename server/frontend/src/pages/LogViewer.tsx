import { useEffect, useState, useCallback, useRef, useMemo } from 'react'
import { getLogs } from '@/lib/api'
import type { LogEntry } from '@/lib/api'

/** 日志级别配置：颜色、标签、排序权重 */
const LEVEL_CONFIG: Record<string, { label: string; color: string; bg: string; dot: string; order: number }> = {
  ALL:      { label: '全部',   color: 'text-foreground',          bg: 'bg-secondary',           dot: 'bg-muted-foreground', order: 0 },
  INFO:     { label: 'INFO',   color: 'text-blue-400',            bg: 'bg-blue-500/10',         dot: 'bg-blue-500',         order: 1 },
  WARNING:  { label: 'WARN',   color: 'text-amber-400',           bg: 'bg-amber-500/10',        dot: 'bg-amber-500',        order: 2 },
  ERROR:    { label: 'ERROR',  color: 'text-red-400',             bg: 'bg-red-500/10',          dot: 'bg-red-500',          order: 3 },
  DEBUG:    { label: 'DEBUG',  color: 'text-purple-400',          bg: 'bg-purple-500/10',       dot: 'bg-purple-500',       order: 4 },
}

/** 格式化时间戳为 HH:MM:SS.mmm */
function formatTime(ts: string): string {
  const d = new Date(ts)
  const h = String(d.getHours()).padStart(2, '0')
  const m = String(d.getMinutes()).padStart(2, '0')
  const s = String(d.getSeconds()).padStart(2, '0')
  const ms = String(d.getMilliseconds()).padStart(3, '0')
  return `${h}:${m}:${s}.${ms}`
}

/** 格式化日期前缀 */
function formatDate(ts: string): string {
  const d = new Date(ts)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** 日志查看器页面 */
export default function LogViewer() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [activeLevel, setActiveLevel] = useState<string>('ALL')
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [lastUpdate, setLastUpdate] = useState(0)

  const mountedRef = useRef(true)
  const requestIdRef = useRef(0)
  const logContainerRef = useRef<HTMLDivElement>(null)
  const prevLogCountRef = useRef(0)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const loadLogs = useCallback(async () => {
    if (!mountedRef.current) return
    const requestId = ++requestIdRef.current
    setLoading(true)
    try {
      const data = await getLogs({
        level: activeLevel,
        limit: 500,
        search: search || undefined,
      })
      if (mountedRef.current && requestIdRef.current === requestId) {
        setLogs(data.logs || [])
        setTotal(data.total || 0)
        setError('')
        setLastUpdate(Date.now())
      }
    } catch (err) {
      if (mountedRef.current && requestIdRef.current === requestId) {
        setError(err instanceof Error ? err.message : '加载日志失败')
      }
    } finally {
      if (mountedRef.current && requestIdRef.current === requestId) {
        setLoading(false)
      }
    }
  }, [activeLevel, search])

  // 初始加载 + 自动刷新
  useEffect(() => {
    loadLogs()
    if (!autoRefresh) return
    const interval = setInterval(loadLogs, 3000)

    const handleVisibilityChange = () => {
      if (document.hidden) {
        clearInterval(interval)
      } else if (mountedRef.current) {
        loadLogs()
      }
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => {
      clearInterval(interval)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [loadLogs, autoRefresh])

  // 新日志到达时自动滚动到底部
  useEffect(() => {
    if (logs.length > prevLogCountRef.current && logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
    prevLogCountRef.current = logs.length
  }, [logs.length])

  // 搜索防抖
  useEffect(() => {
    const timer = setTimeout(() => {
      setSearch(searchInput)
    }, 400)
    return () => clearTimeout(timer)
  }, [searchInput])

  // 统计各级别日志数量
  const stats = useMemo(() => {
    const counts = { INFO: 0, WARNING: 0, ERROR: 0, DEBUG: 0 }
    logs.forEach((log) => {
      if (log.level in counts) {
        counts[log.level as keyof typeof counts]++
      }
    })
    return counts
  }, [logs])

  // 检测是否跨天
  const dateLabels = useMemo(() => {
    const dates = new Set<string>()
    logs.forEach((log) => dates.add(formatDate(log.timestamp)))
    return dates
  }, [logs])

  return (
    <div className="flex h-full flex-col space-y-4">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">系统日志</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            服务端运行日志，用于调试和排查问题
          </p>
        </div>
        <div className="flex items-center gap-3">
          {lastUpdate > 0 && (
            <span className="text-xs text-muted-foreground">
              最后更新：{new Date(lastUpdate).toLocaleTimeString('zh-CN')}
            </span>
          )}
          <button
            onClick={() => setAutoRefresh((v) => !v)}
            className={`flex h-9 items-center gap-1.5 rounded-lg border px-3 text-sm transition-colors ${
              autoRefresh
                ? 'border-primary/30 bg-primary/10 text-primary'
                : 'border-border bg-card text-muted-foreground hover:bg-accent'
            }`}
          >
            {autoRefresh && (
              <span className="relative flex h-2 w-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-primary" />
              </span>
            )}
            {autoRefresh ? '实时' : '已暂停'}
          </button>
          <button
            onClick={loadLogs}
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

      {/* 统计卡片 + 过滤器 */}
      <div className="flex flex-wrap items-center gap-2">
        {/* 级别过滤 */}
        <div className="flex items-center gap-1 rounded-lg border border-border bg-card p-1">
          {Object.entries(LEVEL_CONFIG).map(([key, cfg]) => {
            const isActive = activeLevel === key
            const count = key === 'ALL' ? logs.length : stats[key as keyof typeof stats] || 0
            return (
              <button
                key={key}
                onClick={() => setActiveLevel(key)}
                className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-all ${
                  isActive
                    ? 'bg-primary text-primary-foreground shadow-sm'
                    : 'text-muted-foreground hover:bg-accent hover:text-foreground'
                }`}
              >
                {key !== 'ALL' && (
                  <span
                    className="inline-block h-1.5 w-1.5 rounded-full"
                    style={{ backgroundColor: isActive ? 'currentColor' : cfg.dot }}
                  />
                )}
                {cfg.label}
                {count > 0 && (
                  <span className={`rounded-full px-1.5 text-[10px] ${
                    isActive ? 'bg-primary-foreground/20' : 'bg-secondary'
                  }`}>
                    {count}
                  </span>
                )}
              </button>
            )
          })}
        </div>

        {/* 搜索框 */}
        <div className="relative flex-1 min-w-[200px]">
          <svg
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
            fill="none" stroke="currentColor" viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="搜索日志内容..."
            className="h-9 w-full rounded-lg border border-border bg-card pl-9 pr-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
          />
          {searchInput && (
            <button
              onClick={() => { setSearchInput(''); setSearch('') }}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>

        {/* 总量 */}
        <div className="flex items-center gap-1.5 rounded-lg border border-border bg-card px-3 py-1.5 text-xs text-muted-foreground">
          <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
          </svg>
          共 {total} 条
        </div>
      </div>

      {/* 错误提示 */}
      {error && (
        <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* 日志终端区域 */}
      <div className="flex-1 min-h-0 overflow-hidden rounded-xl border border-border bg-[#1a1b26] shadow-lg">
        {/* 终端标题栏 */}
        <div className="flex items-center justify-between border-b border-white/5 bg-white/[0.02] px-4 py-2">
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-1.5">
              <span className="h-3 w-3 rounded-full bg-[#FF5F57]" />
              <span className="h-3 w-3 rounded-full bg-[#FEBC2E]" />
              <span className="h-3 w-3 rounded-full bg-[#28C840]" />
            </div>
            <span className="ml-2 text-xs font-medium text-white/40" style={{ fontFamily: 'var(--font-mono)' }}>
              server-probe.log
            </span>
          </div>
          <div className="flex items-center gap-2 text-[10px] text-white/30">
            {dateLabels.size > 1 && (
              <span>{Array.from(dateLabels).length} 天</span>
            )}
            {autoRefresh && (
              <span className="flex items-center gap-1">
                <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-green-400" />
                LIVE
              </span>
            )}
          </div>
        </div>

        {/* 日志内容 */}
        <div
          ref={logContainerRef}
          className="h-[calc(100%-37px)] overflow-y-auto scrollbar-thin"
          style={{ fontFamily: 'var(--font-mono)' }}
        >
          {loading && logs.length === 0 ? (
            <div className="flex items-center justify-center py-16">
              <div className="h-6 w-6 animate-spin rounded-full border-2 border-white/20 border-t-white/60" />
            </div>
          ) : logs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-white/30">
              <svg className="mb-3 h-10 w-10" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
              <span className="text-sm">暂无日志记录</span>
            </div>
          ) : (
            <div className="py-1">
              {logs.map((log, i) => {
                const cfg = LEVEL_CONFIG[log.level] || LEVEL_CONFIG.INFO
                const prevLog = i > 0 ? logs[i - 1] : null
                const showDate = !prevLog || formatDate(log.timestamp) !== formatDate(prevLog.timestamp)

                return (
                  <div key={i}>
                    {showDate && (
                      <div className="sticky top-0 z-10 bg-[#1a1b26]/90 px-4 py-1 text-[10px] font-medium text-white/30 backdrop-blur-sm">
                        ── {formatDate(log.timestamp)} ──
                      </div>
                    )}
                    <div className="group flex items-start gap-3 px-4 py-0.5 hover:bg-white/[0.03] transition-colors">
                      {/* 时间戳 */}
                      <span className="shrink-0 text-[11px] text-white/30 tabular-nums">
                        {formatTime(log.timestamp)}
                      </span>
                      {/* 级别标签 */}
                      <span className={`shrink-0 rounded px-1.5 py-0 text-[10px] font-semibold leading-5 ${cfg.bg} ${cfg.color}`}>
                        {log.level}
                      </span>
                      {/* 日志消息 */}
                      <span className="min-w-0 flex-1 break-all text-[12px] leading-5 text-white/70 group-hover:text-white/90">
                        {log.message}
                      </span>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
