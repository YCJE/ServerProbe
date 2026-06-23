import { useEffect, useState, useCallback } from 'react'
import {
  getAlertRules,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  testAlertRule,
  getNotifyChannels,
} from '@/lib/api'
import type { AlertRule, NotifyChannel } from '@/types'

/** 指标选项 */
const METRIC_OPTIONS = [
  { value: 'cpu_usage', label: 'CPU使用率' },
  { value: 'mem_usage', label: '内存使用率' },
  { value: 'disk_usage', label: '磁盘使用率' },
  { value: 'agent_offline', label: 'Agent离线' },
]

/** 操作符选项 */
const OPERATOR_OPTIONS = [
  { value: '>', label: '> 大于' },
  { value: '<', label: '< 小于' },
  { value: '=', label: '= 等于' },
]

/** 获取指标中文名 */
function getMetricLabel(metric: string): string {
  const opt = METRIC_OPTIONS.find((o) => o.value === metric)
  return opt ? opt.label : metric
}

/** 获取操作符显示 */
function getOperatorLabel(operator: string): string {
  const opt = OPERATOR_OPTIONS.find((o) => o.value === operator)
  return opt ? opt.value : operator
}

/** 获取通知渠道名称 */
function getChannelName(channels: NotifyChannel[], id: number): string {
  const ch = channels.find((c) => c.id === id)
  return ch ? ch.name : `#${id}`
}

/** 格式化持续时间 */
function formatDuration(seconds: number): string {
  if (seconds < 60) return `${seconds}秒`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟`
  return `${Math.floor(seconds / 3600)}小时`
}

/** 表单数据 */
interface FormData {
  name: string
  metric: string
  operator: string
  threshold: number
  duration: number
  enabled: boolean
  notify_channel_id: number
}

/** 空表单 */
const EMPTY_FORM: FormData = {
  name: '',
  metric: 'cpu_usage',
  operator: '>',
  threshold: 80,
  duration: 60,
  enabled: true,
  notify_channel_id: 0,
}

/** 告警规则管理页 */
export default function AlertManagement() {
  const [rules, setRules] = useState<AlertRule[]>([])
  const [channels, setChannels] = useState<NotifyChannel[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [testingId, setTestingId] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<AlertRule | null>(null)
  const [deleting, setDeleting] = useState(false)

  /** 加载数据 */
  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [rulesRes, channelsRes] = await Promise.all([
        getAlertRules(),
        getNotifyChannels(),
      ])
      setRules(rulesRes.rules || [])
      setChannels(channelsRes.channels || [])
    } catch (err) {
      console.error('加载数据失败:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  /** 打开新增弹窗 */
  const handleOpenAdd = () => {
    setEditingId(null)
    setForm({
      ...EMPTY_FORM,
      notify_channel_id: channels.length > 0 ? channels[0].id : 0,
    })
    setFormError('')
    setModalOpen(true)
  }

  /** 打开编辑弹窗 */
  const handleOpenEdit = (rule: AlertRule) => {
    setEditingId(rule.id)
    setForm({
      name: rule.name,
      metric: rule.metric,
      operator: rule.operator,
      threshold: rule.threshold,
      duration: rule.duration,
      enabled: rule.enabled,
      notify_channel_id: rule.notify_channel_id,
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
      setFormError('请输入规则名称')
      return
    }
    if (!form.notify_channel_id) {
      setFormError('请选择通知渠道')
      return
    }

    setSubmitting(true)
    try {
      const payload = {
        name: form.name.trim(),
        metric: form.metric,
        operator: form.operator,
        threshold: Number(form.threshold),
        duration: Number(form.duration),
        enabled: form.enabled,
        notify_channel_id: Number(form.notify_channel_id),
      }

      if (editingId !== null) {
        await updateAlertRule(editingId, payload)
      } else {
        await createAlertRule(payload)
      }

      handleCloseModal()
      await loadData()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  /** 删除规则 */
  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteAlertRule(deleteTarget.id)
      setDeleteTarget(null)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeleting(false)
    }
  }

  /** 测试规则 */
  const handleTest = async (rule: AlertRule) => {
    setTestingId(rule.id)
    try {
      await testAlertRule(rule.id)
      alert(`规则 "${rule.name}" 测试通知已发送`)
    } catch (err) {
      alert(err instanceof Error ? err.message : '测试失败')
    } finally {
      setTestingId(null)
    }
  }

  /** 快速切换启用状态 */
  const handleToggleEnabled = async (rule: AlertRule) => {
    try {
      await updateAlertRule(rule.id, { enabled: !rule.enabled })
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '更新失败')
    }
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">告警管理</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            配置监控指标告警规则，触发时通过通知渠道发送告警
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          disabled={channels.length === 0}
          title={channels.length === 0 ? '请先创建通知渠道' : ''}
          className="flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          添加规则
        </button>
      </div>

      {channels.length === 0 && (
        <div className="rounded-xl border border-warning/50 bg-warning/10 p-3 text-sm text-warning">
          尚未创建通知渠道，请先到「通知渠道」页面创建一个渠道，否则告警规则无法发送通知。
        </div>
      )}

      {/* 规则列表 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">告警规则列表 ({rules.length})</h2>
        </div>

        {loading && rules.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : rules.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无告警规则</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击"添加规则"创建第一个告警规则</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs text-muted-foreground">
                  <th className="px-4 py-2 font-medium">ID</th>
                  <th className="px-4 py-2 font-medium">名称</th>
                  <th className="px-4 py-2 font-medium">指标</th>
                  <th className="px-4 py-2 font-medium">操作符</th>
                  <th className="px-4 py-2 font-medium">阈值</th>
                  <th className="px-4 py-2 font-medium">持续时间</th>
                  <th className="px-4 py-2 font-medium">启用状态</th>
                  <th className="px-4 py-2 font-medium">通知渠道</th>
                  <th className="px-4 py-2 font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {rules.map((rule) => (
                  <tr key={rule.id} className="text-foreground">
                    <td className="px-4 py-3 text-muted-foreground">{rule.id}</td>
                    <td className="px-4 py-3 font-medium">{rule.name}</td>
                    <td className="px-4 py-3">
                      <span className="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">
                        {getMetricLabel(rule.metric)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground font-mono">
                      {getOperatorLabel(rule.operator)}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {rule.metric === 'agent_offline' ? '-' : `${rule.threshold}%`}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {formatDuration(rule.duration)}
                    </td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() => handleToggleEnabled(rule)}
                        className={`inline-flex items-center gap-1 text-xs ${
                          rule.enabled ? 'text-success' : 'text-muted-foreground'
                        }`}
                      >
                        <span
                          className={`inline-block h-1.5 w-1.5 rounded-full ${
                            rule.enabled ? 'bg-success' : 'bg-muted-foreground'
                          }`}
                        />
                        {rule.enabled ? '启用' : '禁用'}
                      </button>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {getChannelName(channels, rule.notify_channel_id)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => handleOpenEdit(rule)}
                          className="text-xs text-primary transition-colors hover:underline"
                        >
                          编辑
                        </button>
                        <button
                          onClick={() => handleTest(rule)}
                          disabled={testingId === rule.id}
                          className="text-xs text-success transition-colors hover:underline disabled:opacity-50"
                        >
                          {testingId === rule.id ? '测试中...' : '测试'}
                        </button>
                        <button
                          onClick={() => setDeleteTarget(rule)}
                          className="text-xs text-destructive transition-colors hover:underline"
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 新增/编辑弹窗 */}
      {modalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={handleCloseModal}>
          <div
            className="max-h-[90vh] w-full max-w-md overflow-y-auto rounded-xl border border-border bg-card p-4 shadow-lg sm:p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-base font-semibold text-foreground">
                {editingId !== null ? '编辑告警规则' : '添加告警规则'}
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
                  规则名称 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="例如：CPU 高负载告警"
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 指标 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  监控指标 <span className="text-destructive">*</span>
                </label>
                <select
                  value={form.metric}
                  onChange={(e) => setForm({ ...form, metric: e.target.value })}
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                >
                  {METRIC_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* 操作符 + 阈值 */}
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    操作符
                  </label>
                  <select
                    value={form.operator}
                    onChange={(e) => setForm({ ...form, operator: e.target.value })}
                    className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  >
                    {OPERATOR_OPTIONS.map((opt) => (
                      <option key={opt.value} value={opt.value}>
                        {opt.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">
                    阈值 {form.metric !== 'agent_offline' && '(%)'}
                  </label>
                  <input
                    type="number"
                    value={form.threshold}
                    onChange={(e) => setForm({ ...form, threshold: Number(e.target.value) })}
                    disabled={form.metric === 'agent_offline'}
                    className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-60"
                  />
                </div>
              </div>

              {/* 持续时间 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  持续时间 (秒)
                </label>
                <input
                  type="number"
                  value={form.duration}
                  onChange={(e) => setForm({ ...form, duration: Number(e.target.value) })}
                  min={1}
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
                <p className="mt-1 text-xs text-muted-foreground/70">
                  指标持续超过阈值多少秒后触发告警（agent_offline 指标表示离线多少秒后告警）
                </p>
              </div>

              {/* 通知渠道 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  通知渠道 <span className="text-destructive">*</span>
                </label>
                <select
                  value={form.notify_channel_id}
                  onChange={(e) => setForm({ ...form, notify_channel_id: Number(e.target.value) })}
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                >
                  <option value={0}>请选择...</option>
                  {channels.map((ch) => (
                    <option key={ch.id} value={ch.id}>
                      {ch.name} ({ch.type})
                    </option>
                  ))}
                </select>
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
                  启用该规则
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
                className="flex h-9 items-center rounded-lg border border-border px-4 text-sm text-foreground transition-colors hover:bg-accent"
              >
                取消
              </button>
              <button
                onClick={handleSubmit}
                disabled={submitting}
                className="flex h-9 items-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
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
            className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-lg"
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
                  确定删除告警规则 "{deleteTarget.name}"？此操作不可恢复。
                </p>
              </div>
            </div>
            <div className="flex items-center justify-end gap-2">
              <button
                onClick={() => setDeleteTarget(null)}
                disabled={deleting}
                className="flex h-9 items-center rounded-lg border border-border px-4 text-sm text-foreground transition-colors hover:bg-accent disabled:opacity-50"
              >
                取消
              </button>
              <button
                onClick={handleDelete}
                disabled={deleting}
                className="flex h-9 items-center rounded-lg bg-destructive px-4 text-sm font-medium text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-50"
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
