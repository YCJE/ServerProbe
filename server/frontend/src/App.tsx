import { useEffect } from 'react'
import { Routes, Route, Navigate, useLocation } from 'react-router-dom'
import { useServerStore } from '@/store/useServerStore'
import Layout from '@/components/Layout'
import Login from '@/pages/Login'
import Setup from '@/pages/Setup'
import Dashboard from '@/pages/Dashboard'
import ServerDetail from '@/pages/ServerDetail'
import AgentManagement from '@/pages/AgentManagement'

/** 受保护路由：未登录时跳转到登录页 */
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const needsSetup = useServerStore((s) => s.needsSetup)
  const location = useLocation()

  if (needsSetup) {
    return <Navigate to="/setup" replace />
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />
  }

  return <>{children}</>
}

function App() {
  const initTheme = useServerStore((s) => s.initTheme)
  const checkSetupStatus = useServerStore((s) => s.checkSetupStatus)
  const connectWebSocket = useServerStore((s) => s.connectWebSocket)
  const disconnectWebSocket = useServerStore((s) => s.disconnectWebSocket)
  const isAuthenticated = useServerStore((s) => s.isAuthenticated)
  const needsSetup = useServerStore((s) => s.needsSetup)

  // 初始化主题
  useEffect(() => {
    initTheme()
  }, [initTheme])

  // 检查是否需要初始化
  useEffect(() => {
    checkSetupStatus()
  }, [checkSetupStatus])

  // 认证状态变化时连接/断开 WebSocket
  useEffect(() => {
    if (isAuthenticated && !needsSetup) {
      connectWebSocket()
      return () => {
        disconnectWebSocket()
      }
    }
  }, [isAuthenticated, needsSetup, connectWebSocket, disconnectWebSocket])

  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/setup" element={<Setup />} />
      <Route
        path="/"
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route index element={<Dashboard />} />
        <Route path="server/:id" element={<ServerDetail />} />
        <Route path="agents" element={<AgentManagement />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App
