import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { useSiteSettings } from '@/store/useSiteSettingsStore'
import ThemeToggle from './ThemeToggle'
import StatusDot from './StatusDot'

/** SVG 图标（lucide 风格线稿） */
const Icon = {
  dashboard: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M3 3h7v9H3zM14 3h7v5h-7zM14 12h7v9h-7zM3 16h7v5H3z" />,
  agent: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M5 12h14M12 5v14M6.5 6.5l11 11M17.5 6.5l-11 11" />,
  tag: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M20.6 13.4L12 22l-8-8V4h10l6.6 9.4zM7.5 7.5h.01" />,
  ping: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M3 12h4l3-8 4 16 3-8h4" />,
  alert: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M12 9v4m0 4h.01M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" />,
  notify: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M4 4h16v12H5.2L4 17.2zM8 9h8M8 12h5" />,
  service: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M22 12h-4l-3 9L9 3l-3 9H2" />,
  ssl: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10zM9.5 12l2 2 3.5-3.5" />,
  traffic: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M3 3v18h18M7 15l4-4 3 3 5-6" />,
  share: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M10 13a5 5 0 007.5.5l3-3a5 5 0 00-7-7l-1.8 1.7M14 11a5 5 0 00-7.5-.5l-3 3a5 5 0 007 7L12.3 19" />,
  settings: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 15a1.7 1.7 0 00.3 1.9l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.9-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1-1.6 1.7 1.7 0 00-1.9.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.9 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.6-1 1.7 1.7 0 00-.3-1.9l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.9.3h.1a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.6 1.7 1.7 0 001.9-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.9v.1a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z" />,
  logs: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8zM14 2v6h6M8 13h8M8 17h5" />,
  back: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M19 12H5M12 19l-7-7 7-7" />,
  logout: <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9" />,
}

/** 侧边栏导航配置（NodeGet 分组结构） */
const NAV_GROUPS = [
  {
    title: '监控',
    items: [
      { to: '/admin', label: '仪表盘', icon: Icon.dashboard, end: true },
      { to: '/admin/traffic', label: '流量统计', icon: Icon.traffic, end: false },
    ],
  },
  {
    title: '资源管理',
    items: [
      { to: '/admin/agents', label: 'Agent 管理', icon: Icon.agent, end: false },
      { to: '/admin/tags', label: '标签管理', icon: Icon.tag, end: false },
      { to: '/admin/ping-targets', label: '探测目标', icon: Icon.ping, end: false },
      { to: '/admin/share-pages', label: '分享页', icon: Icon.share, end: false },
    ],
  },
  {
    title: '告警与监控',
    items: [
      { to: '/admin/alerts', label: '告警管理', icon: Icon.alert, end: false },
      { to: '/admin/notify', label: '通知渠道', icon: Icon.notify, end: false },
      { to: '/admin/service-monitors', label: '服务监控', icon: Icon.service, end: false },
      { to: '/admin/ssl-monitors', label: 'SSL 监控', icon: Icon.ssl, end: false },
    ],
  },
  {
    title: '系统',
    items: [
      { to: '/admin/settings', label: '站点设置', icon: Icon.settings, end: false },
      { to: '/admin/system', label: '系统状态', icon: Icon.settings, end: false },
      { to: '/admin/logs', label: '系统日志', icon: Icon.logs, end: false },
    ],
  },
]

/** 布局组件（NodeGet 风格：全高侧边栏 + 顶栏 + 主内容区） */
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
  const { siteTitle } = useSiteSettings()
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [collapsed, setCollapsed] = useState(false)
  const hasDisconnectedRef = useRef(false)

  // 首次加载时获取服务器列表并连接 WebSocket
  useEffect(() => {
    if (isAuthenticated) {
      hasDisconnectedRef.current = false
      fetchServers().catch(() => {})
      connectWebSocket()
      return () => {
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

  const handleLogout = useCallback(async () => {
    await logout()
    navigate('/login')
  }, [logout, navigate])

  const onlineCount = useMemo(() => servers.filter((s) => s.online).length, [servers])
  const totalCount = servers.length

  const renderNavItems = (onNavigate?: () => void, compact = false) => (
    <nav className="flex flex-1 flex-col gap-5 overflow-y-auto px-3 py-4 scrollbar-thin">
      {NAV_GROUPS.map((group) => (
        <div key={group.title} className="flex flex-col gap-1">
          {!compact && (
            <h3 className="px-3 pb-1 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground/70">
              {group.title}
            </h3>
          )}
          {compact && <div className="mx-3 mb-1 border-t border-border" />}
          {group.items.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              onClick={onNavigate}
              title={item.label}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-md px-3 py-2 text-[13px] transition-colors ${
                  compact ? 'justify-center px-0' : ''
                } ${
                  isActive
                    ? 'bg-secondary font-medium text-foreground'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                }`
              }
            >
              <svg className="h-4 w-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                {item.icon}
              </svg>
              {!compact && <span className="truncate">{item.label}</span>}
            </NavLink>
          ))}
        </div>
      ))}
    </nav>
  )

  const renderSidebarFooter = (compact = false) => (
    <div className="border-t border-border p-3">
      <a
        href="/"
        className={`flex items-center gap-3 rounded-md px-3 py-2 text-[13px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground ${compact ? 'justify-center px-0' : ''}`}
        title="返回公开页"
      >
        <svg className="h-4 w-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          {Icon.back}
        </svg>
        {!compact && <span>返回公开页</span>}
      </a>
    </div>
  )

  return (
    <div className="flex min-h-screen">
      {/* 桌面端侧边栏（NodeGet: 全高贴边，可折叠） */}
      <aside
        className={`sticky top-0 hidden h-screen shrink-0 flex-col border-r border-border bg-card transition-[width] duration-200 md:flex ${
          collapsed ? 'w-14' : 'w-56'
        }`}
      >
        {/* Logo */}
        <div className={`flex h-14 items-center border-b border-border ${collapsed ? 'justify-center px-0' : 'px-4'}`}>
          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary text-[11px] font-bold text-primary-foreground">
            SP
          </div>
          {!collapsed && (
            <span className="ml-2.5 truncate text-sm font-semibold text-foreground">{siteTitle}</span>
          )}
        </div>

        {renderNavItems(undefined, collapsed)}
        {renderSidebarFooter(collapsed)}

        {/* 折叠按钮 */}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="flex h-10 items-center justify-center border-t border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          aria-label={collapsed ? '展开侧边栏' : '折叠侧边栏'}
        >
          <svg
            className={`h-4 w-4 transition-transform duration-200 ${collapsed ? 'rotate-180' : ''}`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M11 19l-7-7 7-7M19 19l-7-7 7-7" />
          </svg>
        </button>
      </aside>

      {/* 主区域 */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* 顶栏 */}
        <header className="sticky top-0 z-40 flex h-14 items-center justify-between border-b border-border bg-card px-4 sm:px-6">
          {/* 左侧：移动端菜单 + 标题 */}
          <div className="flex items-center gap-3">
            <button
              onClick={() => setMobileNavOpen(!mobileNavOpen)}
              className="flex h-9 w-9 items-center justify-center rounded-md border border-border text-foreground transition-colors hover:bg-muted md:hidden"
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
            <h1 className="text-base font-semibold tracking-tight text-foreground sm:text-lg">
              {siteTitle}
            </h1>
          </div>

          {/* 右侧：WS 状态 + 在线计数 + ThemeToggle + 退出 */}
          <div className="flex items-center gap-2 sm:gap-3">
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <StatusDot online={wsConnected} />
              <span className="hidden lg:inline">{wsConnected ? '实时连接' : '已断开'}</span>
            </div>

            <div className="hidden items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground sm:flex">
              <span className="font-mono font-medium text-success">{onlineCount}</span>
              <span>/</span>
              <span className="font-mono">{totalCount}</span>
              <span className="ml-0.5">在线</span>
            </div>

            <ThemeToggle />

            <button
              onClick={handleLogout}
              className="flex h-9 items-center gap-1.5 rounded-md border border-border px-2.5 text-[13px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:px-3"
            >
              <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                {Icon.logout}
              </svg>
              <span className="hidden sm:inline">退出</span>
            </button>
          </div>
        </header>

        {/* 主内容 */}
        <main className="flex-1 p-4 sm:p-6">
          <div className="mx-auto max-w-[1400px] animate-fade-in">
            <Outlet />
          </div>
        </main>

        {/* 页脚 */}
        <footer className="border-t border-border px-6 py-4 text-center text-xs text-muted-foreground">
          纯只读安全探针 v1.0.0
        </footer>
      </div>

      {/* 移动端侧边栏 - 抽屉式 */}
      {mobileNavOpen && (
        <div className="fixed inset-0 z-50 md:hidden">
          <div
            className="absolute inset-0 bg-black/50 backdrop-blur-sm"
            onClick={() => setMobileNavOpen(false)}
          />
          <aside className="absolute left-0 top-0 flex h-full w-64 max-w-[85vw] flex-col border-r border-border bg-card">
            <div className="flex h-14 shrink-0 items-center justify-between border-b border-border px-4">
              <div className="flex items-center gap-2.5">
                <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-[11px] font-bold text-primary-foreground">
                  SP
                </div>
                <span className="text-sm font-semibold text-foreground">{siteTitle}</span>
              </div>
              <button
                onClick={() => setMobileNavOpen(false)}
                className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted"
                aria-label="关闭菜单"
              >
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
            {renderNavItems(() => setMobileNavOpen(false))}
            {renderSidebarFooter()}
          </aside>
        </div>
      )}
    </div>
  )
}
