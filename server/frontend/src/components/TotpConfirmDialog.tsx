import { useEffect, useRef, useState } from 'react'

interface TotpConfirmDialogProps {
  open: boolean
  title: string
  description?: string
  submitting?: boolean
  error?: string
  onConfirm: (code: string) => void
  onCancel: () => void
}

/** TOTP 动态码确认对话框（敏感操作两步验证再确认） */
export default function TotpConfirmDialog({
  open,
  title,
  description,
  submitting = false,
  error = '',
  onConfirm,
  onCancel,
}: TotpConfirmDialogProps) {
  const [code, setCode] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  // 每次打开时清空并聚焦输入框
  useEffect(() => {
    if (open) {
      setCode('')
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [open])

  if (!open) return null

  const handleSubmit = () => {
    const trimmed = code.trim()
    if (!/^\d{6}$/.test(trimmed)) return
    if (!submitting) onConfirm(trimmed)
  }

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4"
      onClick={submitting ? undefined : onCancel}
    >
      <div
        className="w-full max-w-sm card-soft p-5"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-start justify-between">
          <div className="flex items-start gap-3">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
              <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={1.8}
                  d="M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 15a1.7 1.7 0 00.3 1.9l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.9-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1-1.6 1.7 1.7 0 00-1.9.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.9 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.6-1 1.7 1.7 0 00-.3-1.9l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.9.3h.1a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.6 1.7 1.7 0 001.9-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.9v.1a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z"
                />
              </svg>
            </div>
            <div>
              <h3 className="text-base font-semibold text-foreground">{title}</h3>
              {description && (
                <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{description}</p>
              )}
            </div>
          </div>
          {!submitting && (
            <button
              onClick={onCancel}
              className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
              aria-label="关闭"
            >
              <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>

        <label className="mb-1.5 block text-xs font-medium text-foreground">
          认证器动态验证码
        </label>
        <input
          ref={inputRef}
          type="text"
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={6}
          value={code}
          disabled={submitting}
          onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleSubmit()
            if (e.key === 'Escape' && !submitting) onCancel()
          }}
          placeholder="6 位数字"
          className="h-12 w-full rounded-md border border-input bg-background px-3 text-center font-mono text-lg tracking-[0.5em] shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-50"
        />

        {error && <p className="mt-2 text-xs text-destructive">{error}</p>}

        <div className="mt-4 flex justify-end gap-2">
          <button
            onClick={onCancel}
            disabled={submitting}
            className="flex h-9 items-center rounded-md border border-border bg-card px-4 text-sm font-semibold text-foreground transition-colors hover:bg-accent disabled:opacity-50"
          >
            取消
          </button>
          <button
            onClick={handleSubmit}
            disabled={submitting || !/^\d{6}$/.test(code.trim())}
            className="flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {submitting && (
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent" />
            )}
            确认操作
          </button>
        </div>
      </div>
    </div>
  )
}
