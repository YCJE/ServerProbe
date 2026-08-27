import { useSiteSettingsStore } from '@/store/useSiteSettingsStore'

/**
 * 页脚组件
 * NodeGet 风格：border-t 分隔，支持后台自定义页脚文字
 */
export default function Footer() {
  const customFooter = useSiteSettingsStore((s) => s.customFooter)

  return (
    <footer className="border-t border-border">
      <div className="mx-auto flex max-w-[91.5rem] flex-col gap-2 px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:px-6">
        <p>{customFooter || '纯只读安全探针 v1.0.0'}</p>
        <div className="flex gap-4">
          <a href="/" className="transition-colors hover:text-foreground">
            公开页
          </a>
        </div>
      </div>
    </footer>
  )
}
