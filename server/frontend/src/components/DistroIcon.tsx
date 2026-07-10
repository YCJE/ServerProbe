import { memo, useMemo } from 'react'

/** 支持的发行版类型 */
export type DistroType =
  | 'ubuntu'
  | 'debian'
  | 'centos'
  | 'arch'
  | 'alpine'
  | 'fedora'
  | 'rocky'
  | 'manjaro'
  | 'windows'
  | 'linux'

interface DistroIconProps {
  /** 发行版名称（来自 server.distro 或 server.os） */
  distro?: string
  /** OS 字段（用于回退匹配） */
  os?: string
  /** 图标尺寸（px），默认 16 */
  size?: number
  /** 是否显示发行版名称文字 */
  showLabel?: boolean
  /** 自定义类名 */
  className?: string
}

/** 根据发行版字符串识别类型 */
function detectDistro(distro: string | undefined, os: string | undefined): DistroType {
  const text = `${distro || ''} ${os || ''}`.toLowerCase()

  // Windows 优先匹配
  if (text.includes('windows') || text.includes('win32') || text.includes('win64') || text.includes('winnt')) return 'windows'

  // Linux 发行版匹配
  if (text.includes('ubuntu')) return 'ubuntu'
  if (text.includes('debian')) return 'debian'
  if (text.includes('centos')) return 'centos'
  if (text.includes('rocky')) return 'rocky'
  if (text.includes('fedora')) return 'fedora'
  if (text.includes('alpine')) return 'alpine'
  if (text.includes('manjaro')) return 'manjaro'
  if (text.includes('arch')) return 'arch'

  // 默认通用 Linux
  return 'linux'
}

/** 发行版显示名称映射 */
const DISTRO_LABELS: Record<DistroType, string> = {
  ubuntu: 'Ubuntu',
  debian: 'Debian',
  centos: 'CentOS',
  arch: 'Arch',
  alpine: 'Alpine',
  fedora: 'Fedora',
  rocky: 'Rocky',
  manjaro: 'Manjaro',
  windows: 'Windows',
  linux: 'Linux',
}

/** 发行版主色（用于单色图标） */
const DISTRO_COLORS: Record<DistroType, string> = {
  ubuntu: '#E95420',
  debian: '#A81D33',
  centos: '#932279',
  arch: '#1793D1',
  alpine: '#0D597F',
  fedora: '#294172',
  rocky: '#10B981',
  manjaro: '#35BF5C',
  windows: '#0078D4',
  linux: '#333333',
}

/**
 * Ubuntu 图标
 */
function UbuntuIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="16" cy="16" r="14" fill={color} />
      <circle cx="16" cy="6" r="2.5" fill="#fff" />
      <circle cx="7" cy="20" r="2.5" fill="#fff" />
      <circle cx="25" cy="20" r="2.5" fill="#fff" />
      <path
        d="M16 9.5a6.5 6.5 0 0 1 5.62 3.25l-2.85 1.65A3.25 3.25 0 0 0 16 12.75V9.5z"
        fill="#fff"
      />
      <path
        d="M21.5 16a6.5 6.5 0 0 1-8.87 6.04l1.65-2.85a3.25 3.25 0 0 0 4.42-4.42l2.85-1.65c.62 1.05.95 2.2.95 3.38z"
        fill="#fff"
      />
      <path
        d="M12.63 22.04A6.5 6.5 0 0 1 10.5 16h3.25c0 1.18.66 2.27 1.65 2.85l-1.77 3.19z"
        fill="#fff"
      />
    </svg>
  )
}

/** Debian 图标 */
function DebianIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path
        d="M16 2C8.27 2 2 8.27 2 16s6.27 14 14 14 14-6.27 14-14S23.73 2 16 2z"
        fill={color}
      />
      <path
        d="M18.5 9.5c-.4.05-.3.2-.05.3.3.4-.1.6-.4.5-.5-.2-1.2.2-1.4.6-.2.4 0 .7.3.5.4-.3.9-.4 1.3-.2.5.3.6.8.2 1.2-.4.4-1.1.5-1.6.2-.6-.4-1.4-.3-1.9.3-.4.5-.5 1.2-.2 1.7.3.4.7.4 1 .1.3-.3.3-.8.7-1 .4-.2.9 0 1 .4.1.3-.1.6-.4.8-.6.4-1 .3-1.5 0-.7-.4-1.6-.2-2 .5-.3.5-.2 1.1.2 1.4.5.3 1.1.1 1.4-.4.2-.4.7-.5 1-.2.3.3.2.8-.2 1-.7.4-1.5.2-2-.4-.5-.6-1.4-.7-2-.2-.5.4-.6 1.2-.2 1.7.4.5 1.1.5 1.6.1.4-.3.9-.2 1.1.2.2.4-.1.8-.5 1-.9.4-1.9.1-2.4-.7-.4-.7-1.3-.9-2-.5-.6.4-.8 1.2-.4 1.8.3.5.9.7 1.4.4"
        stroke="#fff"
        strokeWidth="0.8"
        fill="none"
        strokeLinecap="round"
      />
    </svg>
  )
}

/** CentOS 图标 */
function CentosIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M16 2L20 6H12L16 2Z" fill={color} />
      <path d="M16 30L12 26H20L16 30Z" fill={color} />
      <path d="M2 16L6 12V20L2 16Z" fill={color} />
      <path d="M30 16L26 20V12L30 16Z" fill={color} />
      <path d="M16 2L12 6V12L6 12L2 16L6 20H12V26L16 30L20 26V20H26L30 16L26 12H20V6L16 2Z" fill={color} opacity="0.3" />
      <path d="M16 8L20 12H12L16 8Z" fill={color} />
      <path d="M16 24L12 20H20L16 24Z" fill={color} />
      <path d="M8 16L12 12V20L8 16Z" fill={color} />
      <path d="M24 16L20 20V12L24 16Z" fill={color} />
      <rect x="12" y="12" width="8" height="8" fill={color} />
    </svg>
  )
}

/** Arch Linux 图标 */
function ArchIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path
        d="M16 2C12 10 9 16 9 22a7 7 0 0 0 14 0c0-6-3-12-7-20z"
        fill={color}
      />
      <circle cx="16" cy="20" r="2" fill="#fff" />
    </svg>
  )
}

/** Alpine Linux 图标 */
function AlpineIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path
        d="M16 2L2 16L16 30L30 16L16 2Z"
        fill={color}
        opacity="0.2"
      />
      <path
        d="M16 2L2 16L16 30L30 16L16 2Z"
        stroke={color}
        strokeWidth="1.5"
        fill="none"
      />
      <path
        d="M9 19L13 13L17 17L21 11L23 19L9 19Z"
        fill={color}
      />
      <path d="M9 19L23 19" stroke={color} strokeWidth="1.5" />
    </svg>
  )
}

/** Fedora 图标 */
function FedoraIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <circle cx="16" cy="16" r="14" fill={color} />
      <path
        d="M19 8.5c-2.5 0-4.5 2-4.5 4.5v3.2c-.7-.3-1.5-.4-2.3-.4-2.5 0-4.5 1.8-4.5 4.2 0 2.4 2 4.2 4.5 4.2 2.4 0 4.4-1.7 4.5-4V13c0-1.3 1-2.3 2.3-2.3 1.2 0 2.2 1 2.2 2.3 0 1.2-1 2.2-2.2 2.2-.2 0-.4 0-.6-.1v2.3c.2 0 .4.05.6.05 2.5 0 4.5-2 4.5-4.5S21.5 8.5 19 8.5z"
        fill="#fff"
      />
    </svg>
  )
}

/** Rocky Linux 图标 */
function RockyIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path
        d="M16 2L2 16L16 30L30 16L16 2Z"
        fill={color}
        opacity="0.15"
      />
      <path
        d="M16 4L5 15L11 15L16 10L21 15L27 15L16 4Z"
        fill={color}
      />
      <path
        d="M5 17L11 17L16 22L21 17L27 17L16 28L5 17Z"
        fill={color}
        opacity="0.6"
      />
    </svg>
  )
}

/** Manjaro 图标 */
function ManjaroIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M2 6h6v20H2V6z" fill={color} />
      <path d="M12 6h6v8h-6V6z" fill={color} />
      <path d="M12 16h6v10h-6V16z" fill={color} />
      <path d="M20 6h10v6H20V6z" fill={color} />
      <path d="M20 14h10v12H20V14z" fill={color} opacity="0.7" />
    </svg>
  )
}

/** Windows 图标 */
function WindowsIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M3 5L14 3.5V14H3V5Z" fill={color} />
      <path d="M15 3.3L29 1.5V14H15V3.3Z" fill={color} />
      <path d="M3 16H14V26.5L3 25V16Z" fill={color} />
      <path d="M15 16H29V28.5L15 26.7V16Z" fill={color} />
    </svg>
  )
}

/** 通用 Linux 图标（Tux 企鹅简化版） */
function LinuxIcon({ color }: { color: string }) {
  return (
    <svg viewBox="0 0 32 32" fill="none" xmlns="http://www.w3.org/2000/svg">
      {/* 身体 */}
      <ellipse cx="16" cy="19" rx="7" ry="8" fill={color} />
      {/* 头部 */}
      <ellipse cx="16" cy="10" rx="5" ry="5.5" fill={color} />
      {/* 白色腹部 */}
      <ellipse cx="16" cy="21" rx="4.5" ry="6" fill="#fff" />
      {/* 喙 */}
      <path d="M16 12L13 14H19L16 12Z" fill="#FF9500" />
      {/* 眼睛 */}
      <circle cx="13.5" cy="9" r="1.2" fill="#fff" />
      <circle cx="18.5" cy="9" r="1.2" fill="#fff" />
      <circle cx="13.5" cy="9" r="0.6" fill="#000" />
      <circle cx="18.5" cy="9" r="0.6" fill="#000" />
      {/* 脚 */}
      <ellipse cx="12" cy="27" rx="2" ry="1.2" fill="#FF9500" />
      <ellipse cx="20" cy="27" rx="2" ry="1.2" fill="#FF9500" />
    </svg>
  )
}

/** 根据类型渲染对应图标 */
function renderIcon(type: DistroType, color: string) {
  switch (type) {
    case 'ubuntu':
      return <UbuntuIcon color={color} />
    case 'debian':
      return <DebianIcon color={color} />
    case 'centos':
      return <CentosIcon color={color} />
    case 'arch':
      return <ArchIcon color={color} />
    case 'alpine':
      return <AlpineIcon color={color} />
    case 'fedora':
      return <FedoraIcon color={color} />
    case 'rocky':
      return <RockyIcon color={color} />
    case 'manjaro':
      return <ManjaroIcon color={color} />
    case 'windows':
      return <WindowsIcon color={color} />
    case 'linux':
    default:
      return <LinuxIcon color={color} />
  }
}

/**
 * Linux 发行版图标组件
 *
 * - 根据发行版名称匹配对应的 SVG 图标
 * - 支持的发行版：Ubuntu、Debian、CentOS、Arch、Alpine、Fedora、Rocky、Manjaro、Linux(通用)、Windows
 * - 使用内联 SVG（不依赖外部图片）
 * - 无匹配时显示通用 Linux 图标
 */
function DistroIcon({
  distro,
  os,
  size = 16,
  showLabel = false,
  className = '',
}: DistroIconProps) {
  const type = useMemo(() => detectDistro(distro, os), [distro, os])
  const color = DISTRO_COLORS[type]
  const label = DISTRO_LABELS[type]

  return (
    <span
      className={`inline-flex items-center gap-1.5 ${className}`}
      title={label}
    >
      <span
        className="inline-flex shrink-0 items-center justify-center"
        style={{ width: size, height: size }}
      >
        {renderIcon(type, color)}
      </span>
      {showLabel && (
        <span className="text-xs text-muted-foreground">{label}</span>
      )}
    </span>
  )
}

export default memo(DistroIcon)
