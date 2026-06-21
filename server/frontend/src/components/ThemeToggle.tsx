import { useState, useRef, useEffect } from 'react'
import { useServerStore } from '@/store/useServerStore'
import type { Theme } from '@/types'

/** 主题切换组件（浅色/深色/跟随系统） */
export default function ThemeToggle() {
  const theme = useServerStore((s) => s.theme)
  const setTheme = useServerStore((s) => s.setTheme)
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

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

  const options: { value: Theme; label: string; icon: string }[] = [
    { value: 'light', label: '浅色', icon: '☀' },
    { value: 'dark', label: '深色', icon: '☾' },
    { value: 'system', label: '跟随系统', icon: '⌂' },
  ]

  const current = options.find((o) => o.value === theme) || options[2]

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="flex h-9 w-9 items-center justify-center rounded-lg border border-border bg-card text-foreground transition-colors hover:bg-accent"
        title="切换主题"
      >
        <span className="text-base">{current.icon}</span>
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-2 w-36 rounded-lg border border-border bg-card p-1 shadow-lg animate-fade-in">
          {options.map((option) => (
            <button
              key={option.value}
              onClick={() => {
                setTheme(option.value)
                setOpen(false)
              }}
              className={`flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent ${
                theme === option.value
                  ? 'bg-accent font-medium text-foreground'
                  : 'text-muted-foreground'
              }`}
            >
              <span className="w-4 text-center">{option.icon}</span>
              <span>{option.label}</span>
              {theme === option.value && (
                <span className="ml-auto text-primary">✓</span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
