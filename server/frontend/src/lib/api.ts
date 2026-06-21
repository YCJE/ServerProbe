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
} from '@/types'

/** API 基础路径 */
const API_BASE = '/api/v1'

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

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
    credentials: 'include',  // 自动发送 Cookie
  })

  if (response.status === 401) {
    clearToken()
    // 不在 setup-status 和公开 API 请求中跳转
    if (!path.includes('/auth/setup-status') && !path.startsWith('/public/')) {
      // 使用 setTimeout 避免阻塞当前请求的错误处理
      setTimeout(() => {
        window.location.href = '/login'
      }, 0)
    }
    throw new Error('未授权，请重新登录')
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
