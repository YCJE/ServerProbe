import { useCallback, useEffect, useState } from 'react'
import { getAuditLogs, type AuditLogEntry } from '@/lib/api'

const PAGE_SIZE = 50

/** HTTP 方法对应的徽章配色 */
const METHOD_STYLES: Record<string, string> = {
  GET: 'bg-muted text-muted-foreground',
  POST: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  PUT: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  PATCH: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  DELETE: 'bg-destructive/10 text-destructive',
}

/** 审计日志页（P1：管理端变更操作全量落库，可按管理员/操作/结果检索） */
export default function AuditLogs() {
  const [logs, setLogs] = useState<AuditLogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // 筛选条件
  const [username, setUsername] = useState('')
  const [action, setAction] = useState('')
  const [successFilter, setSuccessFilter] = useState<'all' | 'true' | 'false'>('all')

  const loadLogs = useCallback(async (targetPage: number) => {
    setLoading(true)
    setError('')
    try {
      const res = await getAuditLogs({
        username: username.trim() || undefined,
        action: action.trim() || undefined,
        success: successFilter === 'all' ? undefined : successFilter === 'true',
        page: targetPage,
        page_size: PAGE_SIZE,
      })
      setLogs(res.logs || [])
      setTotal(res.total || 0)
      setPage(res.page || targetPage)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载审计日志失败')
    } finally {
      setLoading(false)
    }
  }, [username, action, successFilter])

  useEffect(() => {
    loadLogs(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [username, action, successFilter])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const handleSearch = () => loadLogs(1)

  const handleReset = () => {
    setUsername('')
    setAction('')
    setSuccessFilter('all')
  }

  const formatTime = (timeStr: string) => {
    if (!timeStr) return '-'
    return new Date(timeStr).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  /** "POST /api/v1/agents/:id" → method=POST, path=agents/:id */
  const parseAction = (raw: string): { method: string; path: string } => {
    const spaceIdx = raw.indexOf(' ')
    if (spaceIdx === -1) return { method: '', path: raw }
    const method = raw.slice(0, spaceIdx)
    let path = raw.slice(spaceIdx + 1)
    if (path.startsWith('/api/v1/')) path = path.slice(8)
    return { method, path }
  }

  /** User-Agent 简化显示（浏览器名 + 平台） */
  const shortUA = (ua: string): string => {
    if (!ua) return '-'
    const m = ua.match(/\((.*?)\)/)
    const browser = ua.match(/(Firefox|Edg|Chrome|Safari|curl)\/[\d.]+/g)
    const parts: string[] = []
    if (m) parts.push(m[1].split(';')[0].trim())
    if (browser) parts.push(browser[browser.length - 1])
    return parts.join(' · ') || ua.slice(0, 40)
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-foreground">审计日志</h1>
        <p className="mt-0.5 text-sm text-muted-foreground">
          管理端全部变更操作自动落库（登录 / 增删改 / 敏感操作），保留 180 天
        </p>
      </div>

      {/* 筛选栏 */}
      <div className="card-soft p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="w-full sm:w-44">
            <label className="mb-1 block text-xs font-medium text-muted-foreground">管理员</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSearch()
              }}
              placeholder="精确匹配用户名"
              className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div className="w-full sm:w-56">
            <label className="mb-1 block text-xs font-medium text-muted-foreground">操作</label>
            <input
              type="text"
              value={action}
              onChange={(e) => setAction(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSearch()
              }}
              placeholder="模糊匹配，如 agents"
              className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
            />
          </div>
          <div className="w-full sm:w-32">
            <label className="mb-1 block text-xs font-medium text-muted-foreground">结果</label>
            <select
              value={successFilter}
              onChange={(e) => setSuccessFilter(e.target.value as 'all' | 'true' | 'false')}
              className="h-9 w-full cursor-pointer rounded-md border border-input bg-background px-2 text-sm text-foreground focus:border-primary focus:outline-none"
            >
              <option value="all">全部</option>
              <option value="true">仅成功</option>
              <option value="false">仅失败</option>
            </select>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleSearch}
              className="flex h-9 items-center rounded-md bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
            >
              查询
            </button>
            <button
              onClick={handleReset}
              className="flex h-9 items-center rounded-md border border-border px-4 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              重置
            </button>
          </div>
        </div>
        <p className="mt-2 text-xs text-muted-foreground">
          共 {total.toLocaleString('zh-CN')} 条记录
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-dashed border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* 日志表格 */}
      <div className="card-soft overflow-hidden">
        {loading && logs.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : logs.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8zM14 2v6h6M8 13h8M8 17h5" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无审计日志</p>
            <p className="mt-1 text-xs text-muted-foreground/70">
              管理端的登录与增删改操作会自动记录到这里
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">时间</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">管理员</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">操作</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">目标</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">结果</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">来源 IP</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground whitespace-nowrap">客户端</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {logs.map((log) => {
                  const { method, path } = parseAction(log.action)
                  return (
                    <tr key={log.id} className="text-foreground transition-colors hover:bg-muted/50">
                      <td className="px-3 py-2.5 text-xs tabular-nums text-muted-foreground whitespace-nowrap">
                        {formatTime(log.created_at)}
                      </td>
                      <td className="px-3 py-2.5 font-medium whitespace-nowrap">
                        {log.username || `#${log.admin_id}`}
                      </td>
                      <td className="px-3 py-2.5 whitespace-nowrap">
                        <span className="flex items-center gap-1.5">
                          {method && (
                            <span
                              className={`rounded px-1.5 py-0.5 font-mono text-[10px] font-bold ${
                                METHOD_STYLES[method] || 'bg-muted text-muted-foreground'
                              }`}
                            >
                              {method}
                            </span>
                          )}
                          <code className="font-mono text-xs text-foreground">{path}</code>
                        </span>
                      </td>
                      <td className="max-w-[220px] px-3 py-2.5">
                        <span className="block truncate font-mono text-xs text-muted-foreground" title={log.target}>
                          {log.target.replace('/api/v1', '') || '-'}
                        </span>
                      </td>
                      <td className="px-3 py-2.5 whitespace-nowrap">
                        <span className={`badge-pill ${log.success ? 'badge-success' : 'badge-destructive'}`}>
                          {log.success ? '成功' : '失败'}
                        </span>
                      </td>
                      <td className="px-3 py-2.5 font-mono text-xs text-muted-foreground whitespace-nowrap">
                        {log.ip || '-'}
                      </td>
                      <td className="max-w-[180px] px-3 py-2.5">
                        <span className="block truncate text-xs text-muted-foreground" title={log.user_agent}>
                          {shortUA(log.user_agent)}
                        </span>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}

        {/* 分页 */}
        {total > PAGE_SIZE && (
          <div className="flex items-center justify-between border-t border-dashed border-border px-4 py-3">
            <span className="text-xs text-muted-foreground">
              第 {page} / {totalPages} 页 · 共 {total.toLocaleString('zh-CN')} 条
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={() => loadLogs(page - 1)}
                disabled={page <= 1 || loading}
                className="flex h-8 items-center rounded-md border border-border px-3 text-xs font-medium text-foreground transition-colors hover:bg-accent disabled:opacity-40"
              >
                上一页
              </button>
              <button
                onClick={() => loadLogs(page + 1)}
                disabled={page >= totalPages || loading}
                className="flex h-8 items-center rounded-md border border-border px-3 text-xs font-medium text-foreground transition-colors hover:bg-accent disabled:opacity-40"
              >
                下一页
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
