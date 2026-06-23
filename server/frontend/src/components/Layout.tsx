import { useEffect, useState } from 'react'
import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from './ThemeToggle'

/** 布局组件（顶栏 + 侧边栏 + 主内容区） */
export default function Layout() {
  const navigate = useNavigate()
  const logout = useServerStore((s) => s.logout)
  const wsConnected = useServerStore((s) => s.wsConnected)
  const fetchServers = useServerStore((s) => s.fetchServers)
  const connectWebSocket = useServerStore((s) => s.connectWebSocket)
  const disconnectWebSocket = useServerStore((s) => s.disconnectWebSocket)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const servers = useServerStore((s) => s.servers)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  // 首次加载时获取服务器列表并连接 WebSocket
  useEffect(() => {
    if (isAuthenticated) {
      fetchServers().catch(() => {
        // 错误处理在 API 层已做
      })
      connectWebSocket()
      return () => {
        disconnectWebSocket()
      }
    }
  }, [isAuthenticated, fetchServers, connectWebSocket, disconnectWebSocket])

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  const onlineCount = servers.filter((s) => s.online).length
  const totalCount = servers.length

  const navGroups = [
    {
      title: '管理',
      items: [
        { to: '/admin', label: '仪表盘', icon: '▣', end: true },
        { to: '/admin/agents', label: 'Agent 管理', icon: '⬡', end: false },
        { to: '/admin/ping-targets', label: '探测目标', icon: '◈', end: false },
        { to: '/admin/alerts', label: '告警管理', icon: '⚠', end: false },
        { to: '/admin/notify', label: '通知渠道', icon: '✉', end: false },
        { to: '/admin/system', label: '系统状态', icon: '⚙', end: false },
      ],
    },
  ]

  return (
    <div className="flex h-screen flex-col bg-background">
      {/* 顶栏 */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-border bg-card px-3 sm:px-4">
        <div className="flex items-center gap-2 sm:gap-3">
          {/* 移动端菜单按钮 */}
          <button
            onClick={() => setMobileNavOpen(!mobileNavOpen)}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-border text-foreground transition-colors hover:bg-accent md:hidden"
          >
            <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              {mobileNavOpen ? (
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
              )}
            </svg>
          </button>
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground font-bold text-sm">
              SP
            </div>
            <span className="text-base font-semibold text-foreground sm:text-lg">服务器探针</span>
          </div>
        </div>

        <div className="flex items-center gap-2 sm:gap-3">
          {/* WebSocket 连接状态 */}
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                wsConnected ? 'bg-success animate-pulse' : 'bg-destructive'
              }`}
            />
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
            className="flex h-9 items-center rounded-lg border border-border px-2 text-sm text-foreground transition-colors hover:bg-accent sm:px-3"
          >
            退出
          </button>
        </div>
      </header>

      <div className="flex flex-1 overflow-hidden">
        {/* 侧边栏 - 桌面端 */}
        <aside className="relative hidden w-56 shrink-0 border-r border-border bg-card md:block">
          <nav className="flex flex-col gap-4 p-3">
            {navGroups.map((group) => (
              <div key={group.title} className="flex flex-col gap-1">
                <h3 className="px-3 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
                  {group.title}
                </h3>
                {group.items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.end}
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

          {/* 侧边栏底部信息 */}
          <div className="absolute bottom-0 left-0 w-56 border-t border-border p-3 text-xs text-muted-foreground">
            <a
              href="/"
              className="mb-2 flex items-center gap-1.5 rounded-md px-2 py-1.5 text-foreground transition-colors hover:bg-accent"
            >
              <svg className="h-3.5 w-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>
              返回公开页
            </a>
            <p>纯只读安全探针</p>
            <p className="mt-1 text-muted-foreground/60">v1.0.0</p>
          </div>
        </aside>

        {/* 移动端侧边栏 - 抽屉式 */}
        {mobileNavOpen && (
          <>
            {/* 遮罩 */}
            <div
              className="fixed inset-0 z-40 bg-black/50 md:hidden"
              onClick={() => setMobileNavOpen(false)}
            />
            {/* 抽屉 */}
            <aside className="fixed left-0 top-14 z-50 h-[calc(100vh-3.5rem)] w-64 border-r border-border bg-card shadow-xl md:hidden">
              <nav className="flex flex-col gap-4 p-3">
                {navGroups.map((group) => (
                  <div key={group.title} className="flex flex-col gap-1">
                    <h3 className="px-3 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">
                      {group.title}
                    </h3>
                    {group.items.map((item) => (
                      <NavLink
                        key={item.to}
                        to={item.to}
                        end={item.end}
                        onClick={() => setMobileNavOpen(false)}
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
              <div className="absolute bottom-0 left-0 w-full border-t border-border p-3 text-xs text-muted-foreground">
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
          </>
        )}

        {/* 主内容区 */}
        <main className="flex-1 overflow-y-auto scrollbar-thin">
          <div className="mx-auto max-w-7xl p-3 sm:p-4 md:p-6">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
