import { useCallback, useEffect, useRef, useState } from 'react'
import QRCode from 'qrcode'
import { getTOTPStatus, setupTOTP, enableTOTP, disableTOTP } from '@/lib/api'

/**
 * TOTP 两步验证设置卡片
 * 流程：生成密钥 → 扫码/手动录入认证器 → 输入动态码验证启用 → （可选）密码确认停用
 */
export default function TOTPSettings() {
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [loading, setLoading] = useState(false)
  const [secret, setSecret] = useState('')
  const [otpauthUrl, setOtpauthUrl] = useState('')
  const [qrReady, setQrReady] = useState(false)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [disableCode, setDisableCode] = useState('')
  const [showDisable, setShowDisable] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const qrCanvasRef = useRef<HTMLCanvasElement>(null)

  const loadStatus = useCallback(async () => {
    try {
      const status = await getTOTPStatus()
      setEnabled(status.totp_enabled)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载两步验证状态失败')
    }
  }, [])

  useEffect(() => {
    loadStatus()
  }, [loadStatus])

  // 生成二维码
  useEffect(() => {
    if (!otpauthUrl || !qrCanvasRef.current) {
      setQrReady(false)
      return
    }
    QRCode.toCanvas(qrCanvasRef.current, otpauthUrl, {
      width: 180,
      margin: 1,
      color: { dark: '#000000', light: '#ffffff' },
    })
      .then(() => setQrReady(true))
      .catch(() => setQrReady(false))
  }, [otpauthUrl])

  const handleSetup = async () => {
    setError('')
    setMessage('')
    setLoading(true)
    try {
      const result = await setupTOTP()
      setSecret(result.secret)
      setOtpauthUrl(result.otpauth_url)
      setCode('')
    } catch (err) {
      setError(err instanceof Error ? err.message : '生成密钥失败')
    } finally {
      setLoading(false)
    }
  }

  const handleEnable = async () => {
    setError('')
    setMessage('')
    const trimmed = code.trim()
    if (!/^\d{6}$/.test(trimmed)) {
      setError('请输入 6 位动态验证码')
      return
    }
    setLoading(true)
    try {
      await enableTOTP(trimmed)
      setEnabled(true)
      setSecret('')
      setOtpauthUrl('')
      setQrReady(false)
      setCode('')
      setMessage('两步验证已启用')
    } catch (err) {
      setError(err instanceof Error ? err.message : '启用失败')
    } finally {
      setLoading(false)
    }
  }

  const handleDisable = async () => {
    setError('')
    setMessage('')
    if (!password) {
      setError('请输入密码确认')
      return
    }
    const trimmedCode = disableCode.trim()
    if (!/^\d{6}$/.test(trimmedCode)) {
      setError('请输入认证器当前的 6 位动态码')
      return
    }
    setLoading(true)
    try {
      await disableTOTP(password, trimmedCode)
      setEnabled(false)
      setPassword('')
      setDisableCode('')
      setShowDisable(false)
      setMessage('两步验证已停用')
    } catch (err) {
      setError(err instanceof Error ? err.message : '停用失败')
    } finally {
      setLoading(false)
    }
  }

  const copySecret = async () => {
    try {
      await navigator.clipboard.writeText(secret)
      setMessage('密钥已复制')
    } catch {
      setError('复制失败，请手动选择复制')
    }
  }

  return (
    <div className="card-soft p-5">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-base font-semibold text-foreground">两步验证（TOTP）</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">
            登录时除密码外需输入认证器动态码，大幅提升账户安全性
          </p>
        </div>
        <span
          className={`rounded-full px-2.5 py-1 text-xs font-medium ${
            enabled === null
              ? 'bg-muted text-muted-foreground'
              : enabled
                ? 'bg-green-500/10 text-green-600 dark:text-green-400'
                : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
          }`}
        >
          {enabled === null ? '加载中' : enabled ? '已启用' : '未启用'}
        </span>
      </div>

      {error && (
        <div className="mt-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}
      {message && (
        <div className="mt-3 rounded-md border border-green-500/30 bg-green-500/10 px-3 py-2 text-sm text-green-600 dark:text-green-400">
          {message}
        </div>
      )}

      {/* 未启用：生成密钥流程 */}
      {enabled === false && !secret && (
        <div className="mt-4">
          <button
            onClick={handleSetup}
            disabled={loading}
            className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
          >
            {loading ? '生成中...' : '生成密钥并绑定认证器'}
          </button>
        </div>
      )}

      {/* 绑定流程：显示二维码 + 验证码输入 */}
      {enabled === false && secret && (
        <div className="mt-4 space-y-4">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
            {qrReady && (
              <div className="shrink-0 rounded-lg border border-input bg-white p-2">
                <canvas ref={qrCanvasRef} width={180} height={180} />
              </div>
            )}
            <div className="min-w-0 flex-1 space-y-2">
              <p className="text-sm text-foreground">1. 使用认证器 App 扫描二维码</p>
              <p className="text-sm text-foreground">2. 或手动输入以下密钥</p>
              <div className="flex items-center gap-2">
                <code className="min-w-0 flex-1 truncate rounded-md border border-input bg-muted px-3 py-2 font-mono text-sm text-foreground">
                  {secret}
                </code>
                <button
                  onClick={copySecret}
                  className="shrink-0 rounded-md border border-input px-3 py-2 text-xs font-medium text-foreground transition-colors hover:bg-accent"
                >
                  复制
                </button>
              </div>
              <p className="text-sm text-foreground">3. 输入认证器当前显示的 6 位动态码完成绑定</p>
            </div>
          </div>

          <div className="flex gap-2">
            <input
              type="text"
              inputMode="numeric"
              pattern="\d*"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="6 位动态码"
              className="h-10 w-40 rounded-md border border-input bg-background px-3 text-center font-semibold tracking-widest tabular-nums text-foreground outline-none transition-colors focus:border-primary focus:ring-1 focus:ring-primary"
            />
            <button
              onClick={handleEnable}
              disabled={loading}
              className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {loading ? '验证中...' : '验证并启用'}
            </button>
            <button
              onClick={() => {
                setSecret('')
                setOtpauthUrl('')
                setQrReady(false)
                setCode('')
                setError('')
                setMessage('')
              }}
              className="rounded-lg border border-input px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            >
              取消
            </button>
          </div>
        </div>
      )}

      {/* 已启用：停用入口 */}
      {enabled === true && (
        <div className="mt-4">
          {!showDisable ? (
            <button
              onClick={() => {
                setShowDisable(true)
                setError('')
                setMessage('')
              }}
              className="rounded-lg border border-destructive/40 px-4 py-2 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10"
            >
              停用两步验证
            </button>
          ) : (
            <div className="space-y-2">
              <div className="flex flex-col gap-2 sm:flex-row">
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="输入登录密码确认"
                  className="h-10 flex-1 rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none transition-colors focus:border-primary focus:ring-1 focus:ring-primary"
                  autoComplete="current-password"
                />
                <input
                  type="text"
                  inputMode="numeric"
                  maxLength={6}
                  value={disableCode}
                  onChange={(e) => setDisableCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  placeholder="6 位动态码"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-center font-mono tracking-widest text-sm text-foreground outline-none transition-colors focus:border-primary focus:ring-1 focus:ring-primary sm:w-40"
                  autoComplete="one-time-code"
                />
              </div>
              <div className="flex gap-2">
                <button
                  onClick={handleDisable}
                  disabled={loading}
                  className="rounded-lg bg-destructive px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-destructive/90 disabled:opacity-50"
                >
                  {loading ? '停用中...' : '确认停用'}
                </button>
                <button
                  onClick={() => {
                    setShowDisable(false)
                    setPassword('')
                    setDisableCode('')
                  }}
                  className="rounded-lg border border-input px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                >
                  取消
                </button>
              </div>
              <p className="text-xs text-muted-foreground">
                停用两步验证为敏感降级操作，需同时输入登录密码与认证器当前动态码
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
