// ==================== 基础类型 ====================

/** 主题类型 */
export type Theme = 'light' | 'dark' | 'system'

/** 时间范围 */
export type TimeRange = '1h' | '6h' | '12h' | '1d' | '2d'

// ==================== 服务器相关类型 ====================

/** CPU 信息 */
export interface CpuInfo {
  usage: number
  cores: number
  model: string
  load_1: number
  load_5: number
  load_15: number
}

/** 内存信息 */
export interface MemoryInfo {
  total: number
  used: number
  swap_total: number
  swap_used: number
}

/** 磁盘分区信息 */
export interface DiskInfo {
  device: string
  total: number
  used: number
}

/** 网络信息 */
export interface NetworkInfo {
  rx_speed: number
  tx_speed: number
  tcp_connections: number
  udp_connections: number
}

/** Ping 探测结果 */
export interface PingResult {
  target: string
  name: string
  method: string
  avg_latency: number
  min_latency: number
  max_latency: number
  jitter: number
  loss: number
  packets_sent: number
  packets_recv: number
}

/** 服务器数据（完整） */
export interface ServerData {
  id: number
  hostname: string
  display_name: string
  os: string
  arch: string
  agent_version: string
  online: boolean
  last_seen: number
  cpu: number
  mem: number
  mem_total: number
  mem_used: number
  net_rx: number
  net_tx: number
  uptime: number
  load_1: number
  disk_usage: number
  ping_data: PingResult[]
}

/** 仪表盘实时数据项 */
export interface DashboardItem {
  agent_id: number
  hostname: string
  display_name: string
  online: boolean
  cpu: number
  mem: number
  mem_total: number
  mem_used: number
  net_rx: number
  net_tx: number
  load_1: number
  uptime: number
  disk_usage: number
  ping_data: PingResult[]
  timestamp: number
}

/** 仪表盘 WebSocket 消息 */
export interface DashboardMessage {
  type: 'dashboard_update' | 'dashboard_init'
  servers: DashboardItem[]
}

// ==================== 历史数据类型 ====================

/** 历史数据点 */
export interface HistoryPoint {
  timestamp: number
  cpu_usage: number
  mem_usage: number
  net_rx: number
  net_tx: number
  ping_data: PingResult[]
}

/** 历史数据响应 */
export interface HistoryData {
  agent_id: number
  range: string
  points: HistoryPoint[]
}

// ==================== 认证相关类型 ====================

/** 登录请求 */
export interface LoginRequest {
  username: string
  password: string
}

/** 登录响应 */
export interface LoginResponse {
  token: string
  expires_at: number
}

/** 首次设置请求 */
export interface SetupRequest {
  username: string
  password: string
}

/** 首次设置状态 */
export interface SetupStatus {
  needs_setup: boolean
}

// ==================== 服务器列表响应 ====================

/** 服务器列表响应 */
export interface ServerListResponse {
  servers: ServerData[]
  total: number
}

// ==================== API 通用响应 ====================

/** API 错误响应 */
export interface ApiError {
  error: string
  message?: string
}

// ==================== 注册码相关类型 ====================

/** 注册码 */
export interface RegisterCode {
  code: string
  display_name: string
  remark: string
  expires_at: string
  used: boolean
}

/** Agent 信息 */
export interface AgentInfo {
  id: number
  hostname: string
  display_name: string
  os: string
  arch: string
  agent_version: string
  online: boolean
  last_seen: string
  created_at: string
}
