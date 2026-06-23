import { useEffect, useState, useCallback } from 'react'
import {
  getPingTargets,
  createPingTarget,
  updatePingTarget,
  deletePingTarget,
  getPingInterval,
  setPingInterval,
} from '@/lib/api'
import type { PingTarget } from '@/lib/api'

/** 探测方式选项 */
const METHOD_OPTIONS = [
  { value: 'icmp', label: 'ICMP' },
  { value: 'tcp', label: 'TCP' },
  { value: 'http', label: 'HTTP' },
]

/** 探测间隔选项（秒） */
const INTERVAL_OPTIONS = [
  { value: 1, label: '1 秒' },
  { value: 2, label: '2 秒' },
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 30, label: '30 秒' },
  { value: 60, label: '60 秒' },
  { value: 120, label: '2 分钟' },
  { value: 300, label: '5 分钟' },
  { value: 600, label: '10 分钟' },
]

/** 表单数据 */
interface FormData {
  name: string
  target: string
  method: string
  enabled: boolean
  sort_order: number
}

/** 空表单 */
const EMPTY_FORM: FormData = {
  name: '',
  target: '',
  method: 'icmp',
  enabled: true,
  sort_order: 0,
}

/** 探测目标管理页 */
export default function PingTargets() {
  const [targets, setTargets] = useState<PingTarget[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  // 探测间隔状态
  const [probeInterval, setProbeInterval] = useState<number>(60)
  const [intervalLoading, setIntervalLoading] = useState(false)
  const [intervalSaving, setIntervalSaving] = useState(false)
  const [intervalMessage, setIntervalMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  /** 加载探测间隔 */
  const loadInterval = useCallback(async () => {
    setIntervalLoading(true)
    try {
      const res = await getPingInterval()
      setProbeInterval(res.interval)
    } catch (err) {
      console.error('加载探测间隔失败:', err)
    } finally {
      setIntervalLoading(false)
    }
  }, [])

  /** 加载探测目标列表 */
  const loadTargets = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getPingTargets()
      setTargets(res.targets || [])
    } catch (err) {
      console.error('加载探测目标失败:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadTargets()
    loadInterval()
  }, [loadTargets, loadInterval])

  /** 保存探测间隔 */
  const handleSaveInterval = async () => {
    setIntervalSaving(true)
    setIntervalMessage(null)
    try {
      await setPingInterval(probeInterval)
      setIntervalMessage({ type: 'success', text: '探测间隔已保存' })
      setTimeout(() => setIntervalMessage(null), 3000)
    } catch (err) {
      setIntervalMessage({ type: 'error', text: err instanceof Error ? err.message : '保存失败' })
    } finally {
      setIntervalSaving(false)
    }
  }

  /** 打开新增弹窗 */
  const handleOpenAdd = () => {
    setEditingId(null)
    setForm(EMPTY_FORM)
    setFormError('')
    setModalOpen(true)
  }

  /** 打开编辑弹窗 */
  const handleOpenEdit = (target: PingTarget) => {
    setEditingId(target.id)
    setForm({
      name: target.name,
      target: target.target,
      method: target.method || 'icmp',
      enabled: target.enabled,
      sort_order: target.sort_order || 0,
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

  /** 提交表单（新增/编辑） */
  const handleSubmit = async () => {
    setFormError('')

    if (!form.name.trim()) {
      setFormError('请输入目标名称')
      return
    }
    if (!form.target.trim()) {
      setFormError('请输入目标地址')
      return
    }

    setSubmitting(true)
    try {
      const payload = {
        name: form.name.trim(),
        target: form.target.trim(),
        method: form.method,
        enabled: form.enabled,
        sort_order: form.sort_order,
      }

      if (editingId !== null) {
        await updatePingTarget(editingId, payload)
      } else {
        await createPingTarget(payload)
      }

      handleCloseModal()
      await loadTargets()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  /** 删除目标 */
  const handleDelete = async (target: PingTarget) => {
    if (!confirm(`确定删除探测目标 "${target.name}"？此操作不可恢复。`)) return
    try {
      await deletePingTarget(target.id)
      await loadTargets()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    }
  }

  /** 快速切换启用状态 */
  const handleToggleEnabled = async (target: PingTarget) => {
    try {
      await updatePingTarget(target.id, { enabled: !target.enabled })
      await loadTargets()
    } catch (err) {
      alert(err instanceof Error ? err.message : '更新失败')
    }
  }

  /** 获取探测方式的显示标签 */
  const getMethodLabel = (method: string) => {
    const opt = METHOD_OPTIONS.find((o) => o.value === method)
    return opt ? opt.label : method.toUpperCase()
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">探测目标管理</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            管理三网延迟探测目标，配置 ICMP / TCP / HTTP 探测方式
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          添加目标
        </button>
      </div>

      {/* 探测间隔设置 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">探测间隔设置</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            设置 Ping 探测的执行频率，间隔越小数据越实时但消耗资源越多
          </p>
        </div>
        <div className="p-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <div className="flex-1">
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                探测间隔
              </label>
              {intervalLoading ? (
                <div className="flex h-9 items-center gap-2 text-sm text-muted-foreground">
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  加载中...
                </div>
              ) : (
                <select
                  value={probeInterval}
                  onChange={(e) => setProbeInterval(Number(e.target.value))}
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                >
                  {INTERVAL_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              )}
            </div>
            <button
              onClick={handleSaveInterval}
              disabled={intervalSaving || intervalLoading}
              className="flex h-9 shrink-0 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {intervalSaving ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent" />
                  保存中...
                </>
              ) : (
                '保存'
              )}
            </button>
          </div>
          {intervalMessage && (
            <p className={`mt-2 text-xs ${intervalMessage.type === 'success' ? 'text-success' : 'text-destructive'}`}>
              {intervalMessage.text}
            </p>
          )}
          <p className="mt-2 text-xs text-muted-foreground">
            当前探测间隔：<span className="font-medium text-foreground">{probeInterval} 秒</span>
          </p>
        </div>
      </div>

      {/* 目标列表 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">探测目标列表 ({targets.length})</h2>
        </div>

        {loading && targets.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : targets.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无探测目标</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击"添加目标"创建第一个探测目标</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs text-muted-foreground">
                  <th className="px-4 py-2 font-medium">ID</th>
                  <th className="px-4 py-2 font-medium">名称</th>
                  <th className="px-4 py-2 font-medium">目标地址</th>
                  <th className="px-4 py-2 font-medium">探测方式</th>
                  <th className="px-4 py-2 font-medium">启用状态</th>
                  <th className="px-4 py-2 font-medium">排序</th>
                  <th className="px-4 py-2 font-medium">创建时间</th>
                  <th className="px-4 py-2 font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {targets.map((target) => (
                  <tr key={target.id} className="text-foreground">
                    <td className="px-4 py-3 text-muted-foreground">{target.id}</td>
                    <td className="px-4 py-3 font-medium">{target.name}</td>
                    <td className="px-4 py-3 text-muted-foreground font-mono text-xs">{target.target}</td>
                    <td className="px-4 py-3">
                      <span className="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">
                        {getMethodLabel(target.method)}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() => handleToggleEnabled(target)}
                        className={`inline-flex items-center gap-1 text-xs ${
                          target.enabled ? 'text-success' : 'text-muted-foreground'
                        }`}
                      >
                        <span
                          className={`inline-block h-1.5 w-1.5 rounded-full ${
                            target.enabled ? 'bg-success' : 'bg-muted-foreground'
                          }`}
                        />
                        {target.enabled ? '启用' : '禁用'}
                      </button>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">{target.sort_order}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {target.created_at ? new Date(target.created_at).toLocaleString('zh-CN') : '-'}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => handleOpenEdit(target)}
                          className="text-xs text-primary transition-colors hover:underline"
                        >
                          编辑
                        </button>
                        <button
                          onClick={() => handleDelete(target)}
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
                {editingId !== null ? '编辑探测目标' : '添加探测目标'}
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
                  目标名称 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="例如：电信"
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 目标地址 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  目标地址 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.target}
                  onChange={(e) => setForm({ ...form, target: e.target.value })}
                  placeholder="例如：223.5.5.5 或 https://example.com"
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 探测方式 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">探测方式</label>
                <select
                  value={form.method}
                  onChange={(e) => setForm({ ...form, method: e.target.value })}
                  className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                >
                  {METHOD_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* 排序 + 启用 */}
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                <div className="flex-1">
                  <label className="mb-1 block text-xs font-medium text-muted-foreground">排序</label>
                  <input
                    type="number"
                    value={form.sort_order}
                    onChange={(e) => setForm({ ...form, sort_order: parseInt(e.target.value) || 0 })}
                    className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
                <div className="flex items-center gap-2 pb-2">
                  <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
                    <input
                      type="checkbox"
                      checked={form.enabled}
                      onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                      className="h-4 w-4 rounded border-border"
                    />
                    启用
                  </label>
                </div>
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
    </div>
  )
}
