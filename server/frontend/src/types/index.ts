// ==================== 基础类型 ====================

/** 主题类型 */
export type Theme = 'light' | 'dark' | 'system'

/** 时间范围 */
export type TimeRange = 'realtime' | '1h' | '6h' | '12h' | '1d' | '2d' | '3d'

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
  avg_latency?: number
  min_latency?: number
  max_latency?: number
  jitter?: number
  loss?: number
  packets_sent?: number
  packets_recv?: number
  /** IP 版本标注（4/6，0 或缺省 = 未标注，前端回退到名称启发式识别） */
  ip_version?: number
}

/** 进程信息（资源占用 Top N 进程） */
export interface ProcessInfo {
  pid: number
  name: string
  cpu: number
  memory: number
  rss: number
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
  cpu_model: string
  cpu_cores: number
  mem: number
  mem_total: number
  mem_used: number
  swap_total: number
  swap_used: number
  net_rx: number
  net_tx: number
  total_rx: number
  total_tx: number
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
  /** CPU 温度（摄氏度，0=不可用） */
  temperature?: number
  /** 虚拟化类型（如 KVM、OpenVZ、Docker、None 等） */
  virtualization?: string
  /** Linux 发行版名称（如 Ubuntu 22.04、Debian 12 等） */
  distro?: string
  /** 资源占用 Top N 进程列表 */
  processes?: ProcessInfo[]
  /** NTP 时间偏移（毫秒） */
  time_offset?: number
  /** 标签（逗号分隔的字符串） */
  tags?: string
  // ---- NodeGet 风格元数据（管理员设置，Agent 上报不覆盖） ----
  /** 位置（如 "上海"/"Tokyo"） */
  region?: string
  /** 国家代码（如 "CN"/"JP"，用于国旗与地图） */
  country_code?: string
  /** 供应商备注（如 "Bandwagon"/"Oracle"） */
  isp?: string
  /** 到期时间（null=永不过期，RFC3339 字符串） */
  expires_at?: string | null
  /** 剩余到期天数（null=永不过期） */
  expires_in_days?: number | null
  /** 周期费用数值 */
  price_amount?: number
  /** 币种：CNY/USD/EUR/JPY 等 */
  price_currency?: string
  /** 周期：monthly/yearly */
  price_cycle?: string
  /** 月流量配额字节数（0=不限） */
  traffic_quota_bytes?: number
  /** 当月累计下行字节 */
  monthly_rx?: number
  /** 当月累计上行字节 */
  monthly_tx?: number
  /** 出口 IPv4 */
  ipv4?: string
  /** 出口 IPv6 */
  ipv6?: string
}

/** 仪表盘实时数据项 */
export interface DashboardItem {
  agent_id: number
  hostname: string
  display_name: string
  os: string
  arch: string
  agent_version: string
  online: boolean
  cpu: number
  cpu_model: string
  cpu_cores: number
  mem: number
  mem_total: number
  mem_used: number
  swap_total: number
  swap_used: number
  net_rx: number
  net_tx: number
  total_rx: number
  total_tx: number
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
  /** CPU 温度（摄氏度，0=不可用） */
  temperature?: number
  /** 虚拟化类型（如 KVM、OpenVZ、Docker、None 等） */
  virtualization?: string
  /** Linux 发行版名称（如 Ubuntu 22.04、Debian 12 等） */
  distro?: string
  /** 资源占用 Top N 进程列表 */
  processes?: ProcessInfo[]
  /** NTP 时间偏移（毫秒） */
  time_offset?: number
  timestamp: number
  // ---- NodeGet 风格元数据（管理员设置，Agent 上报不覆盖） ----
  /** 标签（逗号分隔的字符串） */
  tags?: string
  /** 位置（如 "上海"/"Tokyo"） */
  region?: string
  /** 国家代码（如 "CN"/"JP"，用于国旗与地图） */
  country_code?: string
  /** 供应商备注（如 "Bandwagon"/"Oracle"） */
  isp?: string
  /** 到期时间（null=永不过期） */
  expires_at?: string | null
  /** 剩余到期天数（null=永不过期） */
  expires_in_days?: number | null
  /** 周期费用数值 */
  price_amount?: number
  /** 币种：CNY/USD/EUR/JPY 等 */
  price_currency?: string
  /** 周期：monthly/yearly */
  price_cycle?: string
  /** 月流量配额字节数（0=不限） */
  traffic_quota_bytes?: number
  /** 当月累计下行字节 */
  monthly_rx?: number
  /** 当月累计上行字节 */
  monthly_tx?: number
  /** 出口 IPv4 */
  ipv4?: string
  /** 出口 IPv6 */
  ipv6?: string
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
  cpu_model: string
  cpu_cores: number
  mem_usage: number
  mem_total: number
  mem_used: number
  swap_total: number
  swap_used: number
  disk_usage: number
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
  /** 在线状态（1=在线，0=离线），用于在线状态时间线渲染 */
  online?: number
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
  /** TOTP 动态码（启用两步验证后第二步登录使用） */
  totp_code?: string
}

/** 登录响应（Token 通过 HttpOnly Cookie 传递，不在响应体中返回） */
export interface LoginResponse {
  success: boolean
  message: string
  need_totp: boolean
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
  /** 标签（逗号分隔） */
  tags?: string
  // ---- NodeGet 风格元数据 ----
  region?: string
  country_code?: string
  isp?: string
  expires_at?: string | null
  price_amount?: number
  price_currency?: string
  price_cycle?: string
  traffic_quota_bytes?: number
  ipv4?: string
  ipv6?: string
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

/** 告警历史记录（FIRING 触发与 RESOLVED 恢复时间线） */
export interface AlertHistoryItem {
  id: number
  rule_id: number
  rule_name: string
  agent_id: number
  server_name: string
  metric: string
  state: 'firing' | 'resolved'
  value: number
  resolved_value: number
  message: string
  triggered_at: string
  resolved_at: string | null
}

/** 告警历史响应 */
export interface AlertHistoryResponse {
  histories: AlertHistoryItem[]
  total: number
  page: number
  page_size: number
}

// ==================== 标签相关类型 ====================

/** 标签（NodeGet 风格彩色标签） */
export interface Tag {
  id: number
  name: string
  /** 十六进制色值，如 "#3b82f6" */
  color: string
  created_at: string
}

// ==================== TOTP 两步验证相关类型 ====================

/** TOTP 绑定状态 */
export interface TOTPStatus {
  totp_enabled: boolean
}

/** TOTP Setup 响应（生成密钥与 otpauth URL） */
export interface TOTPSetupResponse {
  secret: string
  otpauth_url: string
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

// ==================== 服务监控 (P0-3) ====================

export interface ServiceMonitor {
  id: number
  name: string
  type: 'http' | 'tcp'
  target: string
  expected_status: number
  timeout: number
  interval: number
  enabled: boolean
  last_status: string
  last_latency: number
  last_checked: string
  created_at: string
}

export interface ServiceStatusResult {
  id: number
  name: string
  type: string
  target: string
  last_status: string
  last_latency: number
  last_checked: string
  enabled: boolean
}

// ==================== SSL 证书监控 (P0-4) ====================

export interface SSLCertMonitor {
  id: number
  domain: string
  port: number
  alert_days: number
  enabled: boolean
  last_expiry_date: string
  last_remaining_days: number
  last_checked: string
  created_at: string
}

export interface SSLCertStatusResult {
  id: number
  domain: string
  port: number
  alert_days: number
  last_expiry_date: string
  last_remaining_days: number
  last_checked: string
  enabled: boolean
}

// ==================== 流量统计 (P0-1) ====================

export interface TrafficRecord {
  id: number
  agent_id: number
  date: string
  rx_bytes: number
  tx_bytes: number
  updated_at: string
}

export interface TrafficResponse {
  agent_id: number
  date: string
  traffic: TrafficRecord | null
}

export interface MonthlyTraffic {
  agent_id: number
  year: number
  month: number
  records: TrafficRecord[]
  total: { rx_bytes: number; tx_bytes: number }
}

export interface AllTrafficResponse {
  date: string
  traffic: TrafficRecord[]
  total: { rx_bytes: number; tx_bytes: number }
}

// ==================== 分享页 (P1-8) ====================

export interface SharePage {
  id: number
  share_id: string
  title: string
  description: string
  agent_ids: string
  enabled: boolean
  sort_order: number
  created_at: string
}
