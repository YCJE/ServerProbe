import { useCallback, useEffect, useState } from 'react'
import { getAlertHistory } from '@/lib/api'
import type { AlertHistoryItem, AlertRule } from '@/types'

/** 告警历史时间线（FIRING 触发与 RESOLVED 恢复时间线） */
export default function AlertHistoryTimeline({ rules }: { rules: AlertRule[] }) {
  const [histories, setHistories] = useState<AlertHistoryItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [stateFilter, setStateFilter] = useState<'' | 'firing' | 'resolved'>('')
  const [ruleFilter, setRuleFilter] = useState<number | ''>('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const pageSize = 20

  const loadHistory = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await getAlertHistory({
        state: stateFilter || undefined,
        rule_id: ruleFilter === '' ? undefined : ruleFilter,
        page,
        page_size: pageSize,
      })
      setHistories(res.histories || [])
      setTotal(res.total || 0)
    } catch (err) {
      console.error('加载告警历史失败:', err)
      setHistories([])
      setTotal(0)
      setError(err instanceof Error ? err.message : '网络或服务异常')
    } finally {
      setLoading(false)
    }
  }, [stateFilter, ruleFilter, page])

  useEffect(() => {
    loadHistory()
  }, [loadHistory])

  // 筛选变化时回到第一页
  useEffect(() => {
    setPage(1)
  }, [stateFilter, ruleFilter])

  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  /** 格式化告警数值（按指标单位） */
  const formatValue = (h: AlertHistoryItem, v: number): string => {
    if (v === null || v === undefined) return '-'
    if (h.metric === 'agent_offline' || h.metric === 'service_status') {
      return v >= 1 ? '异常' : '正常'
    }
    if (h.metric === 'ssl_cert_expiry' || h.metric === 'expire_days') {
      return `${Math.round(v)} 天`
    }
    return `${v.toFixed(1)}%`
  }

  /** 格式化触发→恢复的持续时长 */
  const formatSpan = (h: AlertHistoryItem): string => {
    if (!h.resolved_at) return '进行中'
    const start = new Date(h.triggered_at).getTime()
    const end = new Date(h.resolved_at).getTime()
    if (isNaN(start) || isNaN(end) || end < start) return '-'
    const seconds = Math.floor((end - start) / 1000)
    if (seconds < 60) return `${seconds}秒`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟`
    if (seconds < 86400) return `${Math.floor(seconds / 3600)}小时`
    return `${Math.floor(seconds / 86400)}天`
  }

  /** 格式化时间（保留到分钟） */
  const formatTime = (iso: string | null): string => {
    if (!iso) return '-'
    const d = new Date(iso)
    if (isNaN(d.getTime())) return '-'
    const y = d.getFullYear()
    const m = String(d.getMonth() + 1).padStart(2, '0')
    const day = String(d.getDate()).padStart(2, '0')
    const h = String(d.getHours()).padStart(2, '0')
    const min = String(d.getMinutes()).padStart(2, '0')
    return `${y}-${m}-${day} ${h}:${min}`
  }

  return (
    <div className="space-y-4">
      {/* 筛选栏 */}
      <div className="flex flex-wrap items-center gap-2">
        {/* 状态筛选 */}
        <div className="flex items-center rounded-md border border-border bg-muted p-1">
          {(
            [
              { value: '', label: '全部' },
              { value: 'firing', label: '触发中' },
              { value: 'resolved', label: '已恢复' },
            ] as const
          ).map((opt) => (
            <button
              key={opt.value}
              onClick={() => setStateFilter(opt.value)}
              className={`flex h-8 items-center rounded-lg px-3 text-xs font-medium transition-all ${
                stateFilter === opt.value
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>

        {/* 规则筛选 */}
        {rules.length > 0 && (
          <select
            value={ruleFilter === '' ? '' : String(ruleFilter)}
            onChange={(e) => setRuleFilter(e.target.value === '' ? '' : Number(e.target.value))}
            className="h-10 cursor-pointer rounded-md border border-border bg-card px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent focus:border-primary focus:outline-none"
            aria-label="按规则筛选"
          >
            <option value="">全部规则</option>
            {rules.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        )}

        <span className="ml-auto text-xs text-muted-foreground">共 {total} 条记录</span>
      </div>

      {/* 时间线列表 */}
      {loading && histories.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      ) : error ? (
        <div className="flex items-center justify-between gap-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2.5">
          <div className="flex min-w-0 items-center gap-2 text-xs text-destructive">
            <svg className="h-4 w-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M12 9v4m0 4h.01M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" />
            </svg>
            <span className="truncate">告警历史加载失败：{error}</span>
          </div>
          <button
            onClick={() => loadHistory()}
            className="shrink-0 rounded-md border border-destructive/40 px-2.5 py-1 text-xs font-medium text-destructive transition-colors hover:bg-destructive/10"
          >
            重试
          </button>
        </div>
      ) : histories.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12">
          <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <p className="text-sm text-muted-foreground">暂无告警历史</p>
          <p className="mt-1 text-xs text-muted-foreground/70">告警触发与恢复后将记录在此</p>
        </div>
      ) : (
        <div className="card-soft p-5">
          <div className="relative">
            {/* 时间线竖线 */}
            <div className="absolute bottom-2 left-[7px] top-2 w-px bg-border" aria-hidden />
            <div className="space-y-5">
              {histories.map((h) => {
                const firing = h.state === 'firing'
                return (
                  <div key={h.id} className="relative flex gap-4 pl-6">
                    {/* 状态圆点 */}
                    <span
                      className={`absolute left-0 top-1.5 h-[15px] w-[15px] rounded-full border-2 border-card ${
                        firing ? 'bg-destructive' : 'bg-success'
                      }`}
                    >
                      {firing && (
                        <span className="absolute inset-0 animate-ping rounded-full bg-destructive/60" />
                      )}
                    </span>

                    {/* 内容 */}
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className={`badge-pill ${firing ? 'badge-destructive' : 'badge-success'}`}>
                          {firing ? 'FIRING 触发' : '已恢复'}
                        </span>
                        <span className="text-sm font-semibold text-foreground">{h.rule_name}</span>
                        <span className="truncate text-xs text-muted-foreground">· {h.server_name || `Agent #${h.agent_id}`}</span>
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                        <span>
                          触发值 <span className="font-semibold text-foreground">{formatValue(h, h.value)}</span>
                        </span>
                        {!firing && h.resolved_value != null && (
                          <span>
                            恢复值 <span className="font-semibold text-success">{formatValue(h, h.resolved_value)}</span>
                          </span>
                        )}
                        <span>
                          持续 <span className="font-semibold text-foreground">{formatSpan(h)}</span>
                        </span>
                        <span className="tabular-nums">{formatTime(h.triggered_at)}</span>
                        {!firing && h.resolved_at && (
                          <span className="tabular-nums">→ {formatTime(h.resolved_at)}</span>
                        )}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>

          {/* 分页 */}
          {totalPages > 1 && (
            <div className="mt-5 flex items-center justify-between border-t border-dashed border-border pt-4">
              <span className="text-xs text-muted-foreground">
                第 {page} / {totalPages} 页
              </span>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  className="flex h-8 items-center rounded-lg border border-border bg-secondary px-3 text-xs font-medium text-foreground transition-colors hover:bg-accent disabled:opacity-50"
                >
                  上一页
                </button>
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page >= totalPages}
                  className="flex h-8 items-center rounded-lg border border-border bg-secondary px-3 text-xs font-medium text-foreground transition-colors hover:bg-accent disabled:opacity-50"
                >
                  下一页
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
