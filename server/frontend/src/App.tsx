import { useEffect } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import ErrorBoundary from '@/components/ErrorBoundary'
import PublicLayout from '@/components/PublicLayout'
import Layout from '@/components/Layout'
import Login from '@/pages/Login'
import Setup from '@/pages/Setup'
import PublicDashboard from '@/pages/PublicDashboard'
import PublicServerDetail from '@/pages/PublicServerDetail'
import Dashboard from '@/pages/Dashboard'
import ServerDetail from '@/pages/ServerDetail'
import AgentManagement from '@/pages/AgentManagement'
import PingTargets from '@/pages/PingTargets'
import AlertManagement from '@/pages/AlertManagement'
import NotifyChannels from '@/pages/NotifyChannels'
import SystemStatus from '@/pages/SystemStatus'
import LogViewer from '@/pages/LogViewer'

function App() {
  const initTheme = useServerStore((s) => s.initTheme)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const authInitialized = useServerStore((s) => s.authInitialized)
  const needsSetup = useServerStore((s) => s.needsSetup)
  const checkSetupStatus = useServerStore((s) => s.checkSetupStatus)
  const checkAuth = useServerStore((s) => s.checkAuth)
  const location = useLocation()

  // 初始化主题
  useEffect(() => {
    initTheme()
  }, [initTheme])

  // 检查是否需要初始化 + 检查登录状态（Cookie 认证，需异步确认）
  useEffect(() => {
    checkSetupStatus()
    checkAuth()
  }, [checkSetupStatus, checkAuth])

  // 初始化检查完成前显示加载状态，防止首屏闪烁（已登录用户被误重定向到 /login）
  if (!authInitialized && !needsSetup) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    )
  }

  // 未初始化时所有路由都指向 Setup
  // WebSocket 连接管理已移至 Layout.tsx，避免 admin 路由间导航导致 WS 断开重连
  const routes = needsSetup ? (
    <Routes>
      <Route path="*" element={<Setup />} />
    </Routes>
  ) : (
    <Routes>
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
        <Route path="ping-targets" element={<PingTargets />} />
        <Route path="alerts" element={<AlertManagement />} />
        <Route path="notify" element={<NotifyChannels />} />
        <Route path="system" element={<SystemStatus />} />
        <Route path="logs" element={<LogViewer />} />
        <Route path="server/:id" element={<ServerDetail />} />
      </Route>

      {/* 兜底 */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )

  // 使用 location.pathname 作为 ErrorBoundary 的 resetKey，路由切换时清除错误状态，
  // 不使用 key 触发重挂载，避免子树（含 Layout/WS 连接）卸载重建
  return (
    <ErrorBoundary resetKey={location.pathname}>
      {routes}
    </ErrorBoundary>
  )
}

export default App
