/** 格式化字节大小为人类可读字符串 */
export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes == null || bytes <= 0 || !isFinite(bytes)) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1)
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`
}

/** 格式化速率（字节/秒 -> 人类可读） */
export function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec === 0 || bytesPerSec == null) return '0 B/s'
  return `${formatBytes(bytesPerSec)}/s`
}

/** 格式化运行时间（秒 -> 天时分） */
export function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return '-'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)

  if (days > 0) {
    return `${days}天 ${hours}小时`
  }
  if (hours > 0) {
    return `${hours}小时 ${minutes}分`
  }
  return `${minutes}分钟`
}

/** 格式化时间戳为相对时间 */
export function formatRelativeTime(timestamp: number): string {
  if (!timestamp) return '-'
  const now = Date.now()
  const diff = now - timestamp * 1000
  const seconds = Math.floor(diff / 1000)

  if (seconds < 60) return '刚刚'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟前`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}小时前`
  return `${Math.floor(seconds / 86400)}天前`
}

/** 根据使用率返回颜色 class */
export function getUsageColor(usage: number): string {
  if (usage >= 90) return 'bg-destructive'
  if (usage >= 80) return 'bg-warning'
  if (usage >= 60) return 'bg-primary'
  return 'bg-success'
}

/** 根据使用率返回文本颜色 class */
export function getUsageTextColor(usage: number): string {
  if (usage >= 90) return 'text-destructive'
  if (usage >= 80) return 'text-warning'
  return 'text-foreground'
}

/** 根据丢包率返回颜色 class */
export function getLossColor(loss: number): string {
  if (loss > 20) return 'text-destructive'
  if (loss > 0) return 'text-warning'
  return 'text-muted-foreground'
}

/** 格式化延迟 */
export function formatLatency(latency: number): string {
  if (latency == null || latency < 0) return '---'
  return `${latency.toFixed(1)} ms`
}

/** 格式化丢包率 */
export function formatLoss(loss: number): string {
  if (loss == null || loss < 0) return '---'
  return `${loss.toFixed(1)}%`
}
