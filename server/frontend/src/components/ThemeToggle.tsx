import { useState, useRef, useEffect } from 'react'
import { useServerStore } from '@/store/useServerStore'
import type { Theme } from '@/types'

/** 主题切换组件（浅色/深色/跟随系统） - NodeGet 风格 */
export default function ThemeToggle() {
  const theme = useServerStore((s) => s.theme)
  const setTheme = useServerStore((s) => s.setTheme)
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // 点击外部关闭下拉菜单
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // 组件卸载时清理未完成的 setTimeout，避免内存泄漏与对已卸载组件的操作
  useEffect(() => () => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
  }, [])

  const options: { value: Theme; label: string; icon: string }[] = [
    { value: 'dark', label: '深色', icon: '☾' },
    { value: 'light', label: '浅色', icon: '☀' },
    { value: 'system', label: '跟随系统', icon: '⌂' },
  ]

  const current = options.find((o) => o.value === theme) || options[0]

  /** 切换主题：添加 .theme-changing 禁用过渡，90ms 后移除 */
  const handleChange = (value: Theme) => {
    document.documentElement.classList.add('theme-changing')
    setTheme(value)
    if (timeoutRef.current) clearTimeout(timeoutRef.current)
    timeoutRef.current = setTimeout(() => {
      document.documentElement.classList.remove('theme-changing')
    }, 90)
    setOpen(false)
  }

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="flex h-9 w-9 items-center justify-center rounded-md border border-border bg-card text-foreground transition-colors hover:bg-accent"
        title="切换主题"
        aria-label="切换主题"
        aria-expanded={open}
      >
        <span className="text-base">{current.icon}</span>
      </button>

      {open && (
        <div className="animate-scale-in absolute right-0 top-full z-50 mt-2 w-36 rounded-lg border border-border bg-popover py-1 shadow-md">
          {options.map((option) => (
            <button
              key={option.value}
              onClick={() => handleChange(option.value)}
              className={`flex w-full items-center gap-2 px-2.5 py-2 text-sm transition-colors hover:bg-accent ${
                theme === option.value
                  ? 'font-medium text-foreground'
                  : 'text-muted-foreground'
              }`}
            >
              <span className="w-4 text-center">{option.icon}</span>
              <span>{option.label}</span>
              {theme === option.value && (
                <svg
                  className="ml-auto h-4 w-4 text-primary"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2.5}
                    d="M5 13l4 4L19 7"
                  />
                </svg>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
