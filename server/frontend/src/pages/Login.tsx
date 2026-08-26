import { useState, useEffect, useRef, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from '@/components/ThemeToggle'

/** 登录页（支持 TOTP 两步验证：密码验证通过后进入动态码步骤） */
export default function Login() {
  const navigate = useNavigate()
  const login = useServerStore((s) => s.login)
  const authLoading = useServerStore((s) => s.authLoading)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const needsSetup = useServerStore((s) => s.needsSetup)

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [error, setError] = useState('')
  /** 是否处于 TOTP 二步验证阶段（密码已验证通过） */
  const [needTOTP, setNeedTOTP] = useState(false)
  const totpInputRef = useRef<HTMLInputElement>(null)

  // 如果需要初始化，跳转到设置页
  useEffect(() => {
    if (needsSetup) {
      navigate('/setup', { replace: true })
    }
  }, [needsSetup, navigate])

  // 已登录则跳转到管理后台
  useEffect(() => {
    if (isAuthenticated && !needsSetup) {
      navigate('/admin', { replace: true })
    }
  }, [isAuthenticated, needsSetup, navigate])

  // 进入 TOTP 步骤时聚焦动态码输入框
  useEffect(() => {
    if (needTOTP) {
      totpInputRef.current?.focus()
    }
  }, [needTOTP])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (needTOTP) {
      // 第二步：提交 TOTP 动态码（连同已验证的用户名密码）
      const code = totpCode.trim()
      if (!/^\d{6}$/.test(code)) {
        setError('请输入 6 位动态验证码')
        return
      }
      try {
        const result = await login(username.trim(), password, code)
        if (result.needTOTP) {
          setError('验证码错误或已过期，请重试')
          setTotpCode('')
          totpInputRef.current?.focus()
          return
        }
        navigate('/admin', { replace: true })
      } catch (err) {
        setError(err instanceof Error ? err.message : '登录失败')
        setTotpCode('')
      }
      return
    }

    // 第一步：用户名密码
    if (!username.trim()) {
      setError('请输入用户名')
      return
    }
    if (!password) {
      setError('请输入密码')
      return
    }

    try {
      const result = await login(username.trim(), password)
      if (result.needTOTP) {
        // 密码正确，进入 TOTP 二步验证
        setNeedTOTP(true)
        setTotpCode('')
        return
      }
      navigate('/admin', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    }
  }

  const handleBackToPassword = () => {
    setNeedTOTP(false)
    setTotpCode('')
    setError('')
    setPassword('')
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-background px-4">
      {/* 背景装饰 */}
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute -top-40 -right-40 h-96 w-96 rounded-full bg-primary/10 blur-3xl" />
        <div className="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary/5 blur-3xl" />
      </div>

      {/* 主题切换 */}
      <div className="absolute right-4 top-4">
        <ThemeToggle />
      </div>

      {/* 登录卡片 */}
      <div className="relative w-full max-w-md">
        <div className="card-soft p-8 animate-fade-in">
          {/* Logo */}
          <div className="mb-8 text-center">
            <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-xl">
              SP
            </div>
            <h1 className="text-2xl font-bold text-primary">服务器探针</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {needTOTP ? '请输入两步验证码' : '安全第一的纯只读服务器监控'}
            </p>
          </div>

          {/* 登录表单 */}
          <form onSubmit={handleSubmit} className="space-y-4">
            {!needTOTP ? (
              <>
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    用户名
                  </label>
                  <input
                    type="text"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                    placeholder="请输入用户名"
                    autoComplete="username"
                    autoFocus
                  />
                </div>

                <div>
                  <label className="mb-1.5 block text-sm font-medium text-foreground">
                    密码
                  </label>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                    placeholder="请输入密码"
                    autoComplete="current-password"
                  />
                </div>
              </>
            ) : (
              <div>
                <label className="mb-1.5 block text-sm font-medium text-foreground">
                  两步验证码
                </label>
                <input
                  ref={totpInputRef}
                  type="text"
                  inputMode="numeric"
                  pattern="\d*"
                  maxLength={6}
                  value={totpCode}
                  onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  className="h-12 w-full rounded-md border border-input bg-background px-3 text-center text-lg font-semibold tracking-[0.5em] tabular-nums shadow-sm text-foreground outline-none transition-colors placeholder:tracking-normal placeholder:font-normal placeholder:text-sm placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                  placeholder="输入认证器中的 6 位动态码"
                  autoComplete="one-time-code"
                  autoFocus
                />
                <p className="mt-2 text-xs text-muted-foreground">
                  打开验证器 App（Google Authenticator 等），输入当前 6 位动态码
                </p>
              </div>
            )}

            {error && (
              <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={authLoading}
              className="w-full rounded-xl bg-primary py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {authLoading ? '登录中...' : needTOTP ? '验证并登录' : '登录'}
            </button>

            {needTOTP && (
              <button
                type="button"
                onClick={handleBackToPassword}
                className="w-full rounded-xl border border-input py-2.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                返回重新输入密码
              </button>
            )}
          </form>
        </div>

        <p className="mt-4 text-center text-xs text-muted-foreground">
          纯只读架构 · 强制 TLS · 非 root 运行
        </p>
      </div>
    </div>
  )
}
