import { useEffect, useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import PublicDashboard from '@/pages/PublicDashboard'

/**
 * 分享页视图：根据 shareId 渲染公开仪表盘。
 * 当前实现复用 PublicDashboard 组件，未来可根据 SharePage.agent_ids 做过滤。
 */
export default function SharePageView() {
  const { shareId } = useParams<{ shareId: string }>()
  const [valid, setValid] = useState<boolean | null>(null)

  useEffect(() => {
    if (!shareId) {
      setValid(false)
      return
    }
    // 通过公开 API 验证 shareId 是否有效
    fetch(`/api/v1/public/share/${shareId}`)
      .then((res) => {
        if (res.ok) {
          setValid(true)
        } else {
          setValid(false)
        }
      })
      .catch(() => setValid(false))
  }, [shareId])

  if (valid === null) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    )
  }

  if (!valid) {
    return <Navigate to="/" replace />
  }

  // 复用 PublicDashboard 组件渲染公开数据
  return <PublicDashboard />
}
