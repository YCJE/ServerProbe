import type { DashboardMessage } from '@/types'
import { getToken } from './api'

/** WebSocket 重连配置 */
const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000, 30000, 60000]
const MAX_RECONNECT_INDEX = RECONNECT_DELAYS.length - 1

/** WebSocket 连接管理器 */
export class DashboardWebSocket {
  private ws: WebSocket | null = null
  private url: string
  private reconnectIndex = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private shouldReconnect = true
  private listeners: Set<(message: DashboardMessage) => void> = new Set()
  private statusListeners: Set<(connected: boolean) => void> = new Set()
  private connected = false

  constructor() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    this.url = `${protocol}//${host}/ws/dashboard`
  }

  /** 建立 WebSocket 连接 */
  connect(): void {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return
    }

    this.shouldReconnect = true
    const token = getToken()

    if (!token) {
      console.warn('[WS] 无 Token，跳过连接')
      return
    }

    // 通过 URL 查询参数传递 Token（兼容性最好）
    const wsUrl = `${this.url}?token=${encodeURIComponent(token)}`

    try {
      this.ws = new WebSocket(wsUrl)
    } catch (err) {
      console.error('[WS] 创建连接失败:', err)
      this.scheduleReconnect()
      return
    }

    this.ws.onopen = () => {
      console.log('[WS] 连接已建立')
      this.reconnectIndex = 0
      this.setConnected(true)
    }

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data) as DashboardMessage
        this.listeners.forEach((listener) => listener(message))
      } catch (err) {
        console.error('[WS] 消息解析失败:', err)
      }
    }

    this.ws.onerror = (event) => {
      console.error('[WS] 连接错误:', event)
    }

    this.ws.onclose = (event) => {
      console.log(`[WS] 连接关闭 (code: ${event.code})`)
      this.setConnected(false)
      this.ws = null
      if (this.shouldReconnect) {
        this.scheduleReconnect()
      }
    }
  }

  /** 断开连接 */
  disconnect(): void {
    this.shouldReconnect = false
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.setConnected(false)
  }

  /** 添加消息监听器 */
  onMessage(listener: (message: DashboardMessage) => void): () => void {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }

  /** 添加连接状态监听器 */
  onStatusChange(listener: (connected: boolean) => void): () => void {
    this.statusListeners.add(listener)
    listener(this.connected)
    return () => {
      this.statusListeners.delete(listener)
    }
  }

  /** 是否已连接 */
  isConnected(): boolean {
    return this.connected
  }

  /** 安排重连 */
  private scheduleReconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
    }

    const delay = RECONNECT_DELAYS[Math.min(this.reconnectIndex, MAX_RECONNECT_INDEX)]
    this.reconnectIndex++
    console.log(`[WS] 将在 ${delay}ms 后重连 (第 ${this.reconnectIndex} 次)`)

    this.reconnectTimer = setTimeout(() => {
      this.connect()
    }, delay)
  }

  /** 设置连接状态并通知监听器 */
  private setConnected(connected: boolean): void {
    if (this.connected !== connected) {
      this.connected = connected
      this.statusListeners.forEach((listener) => listener(connected))
    }
  }
}

/** 全局 WebSocket 实例 */
let dashboardWs: DashboardWebSocket | null = null

/** 获取全局 WebSocket 实例（单例） */
export function getDashboardWebSocket(): DashboardWebSocket {
  if (!dashboardWs) {
    dashboardWs = new DashboardWebSocket()
  }
  return dashboardWs
}
