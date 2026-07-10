import { useEffect, useMemo, useState } from 'react'
import { Outlet } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from './ThemeToggle'
import Background from './Background'
import Footer from './Footer'
import StatusDot from './StatusDot'

/**
 * 公开页面布局组件 - NodeGet 风格
 * 浮动式毛玻璃导航栏 + 网格背景 + Footer
 */
export default function PublicLayout() {
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)
  const servers = useServerStore((s) => s.servers)
  const connectPublicDashboardWS = useServerStore((s) => s.connectPublicDashboardWS)
  const disconnectPublicDashboardWS = useServerStore((s) => s.disconnectPublicDashboardWS)
  const [scrolled, setScrolled] = useState(false)

  // 在 Layout 层管理 WS 连接，确保页面切换时 WS 不会被断开
  useEffect(() => {
    connectPublicDashboardWS()
    return () => disconnectPublicDashboardWS()
  }, [connectPublicDashboardWS, disconnectPublicDashboardWS])

  // 滚动检测：scrollY > 12 时增强导航栏阴影
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 12)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  const onlineCount = useMemo(() => servers.filter((s) => s.online).length, [servers])

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
            {/* 左侧: Logo 图标 + 标题 */}
            <div className="flex items-center gap-2.5">
              <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary text-primary-foreground">
                <svg
                  className="h-5 w-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01"
                  />
                </svg>
              </div>
              <span className="text-base font-bold tracking-wide text-primary sm:text-xl">
                服务器探针
              </span>
            </div>

            {/* 右侧: 在线数药丸 + WS 状态 + 主题切换 + 后台管理 */}
            <div className="flex items-center gap-2 sm:gap-3">
              {/* 在线数/总数 指示器（药丸形状） */}
              <div className="hidden items-center gap-2 rounded-full bg-secondary px-3 py-1 text-xs md:flex">
                <StatusDot online={publicWsConnected} />
                <span>
                  <span className="font-semibold text-success">{onlineCount}</span>
                  <span className="text-muted-foreground"> / {servers.length} 在线</span>
                </span>
              </div>

              {/* WebSocket 连接状态 */}
              <div className="hidden items-center gap-1.5 text-xs text-muted-foreground lg:flex">
                <StatusDot online={publicWsConnected} />
                <span>{publicWsConnected ? '已连接' : '已断开'}</span>
              </div>

              <ThemeToggle />

              {isAuthenticated ? (
                <a
                  href="/admin"
                  className="flex h-9 items-center rounded-lg border border-border bg-secondary px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent"
                >
                  管理后台
                </a>
              ) : (
                <a
                  href="/login"
                  className="flex h-9 items-center rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
                >
                  管理后台
                </a>
              )}
            </div>
          </div>
        </div>
      </header>

      {/* 主内容区 */}
      <div className="relative z-10 pt-[6.9rem] sm:pt-[7.6rem]">
        <main className="mx-auto max-w-[91.5rem] px-4 pb-4 sm:px-6 sm:pb-6">
          <Outlet />
        </main>
      </div>

      {/* 底部 */}
      <Footer />
    </div>
  )
}
