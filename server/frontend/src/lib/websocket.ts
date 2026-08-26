import type { DashboardMessage } from '@/types'
import { checkAuth, ApiError } from './api'

/** WebSocket 重连配置 */
const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000, 30000, 60000]
const MAX_RECONNECT_INDEX = RECONNECT_DELAYS.length - 1

/**
 * WebSocket 连接管理器
 * 认证方式: HttpOnly Cookie（浏览器自动携带，无需通过 URL 参数传递）
 * - 管理后台 WS: Cookie 中的 JWT Token 由浏览器自动发送
 * - 公开 WS: 无需认证
 */
export class DashboardWebSocket {
  private ws: WebSocket | null = null
  private url: string
  private requireToken: boolean
  private reconnectIndex = 0
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private shouldReconnect = true
  private listeners: Set<(message: DashboardMessage) => void> = new Set()
  private statusListeners: Set<(connected: boolean) => void> = new Set()
  private connected = false

  /**
   * @param path WebSocket 路径，例如 '/ws/dashboard' 或 '/ws/public/dashboard'
   * @param requireToken 是否需要认证（仅影响 401 时的重定向行为）
   */
  constructor(path: string = '/ws/dashboard', requireToken: boolean = true) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    this.url = `${protocol}//${host}${path}`
    this.requireToken = requireToken
  }

  /** 建立 WebSocket 连接（手动调用入口，重置退避索引） */
  connect(): void {
    if (this.ws && (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING)) {
      return
    }

    // 清理待执行的重连定时器，否则定时器稍后触发 openSocket 会造成重复连接
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }

    this.shouldReconnect = true
    // 手动连接重置退避索引，避免上次断连累积的长延迟影响首次重连
    this.reconnectIndex = 0
    this.openSocket()
  }

  /** 实际创建 WebSocket 连接（自动重连复用，保留退避索引） */
  private openSocket(): void {
    // Cookie 由浏览器自动发送，无需在 URL 中传递 Token
    try {
      this.ws = new WebSocket(this.url)
    } catch (err) {
      console.error('[WS] 创建连接失败:', err)
      this.scheduleReconnect()
      return
    }

    this.ws.onopen = () => {
      console.log('[WS] 连接已建立:', this.url)
      this.reconnectIndex = 0
      this.setConnected(true)
    }

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data) as DashboardMessage
        this.listeners.forEach((listener) => {
          try {
            listener(message)
          } catch (err) {
            console.error('[WS] 消息监听器异常:', err)
          }
        })
      } catch (err) {
        console.error('[WS] 消息解析失败:', err)
      }
    }

    this.ws.onerror = (event) => {
      console.error('[WS] 连接错误:', event)
    }

    this.ws.onclose = (event) => {
      console.log(`[WS] 连接关闭 (code: ${event.code}):`, this.url)
      this.setConnected(false)
      this.ws = null

      // 认证失败：后端在 WS 升级前返回 HTTP 401，浏览器触发 onclose code=1006
      // 后端从不发送 4001/4003（保留兼容），实际认证失败表现为 1006
      if (event.code === 4001 || event.code === 4003) {
        this.shouldReconnect = false
        if (this.requireToken && window.location.pathname !== '/login') {
          window.location.replace('/login')
        }
        return
      }

      // 异常关闭（1006）+ 需要认证的连接：可能是 Cookie 过期导致后端拒绝升级
      // 先检查认证状态，避免无限重连 401 端点
      if (event.code === 1006 && this.requireToken && this.shouldReconnect) {
        this.shouldReconnect = false // 暂停重连，等待认证检查结果
        checkAuth()
          .then((res) => {
            if (res.authenticated) {
              // 仍已认证，说明是网络抖动，恢复重连
              this.shouldReconnect = true
              this.scheduleReconnect()
            } else {
              // Cookie 已过期，重定向到登录页
              if (window.location.pathname !== '/login') {
                window.location.replace('/login')
              }
            }
          })
          .catch((err) => {
            if (err instanceof ApiError) {
              // API 返回了错误响应（如 401），Cookie 已失效，重定向到登录页
              if (window.location.pathname !== '/login') {
                window.location.replace('/login')
              }
              return
            }
            // 真正的网络错误，保守恢复重连
            this.shouldReconnect = true
            this.scheduleReconnect()
          })
        return
      }

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
      // 断开前清除所有事件处理器，防止 onclose 竞态触发重连
      this.ws.onopen = null
      this.ws.onmessage = null
      this.ws.onerror = null
      this.ws.onclose = null
      this.ws.close()
      this.ws = null
    }
    this.listeners.clear()
    this.statusListeners.clear()
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
    console.log(`[WS] 将在 ${delay}ms 后重连 (第 ${this.reconnectIndex} 次):`, this.url)

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      // 调用 openSocket 而非 connect，避免重置退避索引导致指数退避失效
      this.openSocket()
    }, delay)
  }

  /** 设置连接状态并通知监听器 */
  private setConnected(connected: boolean) {
    if (this.connected !== connected) {
      this.connected = connected
      this.statusListeners.forEach((listener) => {
        try {
          listener(connected)
        } catch (err) {
          console.error('[WS] 状态监听器异常:', err)
        }
      })
    }
  }
}

/** 全局 WebSocket 实例缓存（按路径区分） */
const wsInstances = new Map<string, DashboardWebSocket>()

/** 获取管理后台仪表盘 WebSocket 实例（单例，需要认证） */
export function getDashboardWebSocket(): DashboardWebSocket {
  const path = '/ws/dashboard'
  if (!wsInstances.has(path)) {
    wsInstances.set(path, new DashboardWebSocket(path, true))
  }
  return wsInstances.get(path)!
}

/** 获取公开仪表盘 WebSocket 实例（单例，无需认证） */
export function getPublicDashboardWebSocket(): DashboardWebSocket {
  const path = '/ws/public/dashboard'
  if (!wsInstances.has(path)) {
    wsInstances.set(path, new DashboardWebSocket(path, false))
  }
  return wsInstances.get(path)!
}
