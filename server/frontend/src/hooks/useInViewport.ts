import { useEffect, useRef, useState } from 'react'

export interface InViewportResult<T extends HTMLElement = HTMLDivElement> {
  /** 附加到目标元素的 ref */
  ref: React.RefObject<T | null>
  /** 元素是否在视口内（考虑 margin 扩展范围） */
  isInViewport: boolean
}

/**
 * 视口懒加载 hook
 *
 * 使用 IntersectionObserver 监听元素是否在视口内。
 * 通过 rootMargin 扩展判断范围，让元素在即将进入视口时即开始加载。
 *
 * @param margin rootMargin，默认 320px（提前 320px 加载）
 * @returns { ref, isInViewport }
 */
export function useInViewport<T extends HTMLElement = HTMLDivElement>(
  margin: number = 320,
): InViewportResult<T> {
  const ref = useRef<T | null>(null)
  const [isInViewport, setIsInViewport] = useState<boolean>(false)

  useEffect(() => {
    const el = ref.current
    if (!el) return

    // 如果浏览器不支持 IntersectionObserver，降级为始终可见
    if (typeof IntersectionObserver === 'undefined') {
      setIsInViewport(true)
      return
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          // 一旦进入视口即标记为可见，并保持（避免反复触发）
          if (entry.isIntersecting) {
            setIsInViewport(true)
            // 已经加载过，可以停止观察
            observer.disconnect()
          }
        }
      },
      {
        // rootMargin: 提前 margin 像素触发
        rootMargin: `${margin}px 0px ${margin}px 0px`,
        threshold: 0,
      },
    )

    observer.observe(el)

    return () => {
      observer.disconnect()
    }
  }, [margin])

  return { ref, isInViewport }
}

export default useInViewport
