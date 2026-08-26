import { useEffect, useState } from 'react'
import { useTagStore } from '@/store/useTagStore'
import type { Tag } from '@/types'

/** 预设颜色（NodeGet 风格标签色板） */
const PRESET_COLORS = [
  '#3b82f6', '#8b5cf6', '#ec4899', '#ef4444', '#f97316',
  '#eab308', '#84cc16', '#22c55e', '#14b8a6', '#06b6d4',
  '#64748b', '#78716c',
]

/** 表单数据 */
interface FormData {
  name: string
  color: string
}

const EMPTY_FORM: FormData = { name: '', color: PRESET_COLORS[0] }

/** 标签管理页（CRUD + 颜色） */
export default function TagManagement() {
  const tags = useTagStore((s) => s.tags)
  const loading = useTagStore((s) => s.loading)
  const loaded = useTagStore((s) => s.loaded)
  const fetchTags = useTagStore((s) => s.fetchTags)
  const addTag = useTagStore((s) => s.addTag)
  const editTag = useTagStore((s) => s.editTag)
  const removeTag = useTagStore((s) => s.removeTag)

  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!loaded) void fetchTags()
  }, [loaded, fetchTags])

  /** 打开新增弹窗 */
  const handleOpenAdd = () => {
    setEditingId(null)
    setForm(EMPTY_FORM)
    setFormError('')
    setModalOpen(true)
  }

  /** 打开编辑弹窗 */
  const handleOpenEdit = (tag: Tag) => {
    setEditingId(tag.id)
    setForm({ name: tag.name, color: tag.color || PRESET_COLORS[0] })
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
    const name = form.name.trim()
    if (!name) {
      setFormError('请输入标签名称')
      return
    }
    if (name.length > 32) {
      setFormError('标签名称不能超过 32 个字符')
      return
    }
    if (!/^#[0-9a-fA-F]{6}$/.test(form.color)) {
      setFormError('颜色格式不正确')
      return
    }

    setSubmitting(true)
    try {
      if (editingId !== null) {
        await editTag(editingId, { name, color: form.color })
      } else {
        await addTag({ name, color: form.color })
      }
      handleCloseModal()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  /** 删除标签 */
  const handleDelete = async (tag: Tag) => {
    if (!confirm(`确定删除标签 "${tag.name}"？此操作不可恢复。`)) return
    try {
      await removeTag(tag.id)
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    }
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-primary">标签管理</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            管理服务器标签及徽章颜色，颜色将同步展示在仪表盘卡片上
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex h-10 items-center gap-1.5 rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          添加标签
        </button>
      </div>

      {/* 标签列表 */}
      <div className="card-soft overflow-hidden">
        <div className="border-b border-dashed border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">标签列表 ({tags.length})</h2>
        </div>

        {loading && !loaded ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : tags.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A2 2 0 013 12V7a4 4 0 014-4z" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无标签</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击"添加标签"创建第一个标签</p>
          </div>
        ) : (
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-secondary/30">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">标签预览</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">名称</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">颜色</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">创建时间</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {tags.map((tag) => (
                  <tr key={tag.id} className="text-foreground transition-colors hover:bg-muted/50">
                    <td className="px-3 py-3 tabular-nums text-muted-foreground">{tag.id}</td>
                    <td className="px-3 py-3">
                      <span
                        className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-white"
                        style={{ backgroundColor: tag.color }}
                      >
                        <span className="inline-block h-1.5 w-1.5 rounded-full bg-white/70" />
                        {tag.name}
                      </span>
                    </td>
                    <td className="px-3 py-3 font-medium">{tag.name}</td>
                    <td className="px-3 py-3">
                      <div className="flex items-center gap-2">
                        <span
                          className="inline-block h-4 w-4 rounded-full border border-border"
                          style={{ backgroundColor: tag.color }}
                        />
                        <span className="font-mono text-xs text-muted-foreground">{tag.color}</span>
                      </div>
                    </td>
                    <td className="px-3 py-3 text-xs tabular-nums text-muted-foreground">
                      {tag.created_at ? new Date(tag.created_at).toLocaleString('zh-CN') : '-'}
                    </td>
                    <td className="px-3 py-3">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => handleOpenEdit(tag)}
                          className="text-xs font-medium text-primary transition-colors hover:underline"
                        >
                          编辑
                        </button>
                        <button
                          onClick={() => handleDelete(tag)}
                          className="text-xs font-medium text-destructive transition-colors hover:underline"
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

      {/* 使用说明 */}
      <div className="card-soft p-4">
        <h2 className="text-sm font-semibold text-foreground">使用说明</h2>
        <ul className="mt-2 space-y-1.5 text-xs text-muted-foreground">
          <li>· 在「Agent 管理」中为服务器分配标签（逗号分隔多个标签）</li>
          <li>· 标签颜色会自动同步到仪表盘服务器卡片和列表的徽章上</li>
          <li>· 仪表盘顶部的标签筛选器基于服务器已分配的标签动态生成</li>
          <li>· 删除标签不会移除服务器上的标签名称，仅清除颜色配置</li>
        </ul>
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
                {editingId !== null ? '编辑标签' : '添加标签'}
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
                  标签名称 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="例如：香港 / 生产环境"
                  maxLength={32}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 预设颜色 */}
              <div>
                <label className="mb-1.5 block text-xs font-medium text-muted-foreground">预设颜色</label>
                <div className="grid grid-cols-6 gap-2">
                  {PRESET_COLORS.map((color) => (
                    <button
                      key={color}
                      type="button"
                      onClick={() => setForm({ ...form, color })}
                      className={`flex h-8 w-8 items-center justify-center rounded-lg border-2 transition-all ${
                        form.color.toLowerCase() === color.toLowerCase()
                          ? 'border-foreground scale-110'
                          : 'border-transparent hover:scale-105'
                      }`}
                      style={{ backgroundColor: color }}
                      title={color}
                    >
                      {form.color.toLowerCase() === color.toLowerCase() && (
                        <svg className="h-4 w-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                        </svg>
                      )}
                    </button>
                  ))}
                </div>
              </div>

              {/* 自定义颜色 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">自定义颜色</label>
                <div className="flex items-center gap-3">
                  <input
                    type="color"
                    value={form.color}
                    onChange={(e) => setForm({ ...form, color: e.target.value })}
                    className="h-10 w-14 cursor-pointer rounded-md border border-input bg-background p-1"
                  />
                  <input
                    type="text"
                    value={form.color}
                    onChange={(e) => setForm({ ...form, color: e.target.value })}
                    className="h-10 flex-1 rounded-md border border-input bg-background px-3 font-mono text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
              </div>

              {/* 实时预览 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">预览</label>
                <div className="rounded-lg border border-dashed border-border bg-muted/50 p-3">
                  <span
                    className="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium text-white"
                    style={{ backgroundColor: form.color }}
                  >
                    <span className="inline-block h-1.5 w-1.5 rounded-full bg-white/70" />
                    {form.name.trim() || '标签预览'}
                  </span>
                </div>
              </div>

              {formError && <p className="text-xs text-destructive">{formError}</p>}
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
    </div>
  )
}
