import { useEffect, useMemo } from 'react'
import { Outlet } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import { useSiteSettings } from '@/store/useSiteSettingsStore'
import ThemeToggle from './ThemeToggle'
import Footer from './Footer'
import StatusDot from './StatusDot'

/**
 * 公开页面布局组件 - NodeGet 风格
 * 扁平全宽顶栏（描边分隔）+ 公告横幅 + 极淡网格背景 + Footer
 */
export default function PublicLayout() {
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)
  const servers = useServerStore((s) => s.servers)
  const connectPublicDashboardWS = useServerStore((s) => s.connectPublicDashboardWS)
  const disconnectPublicDashboardWS = useServerStore((s) => s.disconnectPublicDashboardWS)
  const { siteTitle, announcement } = useSiteSettings()

  // 在 Layout 层管理 WS 连接，确保页面切换时 WS 不会被断开
  useEffect(() => {
    connectPublicDashboardWS()
    return () => disconnectPublicDashboardWS()
  }, [connectPublicDashboardWS, disconnectPublicDashboardWS])

  const onlineCount = useMemo(() => servers.filter((s) => s.online).length, [servers])

  return (
    <div className="flex min-h-screen flex-col">
      {/* 扁平顶栏（NodeGet: sticky 全宽，border-b 分隔） */}
      <header className="sticky top-0 z-40 border-b border-border bg-card">
        <div className="mx-auto flex h-14 max-w-[91.5rem] items-center justify-between px-4 sm:px-6">
          {/* 左侧: Logo + 标题 */}
          <div className="flex items-center gap-2.5">
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary text-[11px] font-bold text-primary-foreground">
              SP
            </div>
            <span className="text-sm font-semibold tracking-tight text-foreground sm:text-base">
              {siteTitle}
            </span>

            {/* 移动端紧凑在线计数（标题右侧，仅 n/m） */}
            <div className="flex items-center gap-1 rounded-md border border-border px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground sm:hidden">
              <span className="font-mono font-medium text-success">{onlineCount}</span>
              <span>/</span>
              <span className="font-mono">{servers.length}</span>
            </div>
          </div>

          {/* 右侧: 在线计数 + WS 状态 + 主题切换 + 后台入口 */}
          <div className="flex items-center gap-2 sm:gap-3">
            <div className="hidden items-center gap-1 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground sm:flex">
              <span className="font-mono font-medium text-success">{onlineCount}</span>
              <span>/</span>
              <span className="font-mono">{servers.length}</span>
              <span className="ml-0.5">在线</span>
            </div>

            <div className="hidden items-center gap-1.5 text-xs text-muted-foreground lg:flex">
              <StatusDot online={publicWsConnected} />
              <span>{publicWsConnected ? '已连接' : '已断开'}</span>
            </div>

            <ThemeToggle />

            <a
              href={isAuthenticated ? '/admin' : '/login'}
              className={`flex h-9 items-center rounded-md px-3 text-[13px] font-medium transition-colors ${
                isAuthenticated
                  ? 'border border-border bg-card text-foreground hover:bg-accent'
                  : 'bg-primary text-primary-foreground hover:opacity-90'
              }`}
            >
              管理后台
            </a>
          </div>
        </div>
      </header>

      {/* 公告横幅（后台"站点设置"中配置） */}
      {announcement && (
        <div className="border-b border-border bg-primary/5">
          <div className="mx-auto flex max-w-[91.5rem] items-start gap-2 px-4 py-2 text-xs text-foreground/80 sm:px-6">
            <svg className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5.9h.01M4.5 3h15a1.5 1.5 0 011.5 1.5v12A1.5 1.5 0 0119.5 18h-15A1.5 1.5 0 013 16.5v-12A1.5 1.5 0 014.5 3zM11 9v5" />
            </svg>
            <p className="max-h-[40vh] overflow-y-auto whitespace-pre-wrap break-words leading-relaxed scrollbar-thin">
              {announcement}
            </p>
          </div>
        </div>
      )}

      {/* 主内容区（极淡网格背景） */}
      <div className="bg-soft flex-1">
        <main className="mx-auto max-w-[91.5rem] px-4 py-6 sm:px-6">
          <Outlet />
        </main>
      </div>

      {/* 底部 */}
      <Footer />
    </div>
  )
}
