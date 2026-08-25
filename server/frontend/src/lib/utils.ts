import type { ServerData, PingResult } from '@/types'

/** 格式化字节大小为人类可读字符串 */
export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes == null || bytes <= 0 || !isFinite(bytes)) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1)
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`
}

/** 格式化速率（字节/秒 -> 人类可读，如 "1.5 MB/s"） */
export function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec === 0 || bytesPerSec == null || !isFinite(bytesPerSec) || bytesPerSec < 0) return '0 B/s'
  const k = 1024
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s']
  const i = Math.min(Math.floor(Math.log(bytesPerSec) / Math.log(k)), sizes.length - 1)
  return `${parseFloat((bytesPerSec / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

/** 格式化运行时间（秒 -> 短格式，如 "3d 12h", "2h 30m", "5m"） */
export function formatUptime(seconds: number): string {
  if (!seconds || seconds <= 0) return '-'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)

  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${secs}s`
  return `${secs}s`
}

/** 格式化累计流量（字节 -> 人类可读，如 "1.5 GB"） */
export function formatTraffic(bytes: number): string {
  if (bytes == null || bytes <= 0 || !isFinite(bytes)) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1)
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
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
  if (latency == null || isNaN(latency) || latency < 0) return '---'
  return `${latency.toFixed(1)} ms`
}

/** 格式化丢包率 */
export function formatLoss(loss: number): string {
  if (loss == null || isNaN(loss) || loss < 0) return '---'
  return `${loss.toFixed(1)}%`
}

/** 格式化相对时间（如 "刚刚", "2分钟前", "1小时前"） */
export function formatRelativeTime(timestamp: number): string {
  if (!timestamp || timestamp <= 0) return '---'
  // 兼容秒级和毫秒级时间戳
  const ts = timestamp > 1e12 ? timestamp : timestamp * 1000
  const now = Date.now()
  const diff = now - ts
  if (diff < 0) return '刚刚'
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return '刚刚'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}天前`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months}个月前`
  return `${Math.floor(months / 12)}年前`
}

/** 解析 ping_data，兼容 ringbuffer (数组) 和 sqlite (JSON 字符串) 两种格式 */
export function parsePingData(raw: unknown): PingResult[] {
  if (!raw) return []
  if (Array.isArray(raw)) return raw as PingResult[]
  if (typeof raw === 'string') {
    try {
      const parsed = JSON.parse(raw)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
}

/** 地区关键词到 ISO 国家代码的映射 */
const REGION_KEYWORDS: Array<{ codes: string[]; region: string }> = [
  { codes: ['中国', 'cn', 'china', '大陆', '内地', '北京', '上海', '广州', '深圳'], region: 'CN' },
  { codes: ['香港', 'hk', 'hongkong', 'hong kong'], region: 'HK' },
  { codes: ['台湾', 'tw', 'taiwan'], region: 'TW' },
  { codes: ['日本', 'jp', 'japan', '东京', '大阪'], region: 'JP' },
  { codes: ['韩国', 'kr', 'korea', '首尔'], region: 'KR' },
  { codes: ['美国', 'us', 'usa', 'united states', 'america', '洛杉矶', '硅谷', '西雅图', '纽约', '达拉斯'], region: 'US' },
  { codes: ['新加坡', 'sg', 'singapore'], region: 'SG' },
  { codes: ['英国', 'uk', 'gb', 'england', 'london', '伦敦'], region: 'GB' },
  { codes: ['德国', 'de', 'germany', '法兰克福'], region: 'DE' },
  { codes: ['法国', 'fr', 'france', '巴黎'], region: 'FR' },
  { codes: ['加拿大', 'ca', 'canada', '多伦多'], region: 'CA' },
  { codes: ['澳大利亚', 'au', 'australia', '悉尼'], region: 'AU' },
  { codes: ['俄罗斯', 'ru', 'russia'], region: 'RU' },
  { codes: ['印度', 'india', '孟买', 'mumbai', '新德里', 'delhi'], region: 'IN' },
  { codes: ['荷兰', 'nl', 'netherlands', 'amsterdam', '阿姆斯特丹'], region: 'NL' },
  { codes: ['土耳其', 'tr', 'turkey', '伊斯坦布尔'], region: 'TR' },
  { codes: ['巴西', 'br', 'brazil'], region: 'BR' },
  { codes: ['泰国', 'th', 'thailand', '曼谷'], region: 'TH' },
  { codes: ['越南', 'vn', 'vietnam'], region: 'VN' },
  { codes: ['菲律宾', 'ph', 'philippines'], region: 'PH' },
  { codes: ['马来西亚', 'my', 'malaysia'], region: 'MY' },
  { codes: ['印尼', 'id', 'indonesia', '印度尼西亚'], region: 'ID' },
]

/** 从服务器信息提取地区代码（如 "CN", "US"） */
export function getRegionFromServer(
  server: Pick<ServerData, 'display_name' | 'hostname'>,
): string {
  const text = `${server.display_name || ''} ${server.hostname || ''}`.toLowerCase()
  for (const { codes, region } of REGION_KEYWORDS) {
    for (const code of codes) {
      const c = code.toLowerCase()
      if (/^[a-z]{2,3}$/.test(c)) {
        // 短英文代码用单词边界匹配，避免子串误判
        if (new RegExp(`(^|[^a-z])${c}([^a-z]|$)`).test(text)) {
          return region
        }
      } else {
        if (text.includes(c)) return region
      }
    }
  }
  return ''
}

/** 地区代码转国旗 emoji（如 "CN" -> 🇨🇳） */
export function getFlagEmoji(region: string): string {
  if (!region || region.length !== 2) return ''
  const upper = region.toUpperCase()
  if (!/^[A-Z]{2}$/.test(upper)) return ''
  const codePoints = upper.split('').map((c) => 0x1f1e6 + c.charCodeAt(0) - 65)
  return String.fromCodePoint(...codePoints)
}

/** 获取服务器国家代码：管理员设置的 country_code 优先，否则从名称推断 */
export function getCountryCode(
  server: Pick<ServerData, 'country_code' | 'display_name' | 'hostname'>,
): string {
  const cc = (server.country_code || '').trim().toUpperCase()
  if (/^[A-Z]{2}$/.test(cc)) return cc
  return getRegionFromServer(server)
}

/** 常用币种符号映射 */
const CURRENCY_SYMBOLS: Record<string, string> = {
  CNY: '¥',
  USD: '$',
  EUR: '€',
  JPY: 'JP¥',
  GBP: '£',
  HKD: 'HK$',
  TWD: 'NT$',
  KRW: '₩',
  SGD: 'S$',
}

/** 周期显示文案 */
const CYCLE_LABELS: Record<string, string> = {
  monthly: '/月',
  yearly: '/年',
  weekly: '/周',
  quarterly: '/季',
}

/** 格式化费用（如 "$49.99/年"、"¥0/月"），amount<=0 且有周期时返回 "免费" */
export function formatPrice(amount: number, currency: string, cycle: string): string {
  if (!amount || amount <= 0) return '免费'
  const symbol = CURRENCY_SYMBOLS[(currency || '').toUpperCase()] || (currency || '') + ' '
  const cycleLabel = CYCLE_LABELS[(cycle || '').toLowerCase()] || ''
  const num = amount >= 100 ? amount.toFixed(0) : amount.toFixed(2)
  return `${symbol}${num}${cycleLabel}`
}

/** 月流量使用率（0-100+ 百分比）；未设置配额（quota<=0）返回 null 表示不限量 */
export function getMonthlyTrafficPercent(usedBytes: number, quotaBytes: number): number | null {
  if (!quotaBytes || quotaBytes <= 0) return null
  return (usedBytes / quotaBytes) * 100
}

/** 月流量使用率对应颜色（NodeGet 色阶：<80 绿、80-100 橙、>=100 红） */
export function getTrafficColor(percent: number): string {
  if (percent >= 100) return '#f56565'
  if (percent >= 80) return '#f6ad55'
  return '#42b983'
}

/** 到期剩余天数对应的显示颜色：<7 天红、<30 天黄、其他灰 */
export function getExpireColor(days: number): string {
  if (days < 0) return '#f56565'
  if (days < 7) return '#f56565'
  if (days < 30) return '#f6ad55'
  return ''
}

/** 标签徽章色板（NodeGet 柔和渐变色调，按标签名 hash 取色保证稳定） */
const TAG_PALETTE = [
  'rgba(59, 130, 246, 0.15)|#60a5fa',   // 蓝
  'rgba(34, 197, 94, 0.15)|#4ade80',    // 绿
  'rgba(168, 85, 247, 0.15)|#c084fc',   // 紫
  'rgba(236, 72, 153, 0.15)|#f472b6',   // 粉
  'rgba(249, 115, 22, 0.15)|#fb923c',   // 橙
  'rgba(20, 184, 166, 0.15)|#2dd4bf',   // 青
  'rgba(234, 179, 8, 0.15)|#facc15',    // 黄
  'rgba(148, 163, 184, 0.15)|#94a3b8',  // 石板
] as const

/** 解析标签字符串为标签数组（逗号/顿号分隔，去空白） */
export function parseTags(tags?: string | null): string[] {
  if (!tags) return []
  return tags
    .split(/[,，、]/)
    .map((t) => t.trim())
    .filter(Boolean)
}

/** 标签名 hash（FNV-1a，简单稳定） */
function hashString(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** 获取标签徽章样式（背景色 + 文字色，同名标签颜色稳定） */
export function getTagStyle(tag: string): { background: string; color: string } {
  const [bg, fg] = TAG_PALETTE[hashString(tag) % TAG_PALETTE.length].split('|')
  return { background: bg, color: fg }
}

/** 格式化到期日期（RFC3339 -> "2027-01-15"） */
export function formatExpireDate(expiresAt?: string | null): string {
  if (!expiresAt) return ''
  const d = new Date(expiresAt)
  if (isNaN(d.getTime())) return ''
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}
