import { useEffect, useState, useCallback } from 'react'
import {
  generateRegisterCode,
  getRegisterCodes,
  deleteRegisterCode,
  getAgents,
  deleteAgent,
} from '@/lib/api'
import type { RegisterCode, AgentInfo } from '@/types'

/** Agent 管理页 */
export default function AgentManagement() {
  const [codes, setCodes] = useState<RegisterCode[]>([])
  const [agents, setAgents] = useState<AgentInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)
  const [serverUrl, setServerUrl] = useState('')

  // 从当前浏览器地址获取 Server URL
  useEffect(() => {
    const protocol = window.location.protocol
    const host = window.location.host
    setServerUrl(`${protocol}//${host}`)
  }, [])

  // 加载数据
  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [codesRes, agentsRes] = await Promise.all([
        getRegisterCodes(),
        getAgents(),
      ])
      setCodes(codesRes.codes || [])
      setAgents(agentsRes.agents || [])
    } catch (err) {
      console.error('加载数据失败:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  // 生成注册码
  const handleGenerateCode = async () => {
    try {
      await generateRegisterCode()
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '生成注册码失败')
    }
  }

  // 删除注册码
  const handleDeleteCode = async (code: string) => {
    if (!confirm(`确定删除注册码 ${code}？`)) return
    try {
      await deleteRegisterCode(code)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除注册码失败')
    }
  }

  // 删除 Agent
  const handleDeleteAgent = async (id: number, hostname: string) => {
    if (!confirm(`确定删除 Agent "${hostname}"？此操作不可恢复。`)) return
    try {
      await deleteAgent(id)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除 Agent 失败')
    }
  }

  // 复制到剪贴板
  const handleCopy = async (text: string, id: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(id)
      setTimeout(() => setCopied(null), 2000)
    } catch {
      // 降级方案
      const textarea = document.createElement('textarea')
      textarea.value = text
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setCopied(id)
      setTimeout(() => setCopied(null), 2000)
    }
  }

  // 生成一键安装命令
  const getInstallCommand = (code: string) => {
    return `curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-agent.sh | bash -s -- --server ${serverUrl} --code ${code}`
  }

  // 格式化时间
  const formatTime = (timeStr: string) => {
    if (!timeStr) return '-'
    const date = new Date(timeStr)
    return date.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  // 计算剩余时间
  const getRemainingTime = (expiresAt: string) => {
    if (!expiresAt) return ''
    const now = new Date()
    const expire = new Date(expiresAt)
    const diff = Math.floor((expire.getTime() - now.getTime()) / 1000)
    if (diff <= 0) return '已过期'
    const min = Math.floor(diff / 60)
    const sec = diff % 60
    return `${min}分${sec}秒`
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-foreground">Agent 管理</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            生成注册码，在被监控服务器上一键安装 Agent
          </p>
        </div>
        <button
          onClick={handleGenerateCode}
          disabled={loading}
          className="flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
        >
          <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          生成注册码
        </button>
      </div>

      {/* 注册码列表 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">注册码列表</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            每个注册码有效 15 分钟，仅可使用一次。最多保留 5 个未使用注册码。
          </p>
        </div>

        {codes.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无注册码</p>
            <p className="mt-1 text-xs text-muted-foreground/70">点击右上角"生成注册码"按钮创建</p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {codes.map((code) => {
              const installCmd = getInstallCommand(code.code)
              const remaining = getRemainingTime(code.expires_at)
              const isExpired = remaining === '已过期'

              return (
                <div key={code.code} className="p-4">
                  <div className="flex items-start justify-between gap-4">
                    <div className="flex-1">
                      {/* 注册码 */}
                      <div className="flex items-center gap-2">
                        <code className="rounded bg-secondary px-2 py-1 text-sm font-mono font-bold text-secondary-foreground">
                          {code.code}
                        </code>
                        <button
                          onClick={() => handleCopy(code.code, `code-${code.code}`)}
                          className="flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs text-muted-foreground transition-colors hover:bg-accent"
                        >
                          {copied === `code-${code.code}` ? '已复制' : '复制码'}
                        </button>
                        <span className={`text-xs ${isExpired ? 'text-destructive' : 'text-success'}`}>
                          {remaining}
                        </span>
                      </div>

                      {/* 一键安装命令 */}
                      <div className="mt-3">
                        <label className="mb-1 block text-xs text-muted-foreground">
                          一键安装命令 (粘贴到被监控服务器执行)
                        </label>
                        <div className="flex items-start gap-2">
                          <div className="flex-1 overflow-x-auto rounded-lg bg-secondary/50 p-3">
                            <code className="text-xs font-mono text-foreground break-all whitespace-pre-wrap">
                              {installCmd}
                            </code>
                          </div>
                          <button
                            onClick={() => handleCopy(installCmd, `cmd-${code.code}`)}
                            className="flex h-9 shrink-0 items-center gap-1 rounded-lg bg-primary px-3 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90"
                          >
                            {copied === `cmd-${code.code}` ? (
                              <>
                                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                                </svg>
                                已复制
                              </>
                            ) : (
                              <>
                                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                                </svg>
                                复制命令
                              </>
                            )}
                          </button>
                        </div>
                      </div>
                    </div>

                    <button
                      onClick={() => handleDeleteCode(code.code)}
                      className="flex h-8 shrink-0 items-center rounded-lg border border-border px-2 text-xs text-destructive transition-colors hover:bg-destructive/10"
                    >
                      删除
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* 已安装 Agent 列表 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">已安装 Agent ({agents.length})</h2>
        </div>

        {agents.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无已安装的 Agent</p>
            <p className="mt-1 text-xs text-muted-foreground/70">在目标服务器上执行安装命令后，Agent 会自动出现在这里</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs text-muted-foreground">
                  <th className="px-4 py-2 font-medium">ID</th>
                  <th className="px-4 py-2 font-medium">主机名</th>
                  <th className="px-4 py-2 font-medium">系统</th>
                  <th className="px-4 py-2 font-medium">架构</th>
                  <th className="px-4 py-2 font-medium">版本</th>
                  <th className="px-4 py-2 font-medium">状态</th>
                  <th className="px-4 py-2 font-medium">最后在线</th>
                  <th className="px-4 py-2 font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {agents.map((agent) => (
                  <tr key={agent.id} className="text-foreground">
                    <td className="px-4 py-3 text-muted-foreground">{agent.id}</td>
                    <td className="px-4 py-3 font-medium">{agent.hostname || '-'}</td>
                    <td className="px-4 py-3 text-muted-foreground">{agent.os || '-'}</td>
                    <td className="px-4 py-3 text-muted-foreground">{agent.arch || '-'}</td>
                    <td className="px-4 py-3 text-muted-foreground">{agent.agent_version || '-'}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex items-center gap-1 text-xs ${agent.online ? 'text-success' : 'text-muted-foreground'}`}>
                        <span className={`inline-block h-1.5 w-1.5 rounded-full ${agent.online ? 'bg-success' : 'bg-muted-foreground'}`} />
                        {agent.online ? '在线' : '离线'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {formatTime(agent.last_seen)}
                    </td>
                    <td className="px-4 py-3">
                      <button
                        onClick={() => handleDeleteAgent(agent.id, agent.hostname)}
                        className="text-xs text-destructive transition-colors hover:underline"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
