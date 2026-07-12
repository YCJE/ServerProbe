import { useEffect, useState, useCallback } from 'react'
import {
  getSharePages,
  createSharePage,
  updateSharePage,
  deleteSharePage,
} from '@/lib/api'
import type { SharePage } from '@/types'

/** 表单数据 */
interface FormData {
  title: string
  description: string
  agent_ids: string
  enabled: boolean
  sort_order: number
}

/** 空表单 */
const EMPTY_FORM: FormData = {
  title: '',
  description: '',
  agent_ids: '',
  enabled: true,
  sort_order: 0,
}

/** 分享页管理页 */
export default function SharePageManagement() {
  const [pages, setPages] = useState<SharePage[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<SharePage | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [copiedId, setCopiedId] = useState<number | null>(null)

  /** 加载数据 */
  const loadData = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await getSharePages()
      setPages(data || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载数据失败')
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
    setForm(EMPTY_FORM)
    setFormError('')
    setModalOpen(true)
  }

  /** 打开编辑弹窗 */
  const handleOpenEdit = (page: SharePage) => {
    setEditingId(page.id)
    setForm({
      title: page.title,
      description: page.description,
      agent_ids: page.agent_ids,
      enabled: page.enabled,
      sort_order: page.sort_order,
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

    if (!form.title.trim()) {
      setFormError('请输入标题')
      return
    }

    setSubmitting(true)
    try {
      const payload = {
        title: form.title.trim(),
        description: form.description.trim(),
        agent_ids: form.agent_ids.trim(),
        enabled: form.enabled,
        sort_order: Number(form.sort_order),
      }

      if (editingId !== null) {
        await updateSharePage(editingId, payload)
      } else {
        await createSharePage(payload)
      }

      handleCloseModal()
      await loadData()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  /** 快速切换启用状态 */
  const handleToggleEnabled = async (page: SharePage) => {
    try {
      await updateSharePage(page.id, { enabled: !page.enabled })
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '更新失败')
    }
  }

  /** 复制分享链接 */
  const handleCopyLink = async (page: SharePage) => {
    const url = `${window.location.origin}/share/${page.share_id}`
    try {
      await navigator.clipboard.writeText(url)
      setCopiedId(page.id)
      setTimeout(() => setCopiedId(null), 2000)
    } catch {
      // 降级方案
      const textArea = document.createElement('textarea')
      textArea.value = url
      document.body.appendChild(textArea)
      textArea.select()
      try {
        document.execCommand('copy')
        setCopiedId(page.id)
        setTimeout(() => setCopiedId(null), 2000)
      } catch {
        alert('复制失败，请手动复制: ' + url)
      }
      document.body.removeChild(textArea)
    }
  }

  /** 删除分享页 */
  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteSharePage(deleteTarget.id)
      setDeleteTarget(null)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeleting(false)
    }
  }

  /** 解析 agent_ids 为标签数组 */
  const parseAgentIds = (agentIds: string): string[] => {
    if (!agentIds) return []
    return agentIds.split(',').map((s) => s.trim()).filter(Boolean)
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-primary">分享页</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            创建自定义分享页，选择展示的 Agent 并生成公开访问链接
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex h-10 items-center gap-1.5 rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          添加分享页
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-dashed border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* 分享页列表 */}
      <div className="card-soft overflow-hidden">
        <div className="border-b border-dashed border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">分享页列表 ({pages.length})</h2>
        </div>

        {loading && pages.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : pages.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无分享页</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击"添加分享页"创建第一个分享页</p>
          </div>
        ) : (
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-secondary/30">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">标题</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">描述</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">分享 ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">Agent ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">排序</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">启用</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {pages.map((page) => {
                  const agentIds = parseAgentIds(page.agent_ids)
                  return (
                    <tr key={page.id} className="text-foreground transition-colors hover:bg-muted/50">
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">{page.id}</td>
                      <td className="px-3 py-3 font-medium">{page.title}</td>
                      <td className="px-3 py-3 max-w-xs truncate text-muted-foreground">
                        {page.description || '-'}
                      </td>
                      <td className="px-3 py-3">
                        <code className="rounded bg-secondary px-1.5 py-0.5 text-xs font-mono text-foreground">
                          {page.share_id}
                        </code>
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex flex-wrap gap-1">
                          {agentIds.length > 0 ? (
                            agentIds.map((id, idx) => (
                              <span key={idx} className="badge-pill badge-primary">
                                #{id}
                              </span>
                            ))
                          ) : (
                            <span className="text-xs text-muted-foreground">全部</span>
                          )}
                        </div>
                      </td>
                      <td className="px-3 py-3 tabular-nums text-muted-foreground">
                        {page.sort_order}
                      </td>
                      <td className="px-3 py-3">
                        <button
                          onClick={() => handleToggleEnabled(page)}
                          className={`badge-pill ${page.enabled ? 'badge-success' : 'badge-warning'}`}
                        >
                          <span
                            className={`inline-block h-1.5 w-1.5 rounded-full ${
                              page.enabled ? 'bg-success' : 'bg-muted-foreground'
                            }`}
                          />
                          {page.enabled ? '启用' : '禁用'}
                        </button>
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => handleCopyLink(page)}
                            className="text-xs font-medium text-success transition-colors hover:underline"
                          >
                            {copiedId === page.id ? '已复制!' : '复制链接'}
                          </button>
                          <button
                            onClick={() => handleOpenEdit(page)}
                            className="text-xs font-medium text-primary transition-colors hover:underline"
                          >
                            编辑
                          </button>
                          <button
                            onClick={() => setDeleteTarget(page)}
                            className="text-xs font-medium text-destructive transition-colors hover:underline"
                          >
                            删除
                          </button>
                        </div>
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
                {editingId !== null ? '编辑分享页' : '添加分享页'}
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
              {/* 标题 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  标题 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.title}
                  onChange={(e) => setForm({ ...form, title: e.target.value })}
                  placeholder="例如：生产环境监控"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 描述 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  描述
                </label>
                <textarea
                  value={form.description}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                  placeholder="分享页的描述信息（可选）"
                  rows={3}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* Agent IDs */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  Agent ID 列表
                </label>
                <input
                  type="text"
                  value={form.agent_ids}
                  onChange={(e) => setForm({ ...form, agent_ids: e.target.value })}
                  placeholder="例如：1,2,3（留空表示全部）"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
                <p className="mt-1 text-xs text-muted-foreground/70">
                  逗号分隔的 Agent ID，留空则展示全部 Agent
                </p>
              </div>

              {/* 排序 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  排序权重
                </label>
                <input
                  type="number"
                  value={form.sort_order}
                  onChange={(e) => setForm({ ...form, sort_order: Number(e.target.value) })}
                  min={0}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
                <p className="mt-1 text-xs text-muted-foreground/70">
                  数值越小排序越靠前
                </p>
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
                  启用该分享页
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
                  确定删除分享页 "{deleteTarget.title}"？此操作不可恢复。
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
