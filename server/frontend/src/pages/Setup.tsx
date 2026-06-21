import { useState, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from '@/components/ThemeToggle'

/** 首次设置页（创建管理员账户） */
export default function Setup() {
  const navigate = useNavigate()
  const setup = useServerStore((s) => s.setup)
  const authLoading = useServerStore((s) => s.authLoading)
  const needsSetup = useServerStore((s) => s.needsSetup)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const checkSetupStatus = useServerStore((s) => s.checkSetupStatus)

  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')

  // 检查是否需要初始化
  useEffect(() => {
    checkSetupStatus()
  }, [checkSetupStatus])

  // 如果不需要初始化，跳转到登录或管理后台
  useEffect(() => {
    if (!needsSetup && !authLoading) {
      if (isAuthenticated) {
        navigate('/admin', { replace: true })
      } else {
        navigate('/login', { replace: true })
      }
    }
  }, [needsSetup, isAuthenticated, authLoading, navigate])

  const validatePassword = (pwd: string): string | null => {
    if (pwd.length < 12) {
      return '密码长度至少 12 位'
    }
    if (!/[A-Z]/.test(pwd)) {
      return '密码必须包含大写字母'
    }
    if (!/[a-z]/.test(pwd)) {
      return '密码必须包含小写字母'
    }
    if (!/\d/.test(pwd)) {
      return '密码必须包含数字'
    }
    return null
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (!username.trim()) {
      setError('请输入用户名')
      return
    }

    const passwordError = validatePassword(password)
    if (passwordError) {
      setError(passwordError)
      return
    }

    if (password !== confirmPassword) {
      setError('两次输入的密码不一致')
      return
    }

    try {
      await setup(username.trim(), password)
      navigate('/admin', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '设置失败')
    }
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

      {/* 设置卡片 */}
      <div className="relative w-full max-w-md">
        <div className="rounded-2xl border border-border bg-card p-8 shadow-xl animate-fade-in">
          {/* Logo */}
          <div className="mb-6 text-center">
            <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary text-primary-foreground font-bold text-xl">
              SP
            </div>
            <h1 className="text-2xl font-bold text-foreground">初始化设置</h1>
            <p className="mt-1 text-sm text-muted-foreground">创建管理员账户以开始使用</p>
          </div>

          {/* 提示信息 */}
          <div className="mb-6 rounded-lg border border-primary/30 bg-primary/5 px-4 py-3 text-sm text-foreground">
            <p className="mb-1 font-medium">首次使用请创建管理员账户</p>
            <p className="text-xs text-muted-foreground">
              密码要求：至少 12 位，包含大小写字母和数字
            </p>
          </div>

          {/* 设置表单 */}
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-foreground">
                管理员用户名
              </label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full rounded-lg border border-input bg-background px-3 py-2.5 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
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
                className="w-full rounded-lg border border-input bg-background px-3 py-2.5 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                placeholder="至少 12 位，含大小写字母和数字"
                autoComplete="new-password"
              />
            </div>

            <div>
              <label className="mb-1.5 block text-sm font-medium text-foreground">
                确认密码
              </label>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full rounded-lg border border-input bg-background px-3 py-2.5 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                placeholder="请再次输入密码"
                autoComplete="new-password"
              />
            </div>

            {error && (
              <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={authLoading}
              className="w-full rounded-lg bg-primary py-2.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {authLoading ? '创建中...' : '创建管理员账户'}
            </button>
          </form>
        </div>

        <p className="mt-4 text-center text-xs text-muted-foreground">
          纯只读架构 · 强制 TLS · 非 root 运行
        </p>
      </div>
    </div>
  )
}
