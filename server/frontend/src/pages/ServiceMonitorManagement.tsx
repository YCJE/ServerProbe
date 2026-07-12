import { useEffect, useState, useCallback, useRef } from 'react'
import {
  getServiceMonitors,
  createServiceMonitor,
  updateServiceMonitor,
  deleteServiceMonitor,
  testServiceMonitor,
  getServiceMonitorStatuses,
} from '@/lib/api'
import type { ServiceMonitor, ServiceStatusResult } from '@/types'

/** 状态轮询间隔（毫秒） */
const STATUS_POLL_INTERVAL = 30000

/** 格式化延迟 */
function formatLatency(ms: number): string {
  if (!ms || ms <= 0) return '-'
  if (ms < 1) return `${(ms * 1000).toFixed(0)}μs`
  if (ms < 1000) return `${ms.toFixed(0)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

/** 表单数据 */
interface FormData {
  name: string
  type: 'http' | 'tcp'
  target: string
  expected_status: number
  timeout: number
  interval: number
  enabled: boolean
}

/** 空表单 */
const EMPTY_FORM: FormData = {
  name: '',
  type: 'http',
  target: '',
  expected_status: 200,
  timeout: 5,
  interval: 60,
  enabled: true,
}

/** 服务监控管理页 */
export default function ServiceMonitorManagement() {
  const [monitors, setMonitors] = useState<ServiceMonitor[]>([])
  const [statuses, setStatuses] = useState<ServiceStatusResult[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [testingId, setTestingId] = useState<number | null>(null)
  const [testResult, setTestResult] = useState<{ id: number; text: string } | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ServiceMonitor | null>(null)
  const [deleting, setDeleting] = useState(false)

  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  /** 加载监控列表 */
  const loadMonitors = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await getServiceMonitors()
      setMonitors(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载数据失败')
    } finally {
      setLoading(false)
    }
  }, [])

  /** 加载状态列表 */
  const loadStatuses = useCallback(async () => {
    try {
      const data = await getServiceMonitorStatuses()
      setStatuses(data || [])
    } catch (err) {
      // 状态获取失败静默处理，不打断主列表
      console.error('加载服务状态失败:', err)
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
  const getStatus = (id: number): ServiceStatusResult | undefined => {
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
  const handleOpenEdit = (monitor: ServiceMonitor) => {
    setEditingId(monitor.id)
    setForm({
      name: monitor.name,
      type: monitor.type,
      target: monitor.target,
      expected_status: monitor.expected_status,
      timeout: monitor.timeout,
      interval: monitor.interval,
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

    if (!form.name.trim()) {
      setFormError('请输入监控名称')
      return
    }
    if (!form.target.trim()) {
      setFormError('请输入监控目标')
      return
    }
    if (form.timeout < 1 || form.timeout > 60) {
      setFormError('超时时间必须在 1-60 秒之间')
      return
    }
    if (form.interval < 10 || form.interval > 86400) {
      setFormError('检测间隔必须在 10-86400 秒之间')
      return
    }

    setSubmitting(true)
    try {
      const payload = {
        name: form.name.trim(),
        type: form.type,
        target: form.target.trim(),
        expected_status: form.type === 'http' ? Number(form.expected_status) : 0,
        timeout: Number(form.timeout),
        interval: Number(form.interval),
        enabled: form.enabled,
      }

      if (editingId !== null) {
        await updateServiceMonitor(editingId, payload)
      } else {
        await createServiceMonitor(payload)
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
  const handleTest = async (monitor: ServiceMonitor) => {
    setTestingId(monitor.id)
    setTestResult(null)
    try {
      const result = await testServiceMonitor(monitor.id)
      if (result.error) {
        setTestResult({ id: monitor.id, text: `测试失败: ${result.error}` })
      } else {
        setTestResult({
          id: monitor.id,
          text: `状态: ${result.status} | 延迟: ${formatLatency(result.latency)}`,
        })
      }
    } catch (err) {
      setTestResult({ id: monitor.id, text: err instanceof Error ? err.message : '测试失败' })
    } finally {
      setTestingId(null)
    }
  }

  /** 快速切换启用状态 */
  const handleToggleEnabled = async (monitor: ServiceMonitor) => {
    try {
      await updateServiceMonitor(monitor.id, { enabled: !monitor.enabled })
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
      await deleteServiceMonitor(deleteTarget.id)
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
          <h1 className="text-xl font-bold text-primary">服务监控</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            监控 HTTP/TCP 服务的可用性与响应延迟，状态每 30 秒自动刷新
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex h-10 items-center gap-1.5 rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
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
          <h2 className="text-sm font-semibold text-foreground">服务监控列表 ({monitors.length})</h2>
        </div>

        {loading && monitors.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : monitors.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无服务监控</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击"添加监控"创建第一个监控项</p>
          </div>
        ) : (
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-secondary/30">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">名称</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">类型</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">目标</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">状态</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">延迟</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">启用</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {monitors.map((monitor) => {
                  const status = getStatus(monitor.id)
                  const isUp = status?.last_status === 'up'
                  const isDown = status?.last_status === 'down'
                  return (
                    <tr key={monitor.id} className="text-foreground transition-colors hover:bg-muted/50">
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">{monitor.id}</td>
                      <td className="px-3 py-3 font-medium">{monitor.name}</td>
                      <td className="px-3 py-3">
                        <span className="badge-pill badge-primary">{monitor.type.toUpperCase()}</span>
                      </td>
                      <td className="px-3 py-3 font-mono text-xs text-muted-foreground">
                        {monitor.target}
                      </td>
                      <td className="px-3 py-3">
                        {status ? (
                          <span className="flex items-center gap-1.5">
                            <span
                              className={`inline-block h-2 w-2 rounded-full ${
                                isUp ? 'bg-emerald-500' : isDown ? 'bg-red-500' : 'bg-zinc-400'
                              }`}
                            />
                            <span
                              className={
                                isUp
                                  ? 'text-emerald-500'
                                  : isDown
                                  ? 'text-red-500'
                                  : 'text-muted-foreground'
                              }
                            >
                              {isUp ? 'UP' : isDown ? 'DOWN' : status.last_status || '未知'}
                            </span>
                          </span>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </td>
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">
                        {status ? formatLatency(status.last_latency) : '-'}
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
                {editingId !== null ? '编辑服务监控' : '添加服务监控'}
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
              {/* 名称 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  监控名称 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="例如：官网首页"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 类型 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  监控类型 <span className="text-destructive">*</span>
                </label>
                <select
                  value={form.type}
                  onChange={(e) => setForm({ ...form, type: e.target.value as 'http' | 'tcp' })}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                >
                  <option value="http">HTTP</option>
                  <option value="tcp">TCP</option>
                </select>
              </div>

              {/* 目标 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  监控目标 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.target}
                  onChange={(e) => setForm({ ...form, target: e.target.value })}
                  placeholder={form.type === 'http' ? 'https://example.com/health' : 'example.com:3306'}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
                <p className="mt-1 text-xs text-muted-foreground/70">
                  {form.type === 'http' ? '完整的 HTTP(S) URL' : 'host:port 格式'}
                </p>
              </div>

              {/* 期望状态码 (HTTP only) */}
              {form.type === 'http' && (
                <div>
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    期望状态码
                  </label>
                  <input
                    type="number"
                    value={form.expected_status}
                    onChange={(e) => setForm({ ...form, expected_status: Number(e.target.value) })}
                    min={100}
                    max={599}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                  <p className="mt-1 text-xs text-muted-foreground/70">
                    期望返回的 HTTP 状态码（如 200）
                  </p>
                </div>
              )}

              {/* 超时 + 间隔 */}
              <div className="flex flex-col gap-3 sm:flex-row">
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    超时时间 (秒)
                  </label>
                  <input
                    type="number"
                    value={form.timeout}
                    onChange={(e) => setForm({ ...form, timeout: Number(e.target.value) })}
                    min={1}
                    max={60}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    检测间隔 (秒)
                  </label>
                  <input
                    type="number"
                    value={form.interval}
                    onChange={(e) => setForm({ ...form, interval: Number(e.target.value) })}
                    min={10}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
              </div>

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
                className="flex h-10 items-center rounded-xl border border-border bg-secondary px-4 text-sm font-semibold text-foreground transition-colors hover:bg-accent"
              >
                取消
              </button>
              <button
                onClick={handleSubmit}
                disabled={submitting}
                className="flex h-10 items-center rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
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
                  确定删除服务监控 "{deleteTarget.name}"？此操作不可恢复。
                </p>
              </div>
            </div>
            <div className="flex items-center justify-end gap-2">
              <button
                onClick={() => setDeleteTarget(null)}
                disabled={deleting}
                className="flex h-10 items-center rounded-xl border border-border bg-secondary px-4 text-sm font-semibold text-foreground transition-colors hover:bg-accent disabled:opacity-50"
              >
                取消
              </button>
              <button
                onClick={handleDelete}
                disabled={deleting}
                className="flex h-10 items-center rounded-xl bg-destructive px-4 text-sm font-semibold text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-50"
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
