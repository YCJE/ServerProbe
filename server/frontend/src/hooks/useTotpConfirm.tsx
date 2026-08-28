import { useCallback, useRef, useState } from 'react'
import { isTotpRequired } from '@/lib/api'
import TotpConfirmDialog from '@/components/TotpConfirmDialog'

/** 用户在两步验证弹窗中点击取消 */
export class TotpCancelledError extends Error {
  constructor() {
    super('已取消两步验证')
    this.name = 'TotpCancelledError'
  }
}

interface PendingAction<T> {
  title: string
  description?: string
  action: (totpCode?: string) => Promise<T>
  resolve: (value: T) => void
  reject: (err: unknown) => void
}

interface RunOptions {
  /** 弹窗标题（如"删除 Agent"） */
  title: string
  /** 弹窗说明（如"该操作不可恢复，请输入动态码确认"） */
  description?: string
}

/**
 * 敏感操作两步验证 Hook
 *
 * 流程：先不带动态码直接执行 → 若后端返回 403 totp_required（账户已启用 TOTP），
 * 弹出动态码对话框 → 用户输入后携带 X-TOTP-Code 重试 → 成功后继续原 Promise。
 * 未启用 TOTP 的账户不会触发弹窗，直接执行。
 *
 * 用法：
 * const totp = useTotpConfirm()
 * try {
 *   await totp.runWithTotp(
 *     { title: '删除 Agent', description: '此操作不可恢复' },
 *     (code) => deleteAgentAPI(id, code),
 *   )
 * } catch (err) {
 *   if (err instanceof TotpCancelledError) return
 *   alert(err instanceof Error ? err.message : '操作失败')
 * }
 * // 渲染：{totp.dialog}
 */
export function useTotpConfirm() {
  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const pendingRef = useRef<PendingAction<unknown> | null>(null)

  const runWithTotp = useCallback(
    async <T,>(opts: RunOptions, action: (totpCode?: string) => Promise<T>): Promise<T> => {
      try {
        return await action()
      } catch (err) {
        if (!isTotpRequired(err)) throw err
      }
      // 账户已启用 TOTP：打开对话框等待用户输入动态码后重试
      return new Promise<T>((resolve, reject) => {
        setTitle(opts.title)
        setDescription(opts.description || '')
        setError('')
        setSubmitting(false)
        setOpen(true)
        pendingRef.current = {
          title: opts.title,
          description: opts.description,
          action: action as (totpCode?: string) => Promise<unknown>,
          resolve: resolve as (value: unknown) => void,
          reject,
        }
      })
    },
    [],
  )

  const handleConfirm = useCallback(async (code: string) => {
    const pending = pendingRef.current
    if (!pending) return
    setSubmitting(true)
    setError('')
    try {
      const result = await pending.action(code)
      pendingRef.current = null
      setOpen(false)
      setSubmitting(false)
      pending.resolve(result)
    } catch (err) {
      setSubmitting(false)
      // 验证码错误或其他失败：保留弹窗供重试
      setError(err instanceof Error ? err.message : '操作失败，请重试')
    }
  }, [])

  const handleCancel = useCallback(() => {
    const pending = pendingRef.current
    pendingRef.current = null
    setOpen(false)
    setError('')
    setSubmitting(false)
    pending?.reject(new TotpCancelledError())
  }, [])

  const dialog = (
    <TotpConfirmDialog
      open={open}
      title={title}
      description={description}
      submitting={submitting}
      error={error}
      onConfirm={handleConfirm}
      onCancel={handleCancel}
    />
  )

  return { runWithTotp, dialog }
}
