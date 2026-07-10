import { useEffect, useState } from 'react'

/**
 * 动态网格/点阵背景组件
 * fixed 定位，z-0，pointer-events-none
 * 支持网格和点阵两种模式，颜色随主题切换
 */
export default function Background() {
  const [isDark, setIsDark] = useState(() => document.documentElement.classList.contains('dark'))

  useEffect(() => {
    const observer = new MutationObserver(() => {
      setIsDark(document.documentElement.classList.contains('dark'))
    })
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
    return () => observer.disconnect()
  }, [])

  // 背景色：浅色 #f5f8fb，深色 #111827
  const bgColor = isDark ? 'rgb(17 24 39)' : 'rgb(245 248 251)'
  // 网格线颜色：浅色 rgba(183,196,214,X)，深色 rgba(148,163,184,X)
  const lineColor = isDark
    ? 'rgba(148, 163, 184, 0.08)'
    : 'rgba(183, 196, 214, 0.08)'

  return (
    <div
      className="fixed inset-0 z-0 pointer-events-none"
      style={{
        backgroundColor: bgColor,
        backgroundImage: `
          linear-gradient(${lineColor} 1px, transparent 1px),
          linear-gradient(90deg, ${lineColor} 1px, transparent 1px)
        `,
        backgroundSize: '22px 22px',
      }}
      aria-hidden="true"
    />
  )
}
