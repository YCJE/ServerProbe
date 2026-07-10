import { useEffect, useRef, useState } from 'react'

/** 缓动函数：ease-out（cubic） */
function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3)
}

export interface AnimatedNumberResult {
  /** 当前显示的数值（动画过程中会平滑过渡） */
  value: number
  /** 是否正在动画中 */
  animating: boolean
}

/**
 * 数值动画 hook
 *
 * 当目标数值变化时，使用 requestAnimationFrame 平滑过渡到新值。
 * - 缓动函数：ease-out（cubic）
 * - 过渡时长：0.5s
 *
 * @param target 目标数值
 * @returns { value, animating } 当前显示值 + 是否动画中
 */
export function useAnimatedNumber(target: number): AnimatedNumberResult {
  const [displayValue, setDisplayValue] = useState<number>(target)
  const [animating, setAnimating] = useState<boolean>(false)

  // 持久化引用，避免重渲染丢失
  // displayValueRef 始终跟踪当前显示值（包括动画过程中每一帧的值），
  // 这样当 target 在动画进行中再次变化时，新动画能从当前实际显示值开始，
  // 避免从旧起始值开始导致数值"回跳"
  const displayValueRef = useRef<number>(target)
  const toRef = useRef<number>(target)
  const startRef = useRef<number>(0)
  const rafRef = useRef<number>(0)
  const durationRef = useRef<number>(500)

  // 目标变化时启动动画
  useEffect(() => {
    // 取消上一次未完成的动画
    if (rafRef.current) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = 0
    }

    // 从当前实际显示值开始，保证动画连续、不回跳
    const from = displayValueRef.current
    const to = target

    // 数值未变化，直接返回
    if (from === to) {
      displayValueRef.current = to
      setDisplayValue(to)
      setAnimating(false)
      toRef.current = to
      return
    }

    toRef.current = to
    startRef.current = performance.now()
    setAnimating(true)

    const tick = (now: number) => {
      const elapsed = now - startRef.current
      const progress = Math.min(elapsed / durationRef.current, 1)
      const eased = easeOutCubic(progress)
      const current = from + (to - from) * eased

      // 同步更新 displayValueRef，使下次动画起始值始终为当前显示值
      displayValueRef.current = current
      setDisplayValue(current)

      if (progress < 1) {
        rafRef.current = requestAnimationFrame(tick)
      } else {
        // 动画结束
        rafRef.current = 0
        setAnimating(false)
      }
    }

    rafRef.current = requestAnimationFrame(tick)

    return () => {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = 0
      }
    }
    // 仅依赖目标值变化
  }, [target])

  // 卸载时清理
  useEffect(() => {
    return () => {
      if (rafRef.current) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = 0
      }
    }
  }, [])

  return { value: displayValue, animating }
}

export default useAnimatedNumber
