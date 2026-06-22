import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
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
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const needsSetup = useServerStore((s) => s.needsSetup)
  const checkSetupStatus = useServerStore((s) => s.checkSetupStatus)

  // 初始化主题
  useEffect(() => {
    initTheme()
  }, [initTheme])

  // 检查是否需要初始化
  useEffect(() => {
    checkSetupStatus()
  }, [checkSetupStatus])

  // 未初始化时所有路由都指向 Setup
  // WebSocket 连接管理已移至 Layout.tsx，避免 admin 路由间导航导致 WS 断开重连
  if (needsSetup) {
    return (
      <Routes>
        <Route path="*" element={<Setup />} />
      </Routes>
    )
  }

  return (
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
        <Route path="server/:id" element={<ServerDetail />} />
      </Route>

      {/* 兜底 */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App
