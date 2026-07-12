import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from './ThemeToggle'
import Background from './Background'
import Footer from './Footer'
import StatusDot from './StatusDot'

/** 侧边栏导航分组配置（模块级常量，避免每次渲染重建数组） */
const NAV_GROUPS = [
  {
    title: '管理',
    items: [
      { to: '/admin', label: '仪表盘', icon: '▣', end: true },
      { to: '/admin/agents', label: 'Agent 管理', icon: '⬡', end: false },
      { to: '/admin/ping-targets', label: '探测目标', icon: '◈', end: false },
      { to: '/admin/alerts', label: '告警管理', icon: '⚠', end: false },
      { to: '/admin/notify', label: '通知渠道', icon: '✉', end: false },
      { to: '/admin/service-monitors', label: '服务监控', icon: '◉', end: false },
      { to: '/admin/ssl-monitors', label: 'SSL 监控', icon: '🔒', end: false },
      { to: '/admin/traffic', label: '流量统计', icon: '📊', end: false },
      { to: '/admin/share-pages', label: '分享页', icon: '🔗', end: false },
      { to: '/admin/system', label: '系统状态', icon: '⚙', end: false },
      { to: '/admin/logs', label: '系统日志', icon: '📋', end: false },
    ],
  },
]

/** 布局组件（浮动顶栏 + 侧边栏 + 主内容区） - NodeGet 风格 */
export default function Layout() {
  const navigate = useNavigate()
  const location = useLocation()
  const logout = useServerStore((s) => s.logout)
  const wsConnected = useServerStore((s) => s.wsConnected)
  const fetchServers = useServerStore((s) => s.fetchServers)
  const connectWebSocket = useServerStore((s) => s.connectWebSocket)
  const disconnectWebSocket = useServerStore((s) => s.disconnectWebSocket)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const servers = useServerStore((s) => s.servers)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [scrolled, setScrolled] = useState(false)
  // 防止 disconnectWebSocket 在 effect cleanup 中被重复调用
  const hasDisconnectedRef = useRef(false)

  // 首次加载时获取服务器列表并连接 WebSocket
  useEffect(() => {
    if (isAuthenticated) {
      // 重置断开标记，确保本次 effect 生命周期内可正常断开
      hasDisconnectedRef.current = false
      fetchServers().catch(() => {
        // 错误处理在 API 层已做
      })
      connectWebSocket()
      return () => {
        // 使用 ref 防止 disconnectWebSocket 被重复调用
        if (!hasDisconnectedRef.current) {
          hasDisconnectedRef.current = true
          disconnectWebSocket()
        }
      }
    }
  }, [isAuthenticated, fetchServers, connectWebSocket, disconnectWebSocket])

  // 路由变化时关闭移动端导航
  useEffect(() => {
    setMobileNavOpen(false)
  }, [location.pathname])

  // ESC 键关闭移动端导航 + body 滚动锁定
  useEffect(() => {
    if (!mobileNavOpen) return

    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMobileNavOpen(false)
    }
    document.addEventListener('keydown', handleEsc)
    document.body.style.overflow = 'hidden'

    return () => {
      document.removeEventListener('keydown', handleEsc)
      document.body.style.overflow = ''
    }
  }, [mobileNavOpen])

  // 滚动检测：scrollY > 12 时增强导航栏阴影
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 12)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  const handleLogout = useCallback(async () => {
    await logout()
    navigate('/login')
  }, [logout, navigate])

  const onlineCount = useMemo(() => servers.filter((s) => s.online).length, [servers])
  const totalCount = servers.length

  const renderNavItems = (onNavigate?: () => void) => (
    <nav className="flex flex-col gap-4 p-3">
      {NAV_GROUPS.map((group) => (
        <div key={group.title} className="flex flex-col gap-1">
          <h3 className="px-3 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
            {group.title}
          </h3>
          {group.items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              onClick={onNavigate}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors ${
                  isActive
                    ? 'bg-primary text-primary-foreground font-medium'
                    : 'text-muted-foreground hover:bg-accent hover:text-foreground'
                }`
              }
            >
              <span className="text-base">{item.icon}</span>
              <span>{item.label}</span>
            </NavLink>
          ))}
        </div>
      ))}
    </nav>
  )

  return (
    <div className="relative min-h-screen">
      {/* 背景 */}
      <Background />

      {/* 浮动式导航栏 */}
      <header className="fixed inset-x-0 top-0 z-40 pt-3 px-4 sm:px-6">
        <div className="mx-auto max-w-[91.5rem]">
          <div
            className={`glass flex h-16 items-center justify-between rounded-2xl border border-border px-4 transition-shadow duration-200 sm:h-[68px] sm:px-5 ${
              scrolled ? 'shadow-nav-stuck dark:shadow-nav-stuck-dark' : 'shadow-nav dark:shadow-nav-dark'
            }`}
          >
            {/* 左侧：移动端菜单按钮 + Logo + 标题 */}
            <div className="flex items-center gap-2 sm:gap-3">
              <button
                onClick={() => setMobileNavOpen(!mobileNavOpen)}
                className="flex h-9 w-9 items-center justify-center rounded-lg border border-border bg-secondary text-foreground transition-colors hover:bg-accent md:hidden"
                aria-label="切换导航菜单"
              >
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  {mobileNavOpen ? (
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  ) : (
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                  )}
                </svg>
              </button>
              <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-sm">
                SP
              </div>
              <span className="text-base font-bold tracking-wide text-primary sm:text-xl">
                服务器探针
              </span>
            </div>

            {/* 右侧：WS 状态 + 在线计数 + ThemeToggle + 退出 */}
            <div className="flex items-center gap-2 sm:gap-3">
              {/* WebSocket 连接状态 */}
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <StatusDot online={wsConnected} />
                <span className="hidden sm:inline">{wsConnected ? '实时连接' : '已断开'}</span>
              </div>

              {/* 在线/总数 */}
              <div className="hidden items-center gap-1.5 rounded-lg bg-secondary px-3 py-1 text-xs text-secondary-foreground sm:flex">
                <span className="font-medium text-success">{onlineCount}</span>
                <span>/</span>
                <span>{totalCount}</span>
                <span className="ml-1">在线</span>
              </div>

              <ThemeToggle />

              <button
                onClick={handleLogout}
                className="flex h-9 items-center rounded-lg border border-border bg-secondary px-2 text-sm text-foreground transition-colors hover:bg-accent sm:px-3"
              >
                退出
              </button>
            </div>
          </div>
        </div>
      </header>

      {/* 主内容区 */}
      <div className="relative z-10 pt-[6.9rem] sm:pt-[7.6rem]">
        <div className="mx-auto max-w-[91.5rem] px-4 sm:px-6 pb-4 sm:pb-6">
          <div className="flex gap-6">
            {/* 侧边栏 - 桌面端（sticky 浮动卡片） */}
            <aside className="card-soft sticky top-[5.75rem] hidden h-[calc(100vh-7rem)] w-56 shrink-0 flex-col overflow-hidden md:flex">
              <div className="flex-1 overflow-y-auto scrollbar-thin">
                {renderNavItems()}
              </div>
              <div className="border-t border-dashed border-border p-3 text-xs text-muted-foreground">
                <a
                  href="/"
                  className="mb-2 flex items-center gap-1.5 rounded-md px-2 py-1.5 text-foreground transition-colors hover:bg-accent"
                >
                  <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
                  </svg>
                  返回公开页
                </a>
              </div>
            </aside>

            {/* 主内容 */}
            <main className="min-w-0 flex-1">
              <Outlet />
            </main>
          </div>
        </div>
      </div>

      {/* 底部 */}
      <Footer />

      {/* 移动端侧边栏 - 抽屉式 */}
      {mobileNavOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          {/* 遮罩 */}
          <div
            className="absolute inset-0 bg-black/50 backdrop-blur-sm"
            onClick={() => setMobileNavOpen(false)}
          />
          {/* 抽屉 */}
          <aside className="glass absolute left-0 top-0 flex h-full w-72 max-w-[85vw] flex-col border-r border-border shadow-2xl">
            <div className="flex h-16 shrink-0 items-center justify-between border-b border-dashed border-border px-4">
              <div className="flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-sm">
                  SP
                </div>
                <span className="text-base font-bold tracking-wide text-primary">服务器探针</span>
              </div>
              <button
                onClick={() => setMobileNavOpen(false)}
                className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
                aria-label="关闭菜单"
              >
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            <div className="flex-1 overflow-y-auto scrollbar-thin">
              {renderNavItems(() => setMobileNavOpen(false))}
            </div>
            <div className="border-t border-dashed border-border p-3 text-xs text-muted-foreground">
              <a
                href="/"
                className="mb-2 flex items-center gap-1.5 rounded-md px-2 py-1.5 text-foreground transition-colors hover:bg-accent"
              >
                <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
                </svg>
                返回公开页
              </a>
              <p>纯只读安全探针 v1.0.0</p>
            </div>
          </aside>
        </div>
      )}
    </div>
  )
}
