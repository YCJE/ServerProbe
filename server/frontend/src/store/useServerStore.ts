import { create } from 'zustand'
import type {
  ServerData,
  DashboardItem,
  Theme,
} from '@/types'
import {
  getServers,
  getServerDetail,
  getSetupStatus,
  login as apiLogin,
  setup as apiSetup,
  logout as apiLogout,
  getToken,
  clearToken,
} from '@/lib/api'
import { getDashboardWebSocket, getPublicDashboardWebSocket } from '@/lib/websocket'

/** 实时数据历史点（用于详情页实时图表） */
export interface RealtimePoint {
  timestamp: number
  cpu: number
  mem: number
  net_rx: number
  net_tx: number
  ping_data: DashboardItem['ping_data']
}

/** 服务器 Store 状态 */
interface ServerStoreState {
  // 认证状态
  isAuthenticated: boolean
  needsSetup: boolean
  authLoading: boolean

  // 服务器数据
  servers: ServerData[]
  dashboardData: Map<number, DashboardItem>
  serversLoading: boolean

  // WebSocket 连接状态
  wsConnected: boolean
  // 公开 WebSocket 连接状态
  publicWsConnected: boolean

  // 主题
  theme: Theme

  // 当前查看的服务器详情
  currentServer: ServerData | null
  realtimeHistory: RealtimePoint[]
  currentServerLoading: boolean

  // Actions
  initAuth: () => Promise<void>
  checkSetupStatus: () => Promise<void>
  login: (username: string, password: string) => Promise<void>
  setup: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  fetchServers: () => Promise<void>
  fetchServerDetail: (id: number) => Promise<void>
  connectWebSocket: () => void
  disconnectWebSocket: () => void
  connectPublicDashboardWS: () => void
  disconnectPublicDashboardWS: () => void
  handleDashboardMessage: (data: DashboardItem[]) => void
  setTheme: (theme: Theme) => void
  initTheme: () => void
  clearRealtimeHistory: () => void
}

/** 实时历史数据最大保留点数 */
const MAX_REALTIME_POINTS = 1200

/** 应用主题到 DOM */
function applyTheme(theme: Theme): void {
  const root = document.documentElement
  const systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches

  if (theme === 'dark' || (theme === 'system' && systemDark)) {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
}

/** 从 localStorage 加载主题 */
function loadTheme(): Theme {
  const stored = localStorage.getItem('probe_theme') as Theme | null
  return stored || 'system'
}

export const useServerStore = create<ServerStoreState>((set, get) => ({
  // 初始状态
  isAuthenticated: !!getToken(),
  needsSetup: false,
  authLoading: false,
  servers: [],
  dashboardData: new Map(),
  serversLoading: false,
  wsConnected: false,
  publicWsConnected: false,
  theme: loadTheme(),
  currentServer: null,
  realtimeHistory: [],
  currentServerLoading: false,

  // 初始化认证状态
  initAuth: async () => {
    const token = getToken()
    if (token) {
      set({ isAuthenticated: true })
    }
  },

  // 检查是否需要初始化
  checkSetupStatus: async () => {
    set({ authLoading: true })
    try {
      const status = await getSetupStatus()
      set({ needsSetup: status.needs_setup, authLoading: false })
    } catch (err) {
      // 请求失败时不设置 needsSetup=true，避免误跳 Setup 页
      // 保持 needsSetup=false，让用户看到登录页或公开页
      console.error('checkSetupStatus failed:', err)
      set({ needsSetup: false, authLoading: false })
    }
  },

  // 登录
  login: async (username: string, password: string) => {
    set({ authLoading: true })
    try {
      await apiLogin({ username, password })
      set({ isAuthenticated: true, authLoading: false })
    } catch (err) {
      set({ authLoading: false })
      throw err
    }
  },

  // 首次设置
  setup: async (username: string, password: string) => {
    set({ authLoading: true })
    try {
      await apiSetup({ username, password })
      set({ isAuthenticated: true, needsSetup: false, authLoading: false })
    } catch (err) {
      set({ authLoading: false })
      throw err
    }
  },

  // 登出
  logout: async () => {
    get().disconnectWebSocket()
    try {
      await apiLogout()
    } catch {
      // 忽略登出 API 错误
    }
    clearToken()
    set({
      isAuthenticated: false,
      servers: [],
      dashboardData: new Map(),
      currentServer: null,
      realtimeHistory: [],
    })
  },

  // 获取服务器列表
  fetchServers: async () => {
    set({ serversLoading: true })
    try {
      const response = await getServers()
      set({ servers: response.servers, serversLoading: false })
    } catch (err) {
      set({ serversLoading: false })
      // 如果是认证错误，更新认证状态
      if (err instanceof Error && err.message.includes('未授权')) {
        set({ isAuthenticated: false })
      }
      throw err
    }
  },

  // 获取服务器详情
  fetchServerDetail: async (id: number) => {
    set({ currentServerLoading: true })
    try {
      const server = await getServerDetail(id)
      set({ currentServer: server, currentServerLoading: false })
    } catch (err) {
      set({ currentServerLoading: false })
      throw err
    }
  },

  // 连接 WebSocket
  connectWebSocket: () => {
    const ws = getDashboardWebSocket()

    ws.onStatusChange((connected) => {
      set({ wsConnected: connected })
    })

    ws.onMessage((message) => {
      if (message.servers && message.servers.length > 0) {
        get().handleDashboardMessage(message.servers)
      }
    })

    ws.connect()
  },

  // 断开 WebSocket
  disconnectWebSocket: () => {
    const ws = getDashboardWebSocket()
    ws.disconnect()
    set({ wsConnected: false })
  },

  // 连接公开仪表盘 WebSocket（无需登录）
  connectPublicDashboardWS: () => {
    const ws = getPublicDashboardWebSocket()

    ws.onStatusChange((connected) => {
      set({ publicWsConnected: connected })
    })

    ws.onMessage((message) => {
      if (message.servers && message.servers.length > 0) {
        get().handleDashboardMessage(message.servers)
      }
    })

    ws.connect()
  },

  // 断开公开仪表盘 WebSocket
  disconnectPublicDashboardWS: () => {
    const ws = getPublicDashboardWebSocket()
    ws.disconnect()
    set({ publicWsConnected: false })
  },

  // 处理仪表盘实时数据
  handleDashboardMessage: (data: DashboardItem[]) => {
    const newMap = new Map(get().dashboardData)
    const now = Date.now()
    const existingIds = new Set(get().servers.map((s) => s.id))
    const newServers: ServerData[] = []

    for (const item of data) {
      newMap.set(item.agent_id, item)

      // 如果当前正在查看该服务器的详情页，追加实时历史数据
      const currentServer = get().currentServer
      if (currentServer && currentServer.id === item.agent_id) {
        const point: RealtimePoint = {
          timestamp: item.timestamp || now,
          cpu: item.cpu,
          mem: item.mem,
          net_rx: item.net_rx,
          net_tx: item.net_tx,
          ping_data: item.ping_data,
        }

        const history = [...get().realtimeHistory, point]
        // 限制历史数据点数量
        if (history.length > MAX_REALTIME_POINTS) {
          history.splice(0, history.length - MAX_REALTIME_POINTS)
        }
        set({ realtimeHistory: history })
      }

      // 新服务器，添加到列表
      if (!existingIds.has(item.agent_id)) {
        newServers.push({
          id: item.agent_id,
          hostname: item.hostname || `Agent-${item.agent_id}`,
          display_name: item.display_name || '',
          os: '',
          arch: '',
          agent_version: '',
          online: item.online,
          last_seen: item.timestamp,
          cpu: item.cpu,
          mem: item.mem,
          mem_total: item.mem_total,
          mem_used: item.mem_used,
          net_rx: item.net_rx,
          net_tx: item.net_tx,
          uptime: item.uptime,
          load_1: item.load_1,
          disk_usage: item.disk_usage || 0,
          ping_data: item.ping_data || [],
        })
      }
    }

    // 合并新服务器
    if (newServers.length > 0) {
      set({ servers: [...get().servers, ...newServers] })
    }

    // 更新已有服务器的实时数据
    const servers = get().servers.map((server) => {
      const live = newMap.get(server.id)
      if (live) {
        return {
          ...server,
          online: live.online,
          cpu: live.cpu,
          mem: live.mem,
          mem_total: live.mem_total,
          mem_used: live.mem_used,
          net_rx: live.net_rx,
          net_tx: live.net_tx,
          uptime: live.uptime,
          load_1: live.load_1,
          disk_usage: live.disk_usage,
          ping_data: live.ping_data,
          last_seen: live.timestamp,
          hostname: live.hostname || server.hostname,
          display_name: live.display_name || server.display_name,
        }
      }
      return server
    })

    set({ servers, dashboardData: newMap })
  },

  // 设置主题
  setTheme: (theme: Theme) => {
    localStorage.setItem('probe_theme', theme)
    applyTheme(theme)
    set({ theme })
  },

  // 初始化主题
  initTheme: () => {
    const theme = loadTheme()
    applyTheme(theme)
    set({ theme })

    // 监听系统主题变化
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (get().theme === 'system') {
        applyTheme('system')
      }
    })
  },

  // 清除实时历史数据
  clearRealtimeHistory: () => {
    set({ realtimeHistory: [] })
  },
}))
