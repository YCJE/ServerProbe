import { useEffect, useState, useCallback, useRef } from 'react'
import {
  getSSLMonitors,
  createSSLMonitor,
  updateSSLMonitor,
  deleteSSLMonitor,
  testSSLMonitor,
  getSSLMonitorStatuses,
} from '@/lib/api'
import type { SSLCertMonitor, SSLCertStatusResult } from '@/types'
import Skeleton from '@/components/Skeleton'
import EmptyState from '@/components/EmptyState'
import { usePageTitle } from '@/hooks/usePageTitle'

/** 状态轮询间隔（毫秒） */
const STATUS_POLL_INTERVAL = 60000

/** 根据剩余天数返回颜色类名 */
function getDaysColor(days: number): string {
  if (days < 7) return 'text-red-500'
  if (days <= 30) return 'text-amber-500'
  return 'text-emerald-500'
}

/** 根据剩余天数返回圆点颜色类名 */
function getDaysDotColor(days: number): string {
  if (days < 7) return 'bg-red-500'
  if (days <= 30) return 'bg-amber-500'
  return 'bg-emerald-500'
}

/** 检查 expiryDate 是否为有效日期（排除 Go time.Time 零值 "0001-01-01T..."） */
function isValidExpiryDate(date: string | undefined): boolean {
  if (!date) return false
  return !date.startsWith('0001-')
}

/** 格式化 ISO 日期为 YYYY-MM-DD */
function formatExpiryDate(date: string | undefined): string {
  if (!isValidExpiryDate(date)) return '-'
  // ISO 格式 "2026-08-13T00:00:00Z" -> "2026-08-13"
  return date!.substring(0, 10)
}

/** 表单数据 */
interface FormData {
  domain: string
  port: number
  alert_days: number
  enabled: boolean
}

/** 空表单 */
const EMPTY_FORM: FormData = {
  domain: '',
  port: 443,
  alert_days: 30,
  enabled: true,
}

/** SSL 证书监控管理页 */
export default function SSLMonitorManagement() {
  usePageTitle('SSL 监控')
  const [monitors, setMonitors] = useState<SSLCertMonitor[]>([])
  const [statuses, setStatuses] = useState<SSLCertStatusResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [testingId, setTestingId] = useState<number | null>(null)
  const [testResult, setTestResult] = useState<{ id: number; text: string } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<SSLCertMonitor | null>(null)
  const [deleting, setDeleting] = useState(false)

  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  /** 加载监控列表 */
  const loadMonitors = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await getSSLMonitors()
      setMonitors(data.monitors || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [])

  /** 加载状态列表 */
  const loadStatuses = useCallback(async () => {
    try {
      const data = await getSSLMonitorStatuses()
      setStatuses(data.statuses || [])
    } catch (err) {
      console.error('加载 SSL 状态失败:', err)
    }
  }, [])

  useEffect(() => {
    loadMonitors()
    loadStatuses()
    pollTimerRef.current = setInterval(loadStatuses, STATUS_POLL_INTERVAL)
    return () => {
      if (pollTimerRef.current) clearInterval(pollTimerRef.current)
    }
  }, [loadMonitors, loadStatuses])

  /** 获取某监控的实时状态 */
  const getStatus = (id: number): SSLCertStatusResult | undefined => {
    return statuses.find((s) => s.id === id)
  }

  /** 打开新增弹窗 */
  const handleOpenAdd = () => {
    setEditingId(null)
    setForm(EMPTY_FORM)
    setFormError('')
    setModalOpen(true)
  }

  /** 打开编辑弹窗 */
  const handleOpenEdit = (monitor: SSLCertMonitor) => {
    setEditingId(monitor.id)
    setForm({
      domain: monitor.domain,
      port: monitor.port,
      alert_days: monitor.alert_days,
      enabled: monitor.enabled,
    })
    setFormError('')
    setModalOpen(true)
  }

  /** 关闭弹窗 */
  const handleCloseModal = () => {
    setModalOpen(false)
    setEditingId(null)
    setForm(EMPTY_FORM)
    setFormError('')
  }

  /** 提交表单 */
  const handleSubmit = async () => {
    setFormError('')

    if (!form.domain.trim()) {
      setFormError('请输入域名')
      return
    }
    if (form.port < 1 || form.port > 65535) {
      setFormError('端口必须在 1-65535 之间')
      return
    }
    if (form.alert_days < 1 || form.alert_days > 365) {
      setFormError('告警天数必须在 1-365 之间')
      return
    }

    setSubmitting(true)
    try {
      const payload = {
        domain: form.domain.trim(),
        port: Number(form.port),
        alert_days: Number(form.alert_days),
        enabled: form.enabled,
      }

      if (editingId !== null) {
        await updateSSLMonitor(editingId, payload)
      } else {
        await createSSLMonitor(payload)
      }

      handleCloseModal()
      await loadMonitors()
      await loadStatuses()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  /** 测试监控 */
  const handleTest = async (monitor: SSLCertMonitor) => {
    setTestingId(monitor.id)
    setTestResult(null)
    try {
      const result = await testSSLMonitor(monitor.id)
      if (result.error) {
        setTestResult({ id: monitor.id, text: `测试失败: ${result.error}` })
      } else {
        setTestResult({
          id: monitor.id,
          text: `到期日期: ${formatExpiryDate(result.expiry_date)} | 剩余: ${result.remaining_days} 天`,
        })
      }
    } catch (err) {
      setTestResult({ id: monitor.id, text: err instanceof Error ? err.message : '测试失败' })
    } finally {
      setTestingId(null)
    }
  }

  /** 快速切换启用状态 */
  const handleToggleEnabled = async (monitor: SSLCertMonitor) => {
    try {
      await updateSSLMonitor(monitor.id, { enabled: !monitor.enabled })
      await loadMonitors()
    } catch (err) {
      alert(err instanceof Error ? err.message : '更新失败')
    }
  }

  /** 删除监控 */
  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteSSLMonitor(deleteTarget.id)
      setDeleteTarget(null)
      await loadMonitors()
      await loadStatuses()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-foreground">SSL 证书监控</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            监控 SSL/TLS 证书到期时间，状态每 60 秒自动刷新
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex h-10 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          添加监控
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-dashed border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* 监控列表 */}
      <div className="card-soft overflow-hidden">
        <div className="border-b border-dashed border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">SSL 证书监控列表 ({monitors.length})</h2>
        </div>

        {loading && monitors.length === 0 ? (
          <Skeleton variant="table" />
        ) : monitors.length === 0 ? (
          <EmptyState
            icon={
              <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
            }
            title="暂无 SSL 证书监控"
            description={'点击"添加监控"创建第一个监控项'}
          />
        ) : (
          <div className="table-shell">
            <table className="w-full min-w-[720px] text-sm">
              <thead>
                <tr className="border-b border-border">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">域名:端口</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">剩余天数</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">到期日期</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">告警阈值</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">启用</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {monitors.map((monitor) => {
                  const status = getStatus(monitor.id)
                  const remainingDays = status?.last_remaining_days ?? monitor.last_remaining_days ?? 0
                  const expiryDate = status?.last_expiry_date ?? monitor.last_expiry_date
                  const hasData = isValidExpiryDate(expiryDate)
                  return (
                    <tr key={monitor.id} className="text-foreground transition-colors hover:bg-muted/50">
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">{monitor.id}</td>
                      <td className="px-3 py-3 font-mono text-xs">
                        {monitor.domain}:{monitor.port}
                      </td>
                      <td className="px-3 py-3">
                        {hasData ? (
                          <span className="flex items-center gap-1.5">
                            <span className={`inline-block h-2 w-2 rounded-full ${getDaysDotColor(remainingDays)}`} />
                            <span className={`font-medium tabular-nums ${getDaysColor(remainingDays)}`}>
                              {remainingDays} 天
                            </span>
                          </span>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </td>
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">
                        {hasData ? formatExpiryDate(expiryDate) : '-'}
                      </td>
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">
                        {monitor.alert_days} 天
                      </td>
                      <td className="px-3 py-3">
                        <button
                          onClick={() => handleToggleEnabled(monitor)}
                          className={`badge-pill ${monitor.enabled ? 'badge-success' : 'badge-warning'}`}
                        >
                          <span
                            className={`inline-block h-1.5 w-1.5 rounded-full ${
                              monitor.enabled ? 'bg-success' : 'bg-muted-foreground'
                            }`}
                          />
                          {monitor.enabled ? '启用' : '禁用'}
                        </button>
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => handleOpenEdit(monitor)}
                            className="text-xs font-medium text-primary transition-colors hover:underline"
                          >
                            编辑
                          </button>
                          <button
                            onClick={() => handleTest(monitor)}
                            disabled={testingId === monitor.id}
                            className="text-xs font-medium text-success transition-colors hover:underline disabled:opacity-50"
                          >
                            {testingId === monitor.id ? '测试中...' : '测试'}
                          </button>
                          <button
                            onClick={() => setDeleteTarget(monitor)}
                            className="text-xs font-medium text-destructive transition-colors hover:underline"
                          >
                            删除
                          </button>
                        </div>
                        {testResult && testResult.id === monitor.id && (
                          <p className="mt-1 text-xs text-muted-foreground">{testResult.text}</p>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 新增/编辑弹窗 */}
      {modalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={handleCloseModal}>
          <div
            className="max-h-[90vh] w-full max-w-md overflow-y-auto card-soft p-4 sm:p-6 scrollbar-thin"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-base font-semibold text-foreground">
                {editingId !== null ? '编辑 SSL 证书监控' : '添加 SSL 证书监控'}
              </h3>
              <button
                onClick={handleCloseModal}
                className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
              >
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <div className="space-y-4">
              {/* 域名 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  域名 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.domain}
                  onChange={(e) => setForm({ ...form, domain: e.target.value })}
                  placeholder="例如：example.com"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
                <p className="mt-1 text-xs text-muted-foreground/70">
                  不含协议和端口，仅域名
                </p>
              </div>

              {/* 端口 + 告警天数 */}
              <div className="flex flex-col gap-3 sm:flex-row">
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    端口
                  </label>
                  <input
                    type="number"
                    value={form.port}
                    onChange={(e) => setForm({ ...form, port: Number(e.target.value) })}
                    min={1}
                    max={65535}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    告警阈值 (天)
                  </label>
                  <input
                    type="number"
                    value={form.alert_days}
                    onChange={(e) => setForm({ ...form, alert_days: Number(e.target.value) })}
                    min={1}
                    max={365}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
              </div>
              <p className="-mt-2 text-xs text-muted-foreground/70">
                剩余天数低于此阈值时触发告警
              </p>

              {/* 启用开关 */}
              <div className="flex items-center gap-2 pb-1">
                <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                    className="h-4 w-4 rounded border-border"
                  />
                  启用该监控
                </label>
              </div>

              {formError && (
                <p className="text-xs text-destructive">{formError}</p>
              )}
            </div>

            {/* 操作按钮 */}
            <div className="mt-6 flex items-center justify-end gap-2">
              <button
                onClick={handleCloseModal}
                className="flex h-10 items-center rounded-md border border-border bg-card px-4 text-sm font-semibold text-foreground transition-colors hover:bg-accent"
              >
                取消
              </button>
              <button
                onClick={handleSubmit}
                disabled={submitting}
                className="flex h-10 items-center rounded-md bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
              >
                {submitting ? '提交中...' : editingId !== null ? '保存' : '添加'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 删除确认弹窗 */}
      {deleteTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => !deleting && setDeleteTarget(null)}>
          <div
            className="w-full max-w-sm card-soft p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-destructive/10">
                <svg className="h-5 w-5 text-destructive" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <div>
                <h3 className="text-base font-semibold text-foreground">确认删除</h3>
                <p className="mt-0.5 text-sm text-muted-foreground">
                  确定删除 SSL 证书监控 "{deleteTarget.domain}:{deleteTarget.port}"？此操作不可恢复。
                </p>
              </div>
            </div>
            <div className="flex items-center justify-end gap-2">
              <button
                onClick={() => setDeleteTarget(null)}
                disabled={deleting}
                className="flex h-10 items-center rounded-md border border-border bg-card px-4 text-sm font-semibold text-foreground transition-colors hover:bg-accent disabled:opacity-50"
              >
                取消
              </button>
              <button
                onClick={handleDelete}
                disabled={deleting}
                className="flex h-10 items-center rounded-md bg-destructive px-4 text-sm font-semibold text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-50"
              >
                {deleting ? '删除中...' : '删除'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
