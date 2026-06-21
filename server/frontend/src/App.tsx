import { useEffect } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import PublicLayout from '@/components/PublicLayout'
import Layout from '@/components/Layout'
import Login from '@/pages/Login'
import Setup from '@/pages/Setup'
import PublicDashboard from '@/pages/PublicDashboard'
import PublicServerDetail from '@/pages/PublicServerDetail'
import Dashboard from '@/pages/Dashboard'
import ServerDetail from '@/pages/ServerDetail'
import AgentManagement from '@/pages/AgentManagement'

function App() {
  const initTheme = useServerStore((s) => s.initTheme)
  const checkSetupStatus = useServerStore((s) => s.checkSetupStatus)
  const connectWebSocket = useServerStore((s) => s.connectWebSocket)
  const disconnectWebSocket = useServerStore((s) => s.disconnectWebSocket)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const needsSetup = useServerStore((s) => s.needsSetup)
  const authLoading = useServerStore((s) => s.authLoading)
  const location = useLocation()

  // 初始化主题
  useEffect(() => {
    initTheme()
  }, [initTheme])

  // 检查是否需要初始化
  useEffect(() => {
    checkSetupStatus()
  }, [checkSetupStatus])

  // 仅在访问管理后台相关页面时连接需要认证的 WebSocket
  useEffect(() => {
    const isAdminRoute = location.pathname.startsWith('/admin')
    if (isAuthenticated && !needsSetup && isAdminRoute) {
      connectWebSocket()
      return () => {
        disconnectWebSocket()
      }
    }
  }, [isAuthenticated, needsSetup, location.pathname, connectWebSocket, disconnectWebSocket])

  // 仅在首次初始化检查时显示加载状态（不影响公开页面访问）
  if (authLoading && needsSetup) {
    return (
      <div className="flex h-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          <p className="text-sm text-muted-foreground">加载中...</p>
        </div>
      </div>
    )
  }

  return (
    <Routes>
      {/* 初始化设置（未初始化时所有路由都指向 Setup） */}
      {needsSetup && <Route path="*" element={<Setup />} />}

      {/* 公开页面 (无需登录) */}
      <Route element={<PublicLayout />}>
        <Route path="/" element={<PublicDashboard />} />
        <Route path="/server/:id" element={<PublicServerDetail />} />
      </Route>

      {/* 登录页 */}
      <Route
        path="/login"
        element={isAuthenticated ? <Navigate to="/admin" replace /> : <Login />}
      />

      {/* 管理后台 (需要登录) */}
      <Route
        path="/admin"
        element={isAuthenticated ? <Layout /> : <Navigate to="/login" replace />}
      >
        <Route index element={<Dashboard />} />
        <Route path="agents" element={<AgentManagement />} />
        <Route path="server/:id" element={<ServerDetail />} />
      </Route>

      {/* 兜底 */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App
