import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { getSessions, revokeSession, revokeOtherSessions, type AdminSession } from '@/lib/api'
import { useServerStore } from '@/store/useServerStore'
import Skeleton from '@/components/Skeleton'
import EmptyState from '@/components/EmptyState'
import { usePageTitle } from '@/hooks/usePageTitle'

/** RFC3339 → 本地时间显示 */
function formatTime(timeStr: string): string {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

/** User-Agent 简化显示（平台 · 浏览器） */
function shortUA(ua: string): string {
  if (!ua) return '-'
  const platform = ua.match(/\((.*?)\)/)
  const browser = ua.match(/(Firefox|Edg|Chrome|Safari|curl)\/[\d.]+/g)
  const parts: string[] = []
  if (platform) parts.push(platform[1].split(';')[0].trim())
  if (browser) parts.push(browser[browser.length - 1])
  return parts.join(' · ') || ua.slice(0, 40)
}

/** 会话管理页（P2：活跃会话列表 + 单踢/全踢，被踢会话立即 401） */
export default function SessionManagement() {
  usePageTitle('会话管理')
  const navigate = useNavigate()
  const logout = useServerStore((s) => s.logout)
  const [sessions, setSessions] = useState<AdminSession[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [msg, setMsg] = useState('')
  const [revoking, setRevoking] = useState<string | null>(null)
  const [revokingOthers, setRevokingOthers] = useState(false)

  const loadSessions = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await getSessions()
      setSessions(data.sessions || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载会话列表失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadSessions()
  }, [loadSessions])

  /** 撤销单个会话；撤销当前会话等同于登出 */
  const handleRevoke = async (session: AdminSession) => {
    if (session.current) {
      if (!confirm('注销当前会话将退出登录，确定继续？')) return
      setRevoking(session.session_id)
      try {
        await logout()
        navigate('/login', { replace: true })
      } catch {
        // 登出失败时刷新列表兜底
        await loadSessions()
        setRevoking(null)
      }
      return
    }
    if (!confirm('确定注销该会话？对应设备的登录状态将立即失效。')) return
    setRevoking(session.session_id)
    setMsg('')
    setError('')
    try {
      const res = await revokeSession(session.session_id)
      setMsg(res.message || '会话已注销')
      await loadSessions()
    } catch (err) {
      setError(err instanceof Error ? err.message : '注销会话失败')
    } finally {
      setRevoking(null)
    }
  }

  /** 撤销除当前会话外的全部会话 */
  const handleRevokeOthers = async () => {
    const others = sessions.filter((s) => !s.current && !s.revoked)
    if (others.length === 0) {
      setMsg('没有其他活跃会话')
      return
    }
    if (!confirm(`确定注销其他 ${others.length} 个会话？对应设备的登录状态将立即失效。`)) return
    setRevokingOthers(true)
    setMsg('')
    setError('')
    try {
      const res = await revokeOtherSessions()
      setMsg(res.message || `已注销其他 ${res.revoked} 个会话`)
      await loadSessions()
    } catch (err) {
      setError(err instanceof Error ? err.message : '注销其他会话失败')
    } finally {
      setRevokingOthers(false)
    }
  }

  const activeCount = sessions.filter((s) => !s.revoked).length
  const otherActiveCount = sessions.filter((s) => !s.current && !s.revoked).length

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-foreground">会话管理</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            当前账户的全部登录会话（{activeCount} 个活跃），可注销可疑设备；TOTP 变更后其他会话自动失效
          </p>
        </div>
        <button
          onClick={handleRevokeOthers}
          disabled={revokingOthers || otherActiveCount === 0}
          className="flex h-10 items-center rounded-md border border-destructive/40 bg-destructive/10 px-4 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/20 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {revokingOthers ? '注销中…' : `注销其他会话 (${otherActiveCount})`}
        </button>
      </div>

      {msg && (
        <div className="rounded-md border border-success/30 bg-success/10 px-4 py-2.5 text-sm text-success">
          {msg}
        </div>
      )}
      {error && (
        <div className="rounded-md border border-dashed border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* 会话列表 */}
      <div className="card-soft overflow-hidden">
        <div className="border-b border-dashed border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">会话列表</h2>
        </div>

        {loading && sessions.length === 0 ? (
          <Skeleton variant="table" />
        ) : sessions.length === 0 ? (
          <EmptyState
            icon={
              <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 7a4 4 0 11-8 0 4 4 0 018 0zM5 21v-1a7 7 0 0114 0v1" />
              </svg>
            }
            title="暂无会话记录"
            description="登录后此处会显示全部活跃会话"
          />
        ) : (
          <div className="table-shell">
            <table className="w-full min-w-[760px] text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">状态</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">IP 地址</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">设备 / 浏览器</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">登录时间</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">最后活跃</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">过期时间</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {sessions.map((session) => (
                  <tr key={session.id} className="text-foreground transition-colors hover:bg-muted/50">
                    <td className="px-3 py-3">
                      {session.revoked ? (
                        <span className="badge-pill bg-muted text-muted-foreground">已失效</span>
                      ) : session.current ? (
                        <span className="badge-pill badge-success">当前会话</span>
                      ) : (
                        <span className="badge-pill badge-primary">活跃</span>
                      )}
                    </td>
                    <td className="px-3 py-3 font-mono text-xs">{session.ip || '-'}</td>
                    <td className="px-3 py-3 max-w-xs truncate text-muted-foreground" title={session.user_agent}>
                      {shortUA(session.user_agent)}
                    </td>
                    <td className="px-3 py-3 tabular-nums text-muted-foreground">{formatTime(session.created_at)}</td>
                    <td className="px-3 py-3 tabular-nums text-muted-foreground">{formatTime(session.last_seen_at)}</td>
                    <td className="px-3 py-3 tabular-nums text-muted-foreground">{formatTime(session.expires_at)}</td>
                    <td className="px-3 py-3">
                      {!session.revoked && (
                        <button
                          onClick={() => handleRevoke(session)}
                          disabled={revoking === session.session_id}
                          className="text-xs font-medium text-destructive transition-colors hover:underline disabled:opacity-50"
                        >
                          {revoking === session.session_id ? '注销中…' : session.current ? '退出登录' : '注销'}
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <p className="text-xs text-muted-foreground/70">
        会话记录保留 7 天后自动清理；被注销的会话在下一次请求时立即返回 401 并清除登录 Cookie。
      </p>
    </div>
  )
}
