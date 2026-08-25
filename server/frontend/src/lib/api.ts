import type {
  LoginRequest,
  LoginResponse,
  SetupRequest,
  SetupStatus,
  ServerData,
  ServerListResponse,
  HistoryData,
  DashboardItem,
  TimeRange,
  RegisterCode,
  AgentInfo,
  SystemStatus,
  AlertRule,
  NotifyChannel,
  ServiceMonitor,
  ServiceStatusResult,
  SSLCertMonitor,
  SSLCertStatusResult,
  TrafficResponse,
  MonthlyTraffic,
  AllTrafficResponse,
  SharePage,
} from '@/types'

/** API 基础路径 */
const API_BASE = '/api/v1'

/** 自定义 API 错误类，携带 HTTP 状态码 */
export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

/** 防止 401 时多次触发重定向 */
let isRedirecting = false

/**
 * 封装 fetch 请求，自动携带 Cookie（HttpOnly Cookie 包含 JWT Token）
 * credentials: 'include' 确保浏览器自动发送同源 Cookie
 */
async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...((options.headers as Record<string, string>) || {}),
  }

  // 添加超时控制 (15 秒)
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), 15000)

  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
      credentials: 'include',
      signal: controller.signal,
    })
  } catch (err) {
    clearTimeout(timeoutId)
    if (err instanceof DOMException && err.name === 'AbortError') {
      throw new Error('请求超时，请检查网络连接')
    }
    throw new Error('网络请求失败')
  }
  clearTimeout(timeoutId)

  if (response.status === 401) {
    // 非公开接口的 401 表示 Token 过期/无效，重定向到登录页
    // Cookie 由后端在 401 响应中清除，前端只需重定向
    const isPublicPath = path.startsWith('/public/') || path.includes('/auth/setup-status') || path.includes('/auth/login') || path.includes('/auth/me')
    if (!isPublicPath) {
      if (!isRedirecting) {
        isRedirecting = true
        window.location.replace('/login')
      }
    }
    throw new ApiError(401, '未授权，请重新登录')
  }

  if (!response.ok) {
    let message = `请求失败 (${response.status})`
    try {
      const error = await response.json()
      message = error.message || error.error || message
    } catch {
      // 忽略 JSON 解析错误
    }
    throw new ApiError(response.status, message)
  }

  // 处理空响应
  const text = await response.text()
  if (!text) return {} as T
  // 先尝试 JSON.parse，失败再检查 Content-Type 给出友好错误
  try {
    return JSON.parse(text) as T
  } catch {
    const contentType = response.headers.get('Content-Type') || ''
    if (!contentType.toLowerCase().includes('application/json')) {
      throw new ApiError(response.status, `服务器返回了非 JSON 响应 (Content-Type: ${contentType || '未知'})`)
    }
    throw new ApiError(response.status, '响应 JSON 解析失败')
  }
}

// ==================== 认证相关 API ====================

/** 检查是否需要初始化 */
export async function getSetupStatus(): Promise<SetupStatus> {
  return request<SetupStatus>('/auth/setup-status')
}

/** 检查当前登录状态（通过 HttpOnly Cookie 认证） */
export async function checkAuth(): Promise<{ authenticated: boolean }> {
  return request<{ authenticated: boolean }>('/auth/me')
}

/** 首次设置（创建管理员账户，后端自动设置 Cookie） */
export async function setup(data: SetupRequest): Promise<LoginResponse> {
  return request<LoginResponse>('/auth/setup', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

/** 登录（后端自动设置 HttpOnly Cookie） */
export async function login(data: LoginRequest): Promise<LoginResponse> {
  return request<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

/** 登出（后端清除 Cookie） */
export async function logout(): Promise<void> {
  await request('/auth/logout', { method: 'POST' })
}

// ==================== 服务器相关 API ====================

/** 获取服务器列表 */
export async function getServers(): Promise<ServerListResponse> {
  return request<ServerListResponse>('/servers')
}

/** 获取单台服务器详情 */
export async function getServerDetail(id: number): Promise<ServerData> {
  return request<ServerData>(`/servers/${id}`)
}

/** 获取服务器历史数据 */
export async function getServerHistory(
  id: number,
  range: TimeRange,
): Promise<HistoryData> {
  return request<HistoryData>(`/servers/${id}/history?range=${range}`)
}

/** 获取仪表盘数据（HTTP 轮询备用） */
export async function getDashboard(): Promise<{ servers: DashboardItem[] }> {
  return request<{ servers: DashboardItem[] }>('/dashboard')
}

// ==================== Agent 管理相关 API ====================

/** 生成注册码 */
export async function generateRegisterCode(displayName: string, remark: string): Promise<RegisterCode> {
  return request<RegisterCode>('/agents/register-codes', {
    method: 'POST',
    body: JSON.stringify({ display_name: displayName, remark }),
  })
}

/** 获取注册码列表 */
export async function getRegisterCodes(): Promise<{ codes: RegisterCode[] }> {
  return request<{ codes: RegisterCode[] }>('/agents/register-codes')
}

/** 删除注册码 */
export async function deleteRegisterCode(code: string): Promise<void> {
  await request(`/agents/register-codes/${encodeURIComponent(code)}`, { method: 'DELETE' })
}

/** 获取 Agent 列表 */
export async function getAgents(): Promise<{ agents: AgentInfo[] }> {
  return request<{ agents: AgentInfo[] }>('/agents')
}

/** 直接创建 Agent（Komari 风格：先创建记录生成 Token，再在被控机执行一键安装命令） */
export async function createAgent(
  displayName: string,
): Promise<{ agent_id: number; display_name: string; token: string }> {
  return request<{ agent_id: number; display_name: string; token: string }>('/agents', {
    method: 'POST',
    body: JSON.stringify({ display_name: displayName }),
  })
}

/** 获取 Agent Token（用于为已存在的 Agent 生成重装命令） */
export async function getAgentToken(
  id: number,
): Promise<{ agent_id: number; display_name: string; token: string }> {
  return request<{ agent_id: number; display_name: string; token: string }>(
    `/agents/${id}/token`,
  )
}

/** 删除 Agent */
export async function deleteAgent(id: number): Promise<void> {
  await request(`/agents/${id}`, { method: 'DELETE' })
}

/** 更新 Agent 信息（显示名称 + 标签） */
export async function updateAgent(
  id: number,
  data: { display_name: string; tags?: string },
): Promise<{ success: boolean }> {
  return request(`/agents/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

/** Agent 元数据（NodeGet 风格，管理员设置） */
export interface AgentMetaInput {
  region: string
  country_code: string
  isp: string
  /** 到期时间（YYYY-MM-DD，空字符串=永不过期） */
  expires_at: string
  price_amount: number
  price_currency: string
  price_cycle: string
  /** 月流量配额字节数（0=不限） */
  traffic_quota_bytes: number
}

/** 更新 Agent NodeGet 风格元数据 */
export async function updateAgentMeta(
  id: number,
  data: AgentMetaInput,
): Promise<{ success: boolean }> {
  return request(`/agents/${id}/meta`, { method: 'PUT', body: JSON.stringify(data) })
}

// ==================== 公开 API (无需登录) ====================

/** 公开服务器列表响应（过滤了敏感字段） */
export interface PublicServerItem {
  id: number
  display_name: string
  hostname: string
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
  uptime: number
  load_1: number
  load_5: number
  load_15: number
  disk_usage: number
}

/** 公开服务器列表响应 */
export interface PublicServerListResponse {
  servers: PublicServerItem[]
}

/** 获取公开服务器列表 (无需登录) */
export async function getPublicServers(): Promise<PublicServerListResponse> {
  return request<PublicServerListResponse>('/public/servers')
}

/** 获取公开仪表盘数据 (无需登录) */
export async function getPublicDashboard(): Promise<{ servers: DashboardItem[] }> {
  return request<{ servers: DashboardItem[] }>('/public/dashboard')
}

/** 获取公开服务器历史数据 (无需登录) */
export async function getPublicServerHistory(
  id: number,
  range: TimeRange,
): Promise<HistoryData> {
  return request<HistoryData>(`/public/servers/${id}/history?range=${range}`)
}

// ==================== Ping Targets API ====================

export interface PingTarget {
  id: number
  name: string
  target: string
  method: string
  enabled: boolean
  sort_order: number
  created_at: string
}

export async function getPingTargets(): Promise<{ targets: PingTarget[] }> {
  return request('/ping-targets')
}

export async function createPingTarget(data: { name: string; target: string; method?: string; enabled?: boolean; sort_order?: number }): Promise<{ target: PingTarget }> {
  return request('/ping-targets', { method: 'POST', body: JSON.stringify(data) })
}

export async function updatePingTarget(id: number, data: Partial<{ name: string; target: string; method: string; enabled: boolean; sort_order: number }>): Promise<{ target: PingTarget }> {
  return request(`/ping-targets/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

export async function deletePingTarget(id: number): Promise<{ success: boolean }> {
  return request(`/ping-targets/${id}`, { method: 'DELETE' })
}

/** 获取 Ping 探测间隔 */
export async function getPingInterval(): Promise<{ interval: number }> {
  return request('/ping-targets/interval')
}

/** 设置 Ping 探测间隔 */
export async function setPingInterval(interval: number): Promise<{ success: boolean }> {
  return request('/ping-targets/interval', { method: 'PUT', body: JSON.stringify({ interval }) })
}

// ==================== 系统状态 API ====================

/** 获取系统状态 */
export async function getSystemStatus(): Promise<SystemStatus> {
  return request('/system/status')
}

// ==================== 告警规则 API ====================

/** 获取告警规则列表 */
export async function getAlertRules(): Promise<{ rules: AlertRule[] }> {
  return request('/alerts')
}

/** 创建告警规则 */
export async function createAlertRule(data: Omit<AlertRule, 'id' | 'created_at'>): Promise<{ rule: AlertRule }> {
  return request('/alerts', { method: 'POST', body: JSON.stringify(data) })
}

/** 更新告警规则 */
export async function updateAlertRule(id: number, data: Partial<AlertRule>): Promise<{ rule: AlertRule }> {
  return request(`/alerts/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

/** 删除告警规则 */
export async function deleteAlertRule(id: number): Promise<{ success: boolean }> {
  return request(`/alerts/${id}`, { method: 'DELETE' })
}

/** 测试告警规则 */
export async function testAlertRule(id: number): Promise<{ success: boolean }> {
  return request(`/alerts/${id}/test`, { method: 'POST' })
}

// ==================== 通知渠道 API ====================

/** 获取通知渠道列表 */
export async function getNotifyChannels(): Promise<{ channels: NotifyChannel[] }> {
  return request('/notify/channels')
}

/** 创建通知渠道 */
export async function createNotifyChannel(data: { name: string; type: string; config: string }): Promise<{ channel: NotifyChannel }> {
  return request('/notify/channels', { method: 'POST', body: JSON.stringify(data) })
}

/** 更新通知渠道 */
export async function updateNotifyChannel(id: number, data: Partial<{ name: string; type: string; config: string }>): Promise<{ channel: NotifyChannel }> {
  return request(`/notify/channels/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

/** 删除通知渠道 */
export async function deleteNotifyChannel(id: number): Promise<{ success: boolean }> {
  return request(`/notify/channels/${id}`, { method: 'DELETE' })
}

/** 测试通知渠道 */
export async function testNotifyChannel(id: number): Promise<{ success: boolean }> {
  return request(`/notify/channels/${id}/test`, { method: 'POST' })
}

// ==================== 系统日志 API ====================

/** 日志条目 */
export interface LogEntry {
  timestamp: string
  level: 'INFO' | 'WARNING' | 'ERROR' | 'DEBUG'
  message: string
}

/** 日志列表响应 */
export interface LogListResponse {
  logs: LogEntry[]
  total: number
}

/** 获取系统日志 */
export async function getLogs(params?: {
  level?: string
  limit?: number
  search?: string
}): Promise<LogListResponse> {
  const query = new URLSearchParams()
  if (params?.level) query.set('level', params.level)
  if (params?.limit) query.set('limit', String(params.limit))
  if (params?.search) query.set('search', params.search)
  const qs = query.toString()
  return request(`/logs${qs ? `?${qs}` : ''}`)
}

// ==================== 服务监控 API (P0-3) ====================

/** 获取服务监控列表 */
export async function getServiceMonitors(): Promise<{ monitors: ServiceMonitor[] }> {
  return request('/service-monitors')
}

/** 创建服务监控 */
export async function createServiceMonitor(data: Partial<ServiceMonitor>): Promise<ServiceMonitor> {
  return request('/service-monitors', { method: 'POST', body: JSON.stringify(data) })
}

/** 更新服务监控 */
export async function updateServiceMonitor(id: number, data: Partial<ServiceMonitor>): Promise<ServiceMonitor> {
  return request(`/service-monitors/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

/** 删除服务监控 */
export async function deleteServiceMonitor(id: number): Promise<void> {
  await request(`/service-monitors/${id}`, { method: 'DELETE' })
}

/** 测试服务监控 */
export async function testServiceMonitor(id: number): Promise<{ status: string; latency: number; error?: string }> {
  return request(`/service-monitors/${id}/test`, { method: 'POST', body: JSON.stringify({}) })
}

/** 获取服务监控状态列表 */
export async function getServiceMonitorStatuses(): Promise<{ statuses: ServiceStatusResult[] }> {
  return request('/service-monitors/statuses')
}

// ==================== SSL 证书监控 API (P0-4) ====================

/** 获取 SSL 证书监控列表 */
export async function getSSLMonitors(): Promise<{ monitors: SSLCertMonitor[] }> {
  return request('/ssl-monitors')
}

/** 创建 SSL 证书监控 */
export async function createSSLMonitor(data: Partial<SSLCertMonitor>): Promise<SSLCertMonitor> {
  return request('/ssl-monitors', { method: 'POST', body: JSON.stringify(data) })
}

/** 更新 SSL 证书监控 */
export async function updateSSLMonitor(id: number, data: Partial<SSLCertMonitor>): Promise<SSLCertMonitor> {
  return request(`/ssl-monitors/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

/** 删除 SSL 证书监控 */
export async function deleteSSLMonitor(id: number): Promise<void> {
  await request(`/ssl-monitors/${id}`, { method: 'DELETE' })
}

/** 测试 SSL 证书监控 */
export async function testSSLMonitor(id: number): Promise<{ remaining_days: number; expiry_date: string; error?: string }> {
  return request(`/ssl-monitors/${id}/test`, { method: 'POST', body: JSON.stringify({}) })
}

/** 获取 SSL 证书监控状态列表 */
export async function getSSLMonitorStatuses(): Promise<{ statuses: SSLCertStatusResult[] }> {
  return request('/ssl-monitors/statuses')
}

// ==================== 流量统计 API (P0-1) ====================

/** 获取单台 Agent 流量数据 */
export async function getTraffic(
  agentId: number,
  range: 'today' | 'month' | 'custom',
  start?: string,
  end?: string,
): Promise<TrafficResponse | MonthlyTraffic> {
  const params = new URLSearchParams({ range })
  if (start) params.set('start', start)
  if (end) params.set('end', end)
  return request(`/traffic/${agentId}?${params}`)
}

/** 获取全部 Agent 当日流量汇总 */
export async function getAllTraffic(): Promise<AllTrafficResponse> {
  return request('/traffic')
}

// ==================== 分享页 API (P1-8) ====================

/** 获取分享页列表 */
export async function getSharePages(): Promise<{ pages: SharePage[] }> {
  return request('/share-pages')
}

/** 创建分享页 */
export async function createSharePage(data: Partial<SharePage>): Promise<SharePage> {
  return request('/share-pages', { method: 'POST', body: JSON.stringify(data) })
}

/** 更新分享页 */
export async function updateSharePage(id: number, data: Partial<SharePage>): Promise<SharePage> {
  return request(`/share-pages/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

/** 删除分享页 */
export async function deleteSharePage(id: number): Promise<void> {
  await request(`/share-pages/${id}`, { method: 'DELETE' })
}

/** 获取单个分享页详情 */
export async function getSharePage(id: number): Promise<SharePage> {
  return request(`/share-pages/${id}`)
}
