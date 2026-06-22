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
  deleteAgent as deleteAgentAPI,
  getToken,
  clearToken,
  ApiError,
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

  // WebSocket 监听器清理函数（内部使用）
  _wsCleanups: (() => void)[] | null
  _publicWsCleanups: (() => void)[] | null

  // Actions
  initAuth: () => Promise<void>
  checkSetupStatus: () => Promise<void>
  login: (username: string, password: string) => Promise<void>
  setup: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  fetchServers: () => Promise<void>
  fetchServerDetail: (id: number) => Promise<void>
  deleteAgent: (id: number) => Promise<void>
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

/** 确保系统主题变化监听器只注册一次 */
let mediaQueryListenerRegistered = false

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
  _wsCleanups: null,
  _publicWsCleanups: null,

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
      // 使用状态码判断认证错误，避免依赖错误消息字符串
      if (err instanceof ApiError && err.status === 401) {
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

  // 删除 Agent，并刷新服务器列表（从仪表盘移除已删除的 Agent）
  deleteAgent: async (id: number) => {
    await deleteAgentAPI(id)
    // 刷新服务器列表，从仪表盘移除已删除的 Agent
    await get().fetchServers()
  },

  // 连接 WebSocket
  connectWebSocket: () => {
    const ws = getDashboardWebSocket()
    // 先清理旧监听器，防止累积泄漏
    get()._wsCleanups?.forEach((fn) => fn())
    const cleanups = [
      ws.onStatusChange((connected) => set({ wsConnected: connected })),
      ws.onMessage((message) => {
        if (message.servers && message.servers.length > 0) {
          get().handleDashboardMessage(message.servers)
        }
      }),
    ]
    set({ _wsCleanups: cleanups })
    ws.connect()
  },

  // 断开 WebSocket
  disconnectWebSocket: () => {
    get()._wsCleanups?.forEach((fn) => fn())
    set({ _wsCleanups: null, wsConnected: false })
    getDashboardWebSocket().disconnect()
  },

  // 连接公开仪表盘 WebSocket（无需登录）
  connectPublicDashboardWS: () => {
    const ws = getPublicDashboardWebSocket()
    // 先清理旧监听器，防止累积泄漏
    get()._publicWsCleanups?.forEach((fn) => fn())
    const cleanups = [
      ws.onStatusChange((connected) => set({ publicWsConnected: connected })),
      ws.onMessage((message) => {
        if (message.servers && message.servers.length > 0) {
          get().handleDashboardMessage(message.servers)
        }
      }),
    ]
    set({ _publicWsCleanups: cleanups })
    ws.connect()
  },

  // 断开公开仪表盘 WebSocket
  disconnectPublicDashboardWS: () => {
    get()._publicWsCleanups?.forEach((fn) => fn())
    set({ _publicWsCleanups: null, publicWsConnected: false })
    getPublicDashboardWebSocket().disconnect()
  },

  // 处理仪表盘实时数据
  handleDashboardMessage: (data: DashboardItem[]) => {
    const state = get()
    const newMap = new Map(state.dashboardData)
    const now = Date.now()
    const existingIds = new Set(state.servers.map((s) => s.id))
    let newRealtimeHistory = state.realtimeHistory
    const newServersToAdd: ServerData[] = []

    for (const item of data) {
      newMap.set(item.agent_id, item)

      // 如果当前正在查看该服务器的详情页，追加实时历史数据
      if (state.currentServer && state.currentServer.id === item.agent_id) {
        const point: RealtimePoint = {
          timestamp: item.timestamp || now,
          cpu: item.cpu,
          mem: item.mem,
          net_rx: item.net_rx,
          net_tx: item.net_tx,
          ping_data: item.ping_data,
        }
        newRealtimeHistory = [...newRealtimeHistory, point]
        if (newRealtimeHistory.length > MAX_REALTIME_POINTS) {
          newRealtimeHistory = newRealtimeHistory.slice(
            newRealtimeHistory.length - MAX_REALTIME_POINTS,
          )
        }
      }

      // 新服务器，添加到列表
      if (!existingIds.has(item.agent_id)) {
        newServersToAdd.push({
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
          swap_total: item.swap_total || 0,
          swap_used: item.swap_used || 0,
          net_rx: item.net_rx,
          net_tx: item.net_tx,
          uptime: item.uptime,
          load_1: item.load_1,
          load_5: item.load_5 || 0,
          load_15: item.load_15 || 0,
          disk_usage: item.disk_usage || 0,
          disks: item.disks || [],
          tcp_connections: item.tcp_connections || 0,
          udp_connections: item.udp_connections || 0,
          process_count: item.process_count || 0,
          ping_data: item.ping_data || [],
        })
      }
    }

    // 一次性更新所有状态：合并新服务器并更新已有服务器的实时数据
    const allServers = [...state.servers, ...newServersToAdd]
    const updatedServers = allServers.map((server) => {
      const live = newMap.get(server.id)
      if (live) {
        return {
          ...server,
          online: live.online,
          cpu: live.cpu,
          mem: live.mem,
          mem_total: live.mem_total,
          mem_used: live.mem_used,
          swap_total: live.swap_total || 0,
          swap_used: live.swap_used || 0,
          net_rx: live.net_rx,
          net_tx: live.net_tx,
          uptime: live.uptime,
          load_1: live.load_1,
          load_5: live.load_5 || 0,
          load_15: live.load_15 || 0,
          disk_usage: live.disk_usage,
          disks: live.disks || [],
          tcp_connections: live.tcp_connections || 0,
          udp_connections: live.udp_connections || 0,
          process_count: live.process_count || 0,
          ping_data: live.ping_data,
          last_seen: live.timestamp,
          hostname: live.hostname || server.hostname,
          display_name: live.display_name || server.display_name,
        }
      }
      return server
    })

    set({
      dashboardData: newMap,
      servers: updatedServers,
      realtimeHistory: newRealtimeHistory,
    })
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

    // 监听系统主题变化（使用模块级变量确保只注册一次）
    if (!mediaQueryListenerRegistered) {
      const mql = window.matchMedia('(prefers-color-scheme: dark)')
      mql.addEventListener('change', () => {
        if (get().theme === 'system') {
          applyTheme('system')
        }
      })
      mediaQueryListenerRegistered = true
    }
  },

  // 清除实时历史数据
  clearRealtimeHistory: () => {
    set({ realtimeHistory: [] })
  },
}))
