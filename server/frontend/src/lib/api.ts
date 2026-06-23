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

/** 获取存储的 JWT Token */
export function getToken(): string | null {
  return localStorage.getItem('probe_token')
}

/** 存储 JWT Token */
export function setToken(token: string): void {
  localStorage.setItem('probe_token', token)
}

/** 清除 JWT Token */
export function clearToken(): void {
  localStorage.removeItem('probe_token')
}

/** 封装 fetch 请求，自动携带 Token 和 Cookie */
async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...((options.headers as Record<string, string>) || {}),
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
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
    clearToken()
    // 不在 setup-status 和公开 API 请求中跳转
    if (!path.includes('/auth/setup-status') && !path.startsWith('/public/')) {
      // 使用防重定向标志，避免多次 401 触发多次跳转
      if (!isRedirecting) {
        isRedirecting = true
        setTimeout(() => {
          window.location.href = '/login'
          isRedirecting = false
        }, 0)
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
    throw new Error(message)
  }

  // 处理空响应
  const text = await response.text()
  if (!text) {
    return {} as T
  }
  return JSON.parse(text) as T
}

// ==================== 认证相关 API ====================

/** 检查是否需要初始化 */
export async function getSetupStatus(): Promise<SetupStatus> {
  return request<SetupStatus>('/auth/setup-status')
}

/** 首次设置（创建管理员账户） */
export async function setup(data: SetupRequest): Promise<LoginResponse> {
  const result = await request<LoginResponse>('/auth/setup', {
    method: 'POST',
    body: JSON.stringify(data),
  })
  if (result.token) {
    setToken(result.token)
  }
  return result
}

/** 登录 */
export async function login(data: LoginRequest): Promise<LoginResponse> {
  const result = await request<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(data),
  })
  if (result.token) {
    setToken(result.token)
  }
  return result
}

/** 登出 */
export async function logout(): Promise<void> {
  try {
    await request('/auth/logout', { method: 'POST' })
  } finally {
    clearToken()
  }
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
  await request(`/agents/register-codes/${code}`, { method: 'DELETE' })
}

/** 获取 Agent 列表 */
export async function getAgents(): Promise<{ agents: AgentInfo[] }> {
  return request<{ agents: AgentInfo[] }>('/agents')
}

/** 删除 Agent */
export async function deleteAgent(id: number): Promise<void> {
  await request(`/agents/${id}`, { method: 'DELETE' })
}

/** 更新 Agent 信息 */
export async function updateAgent(id: number, data: { display_name: string }): Promise<{ success: boolean }> {
  return request(`/agents/${id}`, { method: 'PUT', body: JSON.stringify(data) })
}

// ==================== 公开 API (无需登录) ====================

/** 公开服务器列表响应（过滤了敏感字段） */
export interface PublicServerItem {
  id: number
  display_name: string
  hostname: string
  os: string
  online: boolean
  cpu: number
  mem: number
  mem_total: number
  mem_used: number
  net_rx: number
  net_tx: number
  uptime: number
  load_1: number
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
