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
          ? 'bg-emerald-500 ring-2 ring-emerald-500/25'
          : 'bg-rose-500 ring-2 ring-rose-500/25'
      }`}
      aria-label={online ? '在线' : '离线'}
    />
  )
}
