import { Outlet } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from './ThemeToggle'

/** 公开页面布局组件（顶部导航栏 + 主内容区） */
export default function PublicLayout() {
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)
  const servers = useServerStore((s) => s.servers)

  const onlineCount = servers.filter((s) => s.online).length

  return (
    <div className="min-h-screen bg-background">
      {/* 顶部导航 */}
      <header className="sticky top-0 z-50 border-b border-border bg-card/95 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center justify-between px-4">
          {/* Logo */}
          <div className="flex items-center gap-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-primary-foreground font-bold text-sm">
              SP
            </div>
            <span className="text-lg font-semibold text-foreground">服务器探针</span>
          </div>

          {/* 右侧操作区 */}
          <div className="flex items-center gap-3">
            {/* WebSocket 连接状态 */}
            <div className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:flex">
              <span
                className={`inline-block h-2 w-2 rounded-full ${
                  publicWsConnected ? 'bg-success animate-pulse' : 'bg-destructive'
                }`}
              />
              <span>{publicWsConnected ? '实时' : '断开'}</span>
            </div>

            {/* 在线/总数 */}
            <div className="hidden items-center gap-1.5 rounded-lg bg-secondary px-3 py-1 text-xs text-secondary-foreground sm:flex">
              <span className="font-medium text-success">{onlineCount}</span>
              <span>/</span>
              <span>{servers.length}</span>
              <span className="ml-1">在线</span>
            </div>

            <ThemeToggle />

            {isAuthenticated ? (
              <a
                href="/admin"
                className="flex h-8 items-center rounded-lg border border-border px-3 text-sm text-foreground transition-colors hover:bg-accent"
              >
                后台管理
              </a>
            ) : (
              <a
                href="/login"
                className="flex h-8 items-center rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
              >
                后台管理
              </a>
            )}
          </div>
        </div>
      </header>

      {/* 主内容 */}
      <main className="mx-auto max-w-7xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
