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
  checkAuth as apiCheckAuth,
  login as apiLogin,
  setup as apiSetup,
  logout as apiLogout,
  deleteAgent as deleteAgentAPI,
  ApiError,
} from '@/lib/api'
import { getDashboardWebSocket, getPublicDashboardWebSocket } from '@/lib/websocket'

/** checkAuth 网络错误重试计数器（模块级，成功时重置） */
let checkAuthRetryCount = 0

/** checkAuth 网络错误重试定时器（模块级，成功或重新调度时清理） */
let checkAuthRetryTimer: ReturnType<typeof setTimeout> | null = null

/** 实时数据历史点（用于详情页实时图表） */
export interface RealtimePoint {
  timestamp: number
  cpu: number
  mem: number
  net_rx: number
  net_tx: number
  ping_data: DashboardItem['ping_data']
  /** 在线状态（1=在线，0=离线），用于在线状态时间线渲染 */
  online: number
}

/** P2: 卡片历史数据点（用于卡片内延迟质量分布和丢包率横条） */
export interface CardHistoryPoint {
  timestamp: number
  ping_data: DashboardItem['ping_data']
  online: boolean
}

/** 服务器 Store 状态 */
interface ServerStoreState {
  // 认证状态
  isAuthenticated: boolean
  authInitialized: boolean  // 初始认证检查是否完成（防止首屏闪烁）
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

  // P2: 卡片历史滚动窗口（所有服务器共享，每台最多 60 点）
  cardHistory: Map<number, CardHistoryPoint[]>

  // WebSocket 监听器清理函数（内部使用）
  _wsCleanups: (() => void)[] | null
  _publicWsCleanups: (() => void)[] | null
  // 最近删除的 Agent ID（防止 WS 消息在 fetchServers 完成前重新引入已删除的 Agent）
  _recentlyDeletedIds: Set<number>

  // Actions
  checkSetupStatus: () => Promise<void>
  checkAuth: () => Promise<void>
  login: (username: string, password: string) => Promise<void>
  setup: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  fetchServers: () => Promise<void>
  fetchServerDetail: (id: number) => Promise<void>
  abortCurrentFetch: () => void
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

/** P2: 卡片历史滚动窗口最大点数（约 3 分钟 @ 3s 上报间隔） */
const MAX_CARD_HISTORY_POINTS = 60

/** fetchServerDetail 请求 ID，用于防止快速切换服务器时旧请求覆盖新数据 */
let fetchServerDetailRequestId = 0

/** 确保系统主题变化监听器只注册一次 */
let mediaQueryListenerRegistered = false

/** 浅比较两个 ping_data 数组是否内容相同 */
function pingDataEqual(a: DashboardItem['ping_data'], b: DashboardItem['ping_data']): boolean {
  if (a === b) return true
  if (!a || !b) return false
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    const pa = a[i], pb = b[i]
    if (pa.target !== pb.target || pa.name !== pb.name ||
        pa.avg_latency !== pb.avg_latency || pa.loss !== pb.loss ||
        pa.min_latency !== pb.min_latency || pa.max_latency !== pb.max_latency ||
        pa.jitter !== pb.jitter || pa.method !== pb.method ||
        pa.packets_sent !== pb.packets_sent || pa.packets_recv !== pb.packets_recv) {
      return false
    }
  }
  return true
}

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
  try {
    const stored = localStorage.getItem('probe_theme')
    // 校验返回值是否为合法主题，无效值回退到默认深色主题
    if (stored === 'light' || stored === 'dark' || stored === 'system') {
      return stored
    }
  } catch {
    // localStorage 不可用（隐私模式等），回退默认值
  }
  return 'dark' // 默认深色主题 (Apple HIG)
}

export const useServerStore = create<ServerStoreState>((set, get) => ({
  // 初始状态
  isAuthenticated: false,
  authInitialized: false,
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
  cardHistory: new Map<number, CardHistoryPoint[]>(),
  _wsCleanups: null,
  _publicWsCleanups: null,
  _recentlyDeletedIds: new Set<number>(),

  // 检查是否需要初始化
  checkSetupStatus: async () => {
    set({ authLoading: true })
    try {
      const status = await getSetupStatus()
      set({ needsSetup: status.needs_setup, authLoading: false })
    } catch (err) {
      console.error('checkSetupStatus failed:', err)
      set({ needsSetup: false, authLoading: false })
    }
  },

  // 检查登录状态（通过 HttpOnly Cookie 认证，页面加载时调用）
  checkAuth: async () => {
    try {
      const result = await apiCheckAuth()
      checkAuthRetryCount = 0
      if (checkAuthRetryTimer) { clearTimeout(checkAuthRetryTimer); checkAuthRetryTimer = null }
      set({ isAuthenticated: result.authenticated, authInitialized: true })
    } catch (err) {
      if (err instanceof ApiError) {
        // 服务端返回 HTTP 错误（非网络错误），标记为未认证
        checkAuthRetryCount = 0
        if (checkAuthRetryTimer) { clearTimeout(checkAuthRetryTimer); checkAuthRetryTimer = null }
        set({ isAuthenticated: false, authInitialized: true })
        return
      }
      // 真正的网络错误（超时/断网）：指数退避重试，最多 10 次
      checkAuthRetryCount++
      if (checkAuthRetryCount > 10) {
        set({ isAuthenticated: false, authInitialized: true })
        return
      }
      const delay = Math.min(3000 * Math.pow(1.5, checkAuthRetryCount - 1), 30000)
      console.error(`checkAuth network error (retry ${checkAuthRetryCount}):`, err)
      if (checkAuthRetryTimer) clearTimeout(checkAuthRetryTimer)
      checkAuthRetryTimer = setTimeout(() => { checkAuthRetryTimer = null; get().checkAuth() }, delay)
    }
  },

  // 登录
  login: async (username: string, password: string) => {
    set({ authLoading: true })
    try {
      const result = await apiLogin({ username, password })
      // 必须检查 success 字段：TOTP 需要二次验证时返回 200 + success=false
      if (!result.success) {
        set({ authLoading: false })
        // 返回 need_totp 信息供 Login 页面处理
        if (result.need_totp) {
          throw new Error('需要两步验证')
        }
        throw new Error(result.message || '登录失败')
      }
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
      const result = await apiSetup({ username, password })
      if (!result.success) {
        set({ authLoading: false })
        throw new Error(result.message || '设置失败')
      }
      set({ isAuthenticated: true, needsSetup: false, authLoading: false })
    } catch (err) {
      set({ authLoading: false })
      throw err
    }
  },

  // 登出（乐观更新：先清状态跳转，再异步通知后端清除 Cookie）
  logout: async () => {
    get().disconnectWebSocket()
    // 清除 checkAuth 重试定时器，防止登出后仍触发认证重试
    if (checkAuthRetryTimer) { clearTimeout(checkAuthRetryTimer); checkAuthRetryTimer = null }
    checkAuthRetryCount = 0
    // 乐观更新：立即清除前端状态，避免 UI 卡顿等待网络响应
    set({
      isAuthenticated: false,
      servers: [],
      dashboardData: new Map(),
      currentServer: null,
      realtimeHistory: [],
      currentServerLoading: false,
      serversLoading: false,
      authLoading: false,
      cardHistory: new Map<number, CardHistoryPoint[]>(),
      _recentlyDeletedIds: new Set<number>(),
    })
    try {
      await apiLogout()
    } catch {
      // 忽略登出 API 错误（前端状态已清，Cookie 由后端过期或下次 checkAuth 清除）
    }
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
    const requestId = ++fetchServerDetailRequestId
    set({ currentServer: null, currentServerLoading: true })
    try {
      const server = await getServerDetail(id)
      // 仅当请求 ID 匹配时才更新状态，防止快速切换时旧请求覆盖新数据
      if (fetchServerDetailRequestId === requestId) {
        set({ currentServer: server, currentServerLoading: false })
      }
    } catch (err) {
      if (fetchServerDetailRequestId === requestId) {
        set({ currentServerLoading: false })
      }
      throw err
    }
  },

  // 中止当前飞行中的 fetchServerDetail 请求（用于组件卸载时）
  abortCurrentFetch: () => {
    fetchServerDetailRequestId++
    set({ currentServer: null, currentServerLoading: false })
  },

  // 删除 Agent，并刷新服务器列表（从仪表盘移除已删除的 Agent）
  deleteAgent: async (id: number) => {
    await deleteAgentAPI(id)
    // 原子化删除：先从 servers 和 dashboardData 中同时移除
    const state = get()
    const newMap = new Map(state.dashboardData)
    newMap.delete(id)
    // 标记为最近删除，防止 WS 消息在 fetchServers 完成前重新引入
    const newDeletedIds = new Set(state._recentlyDeletedIds)
    newDeletedIds.add(id)
    set({
      servers: state.servers.filter((s) => s.id !== id),
      dashboardData: newMap,
      _recentlyDeletedIds: newDeletedIds,
    })
    // 异步刷新服务器列表，完成后清除删除标记
    // 成功时立即清除标记；失败时延迟 30 秒清除，避免过早清除导致已删除 Agent 被 WS 重新引入
    get().fetchServers()
      .then(() => {
        const cur = get()
        const next = new Set(cur._recentlyDeletedIds)
        next.delete(id)
        useServerStore.setState({ _recentlyDeletedIds: next })
      })
      .catch(() => {
        setTimeout(() => {
          const cur = get()
          const next = new Set(cur._recentlyDeletedIds)
          next.delete(id)
          useServerStore.setState({ _recentlyDeletedIds: next })
        }, 30000)
      })
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
    const now = Math.floor(Date.now() / 1000)
    const existingIds = new Set(state.servers.map((s) => s.id))
    let newRealtimeHistory = state.realtimeHistory
    const newServersToAdd: ServerData[] = []
    // P2: 更新卡片历史滚动窗口
    const newCardHistory = new Map(state.cardHistory)
    let cardHistoryChanged = false

    for (const item of data) {
      // 跳过最近删除的 Agent，防止 WS 消息在 fetchServers 完成前重新引入
      if (state._recentlyDeletedIds.has(item.agent_id)) continue
      newMap.set(item.agent_id, item)

      // P2: 追加卡片历史数据点（滚动窗口 60 点）
      const cardPoint: CardHistoryPoint = {
        timestamp: item.timestamp || now,
        ping_data: item.ping_data || [],
        online: item.online,
      }
      const prevHistory = newCardHistory.get(item.agent_id) || []
      // 仅在 timestamp 变化时追加，避免重复点
      if (prevHistory.length === 0 || prevHistory[prevHistory.length - 1].timestamp !== cardPoint.timestamp) {
        const updated = [...prevHistory, cardPoint]
        if (updated.length > MAX_CARD_HISTORY_POINTS) {
          newCardHistory.set(item.agent_id, updated.slice(updated.length - MAX_CARD_HISTORY_POINTS))
        } else {
          newCardHistory.set(item.agent_id, updated)
        }
        cardHistoryChanged = true
      }

      // 如果当前正在查看该服务器的详情页，追加实时历史数据
      if (state.currentServer && state.currentServer.id === item.agent_id) {
        const point: RealtimePoint = {
          timestamp: item.timestamp || now,
          cpu: item.cpu,
          mem: item.mem,
          net_rx: item.net_rx,
          net_tx: item.net_tx,
          ping_data: item.ping_data,
          online: item.online ? 1 : 0,
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
          os: item.os || '',
          arch: item.arch || '',
          agent_version: item.agent_version || '',
          online: item.online,
          last_seen: item.timestamp,
          cpu: item.cpu,
          cpu_model: item.cpu_model || '',
          cpu_cores: item.cpu_cores || 0,
          mem: item.mem,
          mem_total: item.mem_total,
          mem_used: item.mem_used,
          swap_total: item.swap_total || 0,
          swap_used: item.swap_used || 0,
          net_rx: item.net_rx,
          net_tx: item.net_tx,
          total_rx: item.total_rx || 0,
          total_tx: item.total_tx || 0,
          uptime: item.uptime,
          load_1: item.load_1 || 0,
          load_5: item.load_5 || 0,
          load_15: item.load_15 || 0,
          disk_usage: item.disk_usage || 0,
          disks: item.disks || [],
          tcp_connections: item.tcp_connections || 0,
          udp_connections: item.udp_connections || 0,
          process_count: item.process_count || 0,
          ping_data: item.ping_data || [],
          virtualization: item.virtualization,
          distro: item.distro,
          processes: item.processes,
          time_offset: item.time_offset,
        })
      }
    }

    // 一次性更新所有状态：合并新服务器并更新已有服务器的实时数据
    const allServers = [...state.servers, ...newServersToAdd]
    const updatedServers = allServers.map((server) => {
      const live = newMap.get(server.id)
      if (live) {
        // 快速检查：若关键字段均未变化，返回原引用避免不必要的重渲染
        if (
          server.online === live.online &&
          server.cpu === (live.cpu || 0) &&
          server.mem === (live.mem || 0) &&
          server.mem_total === live.mem_total &&
          server.mem_used === live.mem_used &&
          server.net_rx === (live.net_rx || 0) &&
          server.net_tx === (live.net_tx || 0) &&
          server.total_rx === (live.total_rx || 0) &&
          server.total_tx === (live.total_tx || 0) &&
          server.disk_usage === (live.disk_usage || 0) &&
          server.uptime === live.uptime &&
          server.swap_total === (live.swap_total || 0) &&
          server.swap_used === (live.swap_used || 0) &&
          server.load_1 === (live.load_1 || 0) &&
          server.load_5 === (live.load_5 || 0) &&
          server.load_15 === (live.load_15 || 0) &&
          server.tcp_connections === (live.tcp_connections ?? server.tcp_connections) &&
          server.udp_connections === (live.udp_connections ?? server.udp_connections) &&
          server.process_count === (live.process_count ?? server.process_count) &&
          server.last_seen === live.timestamp &&
          server.display_name === (live.display_name || server.display_name) &&
          server.hostname === (live.hostname || server.hostname) &&
          server.cpu_model === (live.cpu_model || server.cpu_model) &&
          server.cpu_cores === (live.cpu_cores || server.cpu_cores) &&
          server.os === (live.os || server.os) &&
          server.arch === (live.arch || server.arch) &&
          server.agent_version === (live.agent_version || server.agent_version) &&
          server.virtualization === (live.virtualization ?? server.virtualization) &&
          server.distro === (live.distro ?? server.distro) &&
          server.time_offset === (live.time_offset ?? server.time_offset) &&
          pingDataEqual(server.ping_data, live.ping_data || [])
        ) {
          return server
        }
        return {
          ...server,
          online: live.online,
          cpu: live.cpu || 0,
          cpu_model: live.cpu_model || server.cpu_model,
          cpu_cores: live.cpu_cores || server.cpu_cores,
          mem: live.mem || 0,
          mem_total: live.mem_total,
          mem_used: live.mem_used,
          swap_total: live.swap_total || 0,
          swap_used: live.swap_used || 0,
          net_rx: live.net_rx || 0,
          net_tx: live.net_tx || 0,
          total_rx: live.total_rx || 0,
          total_tx: live.total_tx || 0,
          uptime: live.uptime,
          load_1: live.load_1 || 0,
          load_5: live.load_5 || 0,
          load_15: live.load_15 || 0,
          disk_usage: live.disk_usage || 0,
          // P1-1: DashboardSummary 不含 disks 字段，保留 fetchServers 获取的已有值
          disks: live.disks ?? server.disks ?? [],
          tcp_connections: live.tcp_connections ?? server.tcp_connections ?? 0,
          udp_connections: live.udp_connections ?? server.udp_connections ?? 0,
          process_count: live.process_count ?? server.process_count ?? 0,
          // 若 ping_data 内容未变则保留旧引用，避免不必要重渲染
          ping_data: pingDataEqual(server.ping_data, live.ping_data || [])
            ? server.ping_data
            : (live.ping_data || []),
          last_seen: live.timestamp,
          hostname: live.hostname || server.hostname,
          display_name: live.display_name || server.display_name,
          os: live.os || server.os,
          arch: live.arch || server.arch,
          agent_version: live.agent_version || server.agent_version,
          virtualization: live.virtualization ?? server.virtualization,
          distro: live.distro ?? server.distro,
          processes: live.processes ?? server.processes,
          time_offset: live.time_offset ?? server.time_offset,
        }
      }
      return server
    })

    // 清理不在服务器列表中的过期数据
    const serverIds = new Set(allServers.map((s) => s.id))
    for (const key of newMap.keys()) {
      if (!serverIds.has(key)) {
        newMap.delete(key)
      }
    }
    // 同步清理 cardHistory 中已删除服务器的过期条目
    for (const key of newCardHistory.keys()) {
      if (!serverIds.has(key)) {
        newCardHistory.delete(key)
        cardHistoryChanged = true
      }
    }

    // 检查是否有实际变化，若所有引用都未变则跳过 set 避免不必要重渲染
    const hasChanges = newServersToAdd.length > 0
      || updatedServers.length !== state.servers.length
      || updatedServers.some((s, i) => s !== state.servers[i])

    if (!hasChanges && newRealtimeHistory === state.realtimeHistory && !cardHistoryChanged) {
      return
    }

    set({
      dashboardData: newMap,
      servers: updatedServers,
      realtimeHistory: newRealtimeHistory,
      cardHistory: newCardHistory,
    })
  },

  // 设置主题
  setTheme: (theme: Theme) => {
    try {
      localStorage.setItem('probe_theme', theme)
    } catch {
      // localStorage 不可用（隐私模式等），忽略写入错误
    }
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
          // 触发 set 使订阅 theme 的组件重渲染
          set({ theme: 'system' })
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
