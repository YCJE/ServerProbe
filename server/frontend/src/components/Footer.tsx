/**
 * 页脚组件
 * NodeGet 风格：border-t + 半透明背景 + 毛玻璃
 */
export default function Footer() {
  return (
    <footer className="relative z-10 border-t border-border/70 bg-background/70 backdrop-blur-sm">
      <div className="mx-auto flex max-w-[91.5rem] justify-between gap-4 px-4 py-4 text-xs text-muted-foreground sm:px-6">
        <p>纯只读安全探针 v1.0.0</p>
        <div className="flex gap-4">
          <a href="/" className="transition-colors hover:text-primary">
            公开页
          </a>
        </div>
      </div>
    </footer>
  )
}
