import { useState, useEffect, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from '@/components/ThemeToggle'

/** 登录页 */
export default function Login() {
  const navigate = useNavigate()
  const login = useServerStore((s) => s.login)
  const authLoading = useServerStore((s) => s.authLoading)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const needsSetup = useServerStore((s) => s.needsSetup)

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

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

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (!username.trim()) {
      setError('请输入用户名')
      return
    }
    if (!password) {
      setError('请输入密码')
      return
    }

    try {
      await login(username.trim(), password)
      navigate('/admin', { replace: true })
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
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

      {/* 登录卡片 */}
      <div className="relative w-full max-w-md">
        <div className="rounded-2xl border border-border bg-card p-8 shadow-xl animate-fade-in">
          {/* Logo */}
          <div className="mb-8 text-center">
            <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary text-primary-foreground font-bold text-xl">
              SP
            </div>
            <h1 className="text-2xl font-bold text-foreground">服务器探针</h1>
            <p className="mt-1 text-sm text-muted-foreground">安全第一的纯只读服务器监控</p>
          </div>

          {/* 登录表单 */}
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-sm font-medium text-foreground">
                用户名
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
                placeholder="请输入密码"
                autoComplete="current-password"
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
              {authLoading ? '登录中...' : '登录'}
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
