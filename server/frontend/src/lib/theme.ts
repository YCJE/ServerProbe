/**
 * CSS 变量 → 可用于 ECharts/SVG 的颜色字符串
 * 变量定义格式为 "H S% L%"（见 index.css），hsl() 包裹后输出。
 * 每次调用实时读取（不缓存），保证主题切换后随重渲染取到新值。
 */
export function cssColor(name: string): string {
  if (typeof window === 'undefined') return 'transparent'
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value ? `hsl(${value})` : 'transparent'
}

/** 带 alpha 的 CSS 变量颜色（ECharts 面积渐变等场景） */
export function cssColorAlpha(name: string, alpha: number): string {
  if (typeof window === 'undefined') return 'transparent'
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value ? `hsl(${value} / ${alpha})` : 'transparent'
}
