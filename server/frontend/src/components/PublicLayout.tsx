import { useMemo } from 'react'
import { Outlet } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ThemeToggle from './ThemeToggle'

/**
 * 公开页面布局组件
 * 顶部导航栏采用 glass 毛玻璃效果，包含 Logo、在线数/总数指示器、
 * WebSocket 连接状态、主题切换和后台管理入口。
 * 移动端隐藏在线数和 WS 状态文字，仅保留 Logo + 主题切换 + 后台按钮。
 */
export default function PublicLayout() {
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const publicWsConnected = useServerStore((s) => s.publicWsConnected)
  const servers = useServerStore((s) => s.servers)

  const onlineCount = useMemo(() => servers.filter((s) => s.online).length, [servers])

  return (
    <div className="min-h-screen bg-background">
      {/* 顶部导航栏 - glass 毛玻璃效果 */}
      <header className="glass sticky top-0 z-50 border-b border-border">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between gap-4 px-4">
          {/* 左侧: Logo 图标 + 标题 */}
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-primary to-apple-cyan shadow-sm">
              <svg
                className="h-5 w-5 text-primary-foreground"
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
            <span className="text-base font-semibold text-foreground sm:text-lg">
              服务器探针
            </span>
          </div>

          {/* 中间: 在线数/总数 指示器（药丸形状） */}
          <div className="hidden items-center gap-2 rounded-full border border-border bg-secondary/50 px-3 py-1.5 md:flex">
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                publicWsConnected ? 'bg-success animate-pulse' : 'bg-muted-foreground'
              }`}
            />
            <span className="text-sm">
              <span className="font-semibold text-success">{onlineCount}</span>
              <span className="text-muted-foreground"> / {servers.length} 在线</span>
            </span>
          </div>

          {/* 右侧: WS 状态 + 主题切换 + 后台管理 */}
          <div className="flex items-center gap-2 sm:gap-3">
            {/* WebSocket 连接状态 */}
            <div className="hidden items-center gap-1.5 text-xs text-muted-foreground lg:flex">
              <span
                className={`inline-block h-2 w-2 rounded-full ${
                  publicWsConnected ? 'bg-success animate-pulse' : 'bg-destructive'
                }`}
              />
              <span>{publicWsConnected ? '已连接' : '已断开'}</span>
            </div>

            <ThemeToggle />

            {isAuthenticated ? (
              <a
                href="/admin"
                className="flex h-9 items-center rounded-lg border border-border bg-card px-3 text-sm font-medium text-foreground transition-colors hover:bg-accent"
              >
                后台管理
              </a>
            ) : (
              <a
                href="/login"
                className="flex h-9 items-center rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
              >
                后台管理
              </a>
            )}
          </div>
        </div>
      </header>

      {/* 主内容区 */}
      <main className="mx-auto max-w-7xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
