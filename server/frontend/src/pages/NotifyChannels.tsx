import { useEffect, useState, useCallback } from 'react'
import {
  getNotifyChannels,
  createNotifyChannel,
  updateNotifyChannel,
  deleteNotifyChannel,
  testNotifyChannel,
} from '@/lib/api'
import type { NotifyChannel } from '@/types'

/** 渠道类型选项 */
const TYPE_OPTIONS = [
  { value: 'webhook', label: 'Webhook' },
  { value: 'telegram', label: 'Telegram' },
  { value: 'email', label: '邮件 (Email)' },
]

/** 获取类型中文名 */
function getTypeLabel(type: string): string {
  const opt = TYPE_OPTIONS.find((o) => o.value === type)
  return opt ? opt.label : type
}

/** Webhook 配置表单字段 */
interface WebhookConfig {
  url: string
  secret: string
}

/** Telegram 配置表单字段 */
interface TelegramConfig {
  bot_token: string
  chat_id: string
}

/** Email 配置表单字段 */
interface EmailConfig {
  smtp_host: string
  smtp_port: number
  username: string
  password: string
  from: string
  to: string
  use_tls: boolean
}

/** 表单数据 */
interface FormData {
  name: string
  type: string
  webhook: WebhookConfig
  telegram: TelegramConfig
  email: EmailConfig
}

/** 空表单 */
const EMPTY_FORM: FormData = {
  name: '',
  type: 'webhook',
  webhook: { url: '', secret: '' },
  telegram: { bot_token: '', chat_id: '' },
  email: {
    smtp_host: '',
    smtp_port: 465,
    username: '',
    password: '',
    from: '',
    to: '',
    use_tls: true,
  },
}

/** 根据类型和表单生成配置 JSON 字符串 */
function buildConfig(type: string, form: FormData): string {
  if (type === 'webhook') {
    return JSON.stringify({
      url: form.webhook.url,
      secret: form.webhook.secret,
    })
  }
  if (type === 'telegram') {
    return JSON.stringify({
      bot_token: form.telegram.bot_token,
      chat_id: form.telegram.chat_id,
    })
  }
  // email
  return JSON.stringify({
    smtp_host: form.email.smtp_host,
    smtp_port: Number(form.email.smtp_port),
    username: form.email.username,
    password: form.email.password,
    from: form.email.from,
    to: form.email.to,
    use_tls: form.email.use_tls,
  })
}

/** 通知渠道管理页 */
export default function NotifyChannels() {
  const [channels, setChannels] = useState<NotifyChannel[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [testingId, setTestingId] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<NotifyChannel | null>(null)
  const [deleting, setDeleting] = useState(false)

  /** 加载数据 */
  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getNotifyChannels()
      setChannels(res.channels || [])
    } catch (err) {
      console.error('加载通知渠道失败:', err)
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
  const handleOpenEdit = (channel: NotifyChannel) => {
    setEditingId(channel.id)
    const cfg = (channel.config || {}) as Record<string, unknown>
    setForm({
      name: channel.name,
      type: channel.type,
      webhook: {
        url: String(cfg.url || ''),
        // 编辑时密码字段显示为空，不填则不修改
        secret: '',
      },
      telegram: {
        bot_token: '',
        chat_id: String(cfg.chat_id || ''),
      },
      email: {
        smtp_host: String(cfg.smtp_host || ''),
        smtp_port: Number(cfg.smtp_port || 465),
        username: String(cfg.username || ''),
        password: '',
        from: String(cfg.from || ''),
        to: String(cfg.to || ''),
        use_tls: cfg.use_tls !== undefined ? Boolean(cfg.use_tls) : true,
      },
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

  /** 校验表单 */
  const validate = (): string => {
    if (!form.name.trim()) return '请输入渠道名称'
    if (form.type === 'webhook') {
      if (!form.webhook.url.trim()) return '请输入 Webhook URL'
    } else if (form.type === 'telegram') {
      // 编辑时 bot_token 可为空（表示不修改），新增时必填
      if (editingId === null && !form.telegram.bot_token.trim()) return '请输入 Bot Token'
      if (!form.telegram.chat_id.trim()) return '请输入 Chat ID'
    } else if (form.type === 'email') {
      if (!form.email.smtp_host.trim()) return '请输入 SMTP 主机'
      if (!form.email.smtp_port) return '请输入 SMTP 端口'
      if (!form.email.from.trim()) return '请输入发件人邮箱'
      if (!form.email.to.trim()) return '请输入收件人邮箱'
      if (editingId === null && !form.email.password.trim()) return '请输入邮箱密码'
    }
    return ''
  }

  /** 提交表单 */
  const handleSubmit = async () => {
    setFormError('')
    const err = validate()
    if (err) {
      setFormError(err)
      return
    }

    setSubmitting(true)
    try {
      const configStr = buildConfig(form.type, form)

      if (editingId !== null) {
        // 编辑：密码字段为空时，从原配置中移除密码字段
        let finalConfigStr = configStr
        if (form.type === 'webhook' && !form.webhook.secret) {
          const parsed = JSON.parse(configStr)
          delete parsed.secret
          finalConfigStr = JSON.stringify(parsed)
        } else if (form.type === 'telegram' && !form.telegram.bot_token) {
          const parsed = JSON.parse(configStr)
          delete parsed.bot_token
          finalConfigStr = JSON.stringify(parsed)
        } else if (form.type === 'email' && !form.email.password) {
          const parsed = JSON.parse(configStr)
          delete parsed.password
          finalConfigStr = JSON.stringify(parsed)
        }
        await updateNotifyChannel(editingId, {
          name: form.name.trim(),
          type: form.type,
          config: finalConfigStr,
        })
      } else {
        await createNotifyChannel({
          name: form.name.trim(),
          type: form.type,
          config: configStr,
        })
      }

      handleCloseModal()
      await loadData()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '操作失败')
    } finally {
      setSubmitting(false)
    }
  }

  /** 删除渠道 */
  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deleteNotifyChannel(deleteTarget.id)
      setDeleteTarget(null)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除失败')
    } finally {
      setDeleting(false)
    }
  }

  /** 测试渠道 */
  const handleTest = async (channel: NotifyChannel) => {
    setTestingId(channel.id)
    try {
      await testNotifyChannel(channel.id)
      alert(`渠道 "${channel.name}" 测试通知已发送`)
    } catch (err) {
      alert(err instanceof Error ? err.message : '测试失败')
    } finally {
      setTestingId(null)
    }
  }

  /** 输入框通用样式 */
  const inputClass =
    'h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary'

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">通知渠道</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            管理告警通知渠道（Webhook / Telegram / 邮件），用于接收告警消息
          </p>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          添加渠道
        </button>
      </div>

      {/* 渠道列表 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">通知渠道列表 ({channels.length})</h2>
        </div>

        {loading && channels.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : channels.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无通知渠道</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击"添加渠道"创建第一个通知渠道</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs text-muted-foreground">
                  <th className="px-4 py-2 font-medium">ID</th>
                  <th className="px-4 py-2 font-medium">名称</th>
                  <th className="px-4 py-2 font-medium">类型</th>
                  <th className="px-4 py-2 font-medium">创建时间</th>
                  <th className="px-4 py-2 font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {channels.map((channel) => (
                  <tr key={channel.id} className="text-foreground">
                    <td className="px-4 py-3 text-muted-foreground">{channel.id}</td>
                    <td className="px-4 py-3 font-medium">{channel.name}</td>
                    <td className="px-4 py-3">
                      <span className="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">
                        {getTypeLabel(channel.type)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {channel.created_at ? new Date(channel.created_at).toLocaleString('zh-CN') : '-'}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => handleOpenEdit(channel)}
                          className="text-xs text-primary transition-colors hover:underline"
                        >
                          编辑
                        </button>
                        <button
                          onClick={() => handleTest(channel)}
                          disabled={testingId === channel.id}
                          className="text-xs text-success transition-colors hover:underline disabled:opacity-50"
                        >
                          {testingId === channel.id ? '测试中...' : '测试'}
                        </button>
                        <button
                          onClick={() => setDeleteTarget(channel)}
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
                {editingId !== null ? '编辑通知渠道' : '添加通知渠道'}
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
                  渠道名称 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="例如：运维群 Webhook"
                  className={inputClass}
                />
              </div>

              {/* 类型 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  渠道类型 <span className="text-destructive">*</span>
                </label>
                <select
                  value={form.type}
                  onChange={(e) => setForm({ ...form, type: e.target.value })}
                  className={inputClass}
                  disabled={editingId !== null}
                >
                  {TYPE_OPTIONS.map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </select>
                {editingId !== null && (
                  <p className="mt-1 text-xs text-muted-foreground/70">编辑时不可修改渠道类型</p>
                )}
              </div>

              {/* 根据类型显示不同配置表单 */}
              {form.type === 'webhook' && (
                <>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      Webhook URL <span className="text-destructive">*</span>
                    </label>
                    <input
                      type="text"
                      value={form.webhook.url}
                      onChange={(e) => setForm({ ...form, webhook: { ...form.webhook, url: e.target.value } })}
                      placeholder="https://example.com/webhook"
                      className={inputClass}
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      Secret {editingId !== null && <span className="text-muted-foreground/60">(留空不修改)</span>}
                    </label>
                    <input
                      type="password"
                      value={form.webhook.secret}
                      onChange={(e) => setForm({ ...form, webhook: { ...form.webhook, secret: e.target.value } })}
                      placeholder="用于签名校验的密钥"
                      className={inputClass}
                    />
                  </div>
                </>
              )}

              {form.type === 'telegram' && (
                <>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      Bot Token {editingId !== null && <span className="text-muted-foreground/60">(留空不修改)</span>}
                      {editingId === null && <span className="text-destructive">*</span>}
                    </label>
                    <input
                      type="password"
                      value={form.telegram.bot_token}
                      onChange={(e) => setForm({ ...form, telegram: { ...form.telegram, bot_token: e.target.value } })}
                      placeholder="123456:ABC-DEF..."
                      className={inputClass}
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      Chat ID <span className="text-destructive">*</span>
                    </label>
                    <input
                      type="text"
                      value={form.telegram.chat_id}
                      onChange={(e) => setForm({ ...form, telegram: { ...form.telegram, chat_id: e.target.value } })}
                      placeholder="例如：-1001234567890"
                      className={inputClass}
                    />
                  </div>
                </>
              )}

              {form.type === 'email' && (
                <>
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div className="col-span-1">
                      <label className="mb-1 block text-xs font-medium text-muted-foreground">
                        SMTP 主机 <span className="text-destructive">*</span>
                      </label>
                      <input
                        type="text"
                        value={form.email.smtp_host}
                        onChange={(e) => setForm({ ...form, email: { ...form.email, smtp_host: e.target.value } })}
                        placeholder="smtp.example.com"
                        className={inputClass}
                      />
                    </div>
                    <div className="col-span-1">
                      <label className="mb-1 block text-xs font-medium text-muted-foreground">
                        SMTP 端口 <span className="text-destructive">*</span>
                      </label>
                      <input
                        type="number"
                        value={form.email.smtp_port}
                        onChange={(e) => setForm({ ...form, email: { ...form.email, smtp_port: Number(e.target.value) } })}
                        placeholder="465"
                        className={inputClass}
                      />
                    </div>
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      用户名
                    </label>
                    <input
                      type="text"
                      value={form.email.username}
                      onChange={(e) => setForm({ ...form, email: { ...form.email, username: e.target.value } })}
                      placeholder="user@example.com"
                      className={inputClass}
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      密码 {editingId !== null && <span className="text-muted-foreground/60">(留空不修改)</span>}
                      {editingId === null && <span className="text-destructive">*</span>}
                    </label>
                    <input
                      type="password"
                      value={form.email.password}
                      onChange={(e) => setForm({ ...form, email: { ...form.email, password: e.target.value } })}
                      placeholder="邮箱密码或授权码"
                      className={inputClass}
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      发件人 <span className="text-destructive">*</span>
                    </label>
                    <input
                      type="text"
                      value={form.email.from}
                      onChange={(e) => setForm({ ...form, email: { ...form.email, from: e.target.value } })}
                      placeholder="from@example.com"
                      className={inputClass}
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-muted-foreground">
                      收件人 <span className="text-destructive">*</span>
                    </label>
                    <input
                      type="text"
                      value={form.email.to}
                      onChange={(e) => setForm({ ...form, email: { ...form.email, to: e.target.value } })}
                      placeholder="to@example.com（多个用逗号分隔）"
                      className={inputClass}
                    />
                  </div>
                  <div className="flex items-center gap-2 pb-1">
                    <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
                      <input
                        type="checkbox"
                        checked={form.email.use_tls}
                        onChange={(e) => setForm({ ...form, email: { ...form.email, use_tls: e.target.checked } })}
                        className="h-4 w-4 rounded border-border"
                      />
                      使用 TLS 加密
                    </label>
                  </div>
                </>
              )}

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
                  确定删除通知渠道 "{deleteTarget.name}"？此操作不可恢复。
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
