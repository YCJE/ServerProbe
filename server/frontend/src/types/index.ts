// ==================== 基础类型 ====================

/** 主题类型 */
export type Theme = 'light' | 'dark' | 'system'

/** 时间范围 */
export type TimeRange = 'realtime' | '1h' | '6h' | '12h' | '1d' | '2d'

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
  swap_total: number
  swap_used: number
  net_rx: number
  net_tx: number
  uptime: number
  load_1: number
  load_5: number
  load_15: number
  disk_usage: number
  disks: Array<{ device: string; total: number; used: number }>
  tcp_connections: number
  udp_connections: number
  process_count: number
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
  swap_total: number
  swap_used: number
  net_rx: number
  net_tx: number
  load_1: number
  load_5: number
  load_15: number
  uptime: number
  disk_usage: number
  disks: Array<{ device: string; total: number; used: number }>
  tcp_connections: number
  udp_connections: number
  process_count: number
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
  mem_total: number
  mem_used: number
  swap_total: number
  swap_used: number
  disk_usage: string
  net_rx: number
  net_tx: number
  tcp_connections: number
  udp_connections: number
  load_1: number
  load_5: number
  load_15: number
  uptime: number
  process_count: number
  ping_data: PingResult[]
}

/** 历史数据响应 */
export interface HistoryData {
  source: 'ringbuffer' | 'sqlite'
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

// ==================== 系统状态相关类型 ====================

/** 系统状态 */
export interface SystemStatus {
  uptime: number
  mem_alloc: number
  mem_sys: number
  mem_num_gc: number
  db_size: number
  online_agents: number
  ws_connections: number
  goroutines: number
  disk_total: number
  disk_free: number
  version: string
}

// ==================== 告警规则相关类型 ====================

/** 告警规则 */
export interface AlertRule {
  id: number
  name: string
  metric: string
  operator: string
  threshold: number
  duration: number
  enabled: boolean
  notify_channel_id: number
  created_at: string
}

// ==================== 通知渠道相关类型 ====================

/** 通知渠道 */
export interface NotifyChannel {
  id: number
  name: string
  type: string
  config: Record<string, unknown>
  created_at: string
}
