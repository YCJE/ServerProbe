/**
 * 在线状态指示点
 * 绿色=在线，红色=离线，带 ring 光环
 */
export default function StatusDot({ online, size = 'sm' }: { online: boolean; size?: 'sm' | 'md' }) {
  const dim = size === 'md' ? 'h-2.5 w-2.5' : 'h-2 w-2'
  return (
    <span
      className={`inline-block ${dim} shrink-0 rounded-full ${
        online
          ? 'bg-success ring-2 ring-success/25'
          : 'bg-destructive ring-2 ring-destructive/25'
      }`}
      aria-label={online ? '在线' : '离线'}
    />
  )
}
