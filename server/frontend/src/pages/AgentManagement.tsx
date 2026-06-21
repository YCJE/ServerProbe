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
  const [generating, setGenerating] = useState(false)
  const [copied, setCopied] = useState<string | null>(null)
  const [serverUrl, setServerUrl] = useState('')

  // 表单状态
  const [displayName, setDisplayName] = useState('')
  const [remark, setRemark] = useState('')
  const [formError, setFormError] = useState('')

  // 每秒触发一次重渲染，用于更新倒计时
  const [, setTick] = useState(0)
  useEffect(() => {
    const interval = setInterval(() => {
      setTick((t) => t + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [])

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
    setFormError('')
    if (!displayName.trim()) {
      setFormError('请输入服务器名称')
      return
    }

    setGenerating(true)
    try {
      await generateRegisterCode(displayName.trim(), remark.trim())
      setDisplayName('')
      setRemark('')
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '生成注册码失败')
    } finally {
      setGenerating(false)
    }
  }

  // 删除注册码
  const handleDeleteCode = async (code: string) => {
    if (!confirm(`确定删除该注册码？`)) return
    try {
      await deleteRegisterCode(code)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除注册码失败')
    }
  }

  // 删除 Agent
  const handleDeleteAgent = async (id: number, name: string) => {
    if (!confirm(`确定删除 Agent "${name}"？此操作不可恢复。`)) return
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

  // 计算剩余时间，格式 "Xm Ys"
  const getRemainingTime = (expiresAt: string) => {
    if (!expiresAt) return '已过期'
    const now = Date.now()
    const expire = new Date(expiresAt).getTime()
    const diff = Math.floor((expire - now) / 1000)
    if (diff <= 0) return '已过期'
    const min = Math.floor(diff / 60)
    const sec = diff % 60
    return `${min}m ${sec}s`
  }

  // 判断是否过期
  const isExpired = (expiresAt: string) => {
    if (!expiresAt) return true
    return new Date(expiresAt).getTime() <= Date.now()
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-xl font-bold text-foreground">Agent 管理</h1>
        <p className="mt-0.5 text-sm text-muted-foreground">
          生成注册码，在被监控服务器上一键安装 Agent
        </p>
      </div>

      {/* 添加服务器表单 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">添加服务器</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            填写服务器信息后生成注册码，用于在被监控服务器上安装 Agent
          </p>
        </div>
        <div className="p-4">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <div className="flex-1">
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                服务器名称 <span className="text-destructive">*</span>
              </label>
              <input
                type="text"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="例如：Web 服务器 01"
                className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
            <div className="flex-1">
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                备注 <span className="text-muted-foreground/60">(可选)</span>
              </label>
              <input
                type="text"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
                placeholder="例如：生产环境"
                className="h-9 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
            <button
              onClick={handleGenerateCode}
              disabled={generating}
              className="flex h-9 shrink-0 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {generating ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent" />
                  生成中...
                </>
              ) : (
                <>
                  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  生成注册码
                </>
              )}
            </button>
          </div>
          {formError && (
            <p className="mt-2 text-xs text-destructive">{formError}</p>
          )}
        </div>
      </div>

      {/* 注册码列表 */}
      <div className="rounded-xl border border-border bg-card">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">注册码列表 ({codes.length})</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            每个注册码有效 15 分钟，仅可使用一次。将安装命令在被监控服务器上执行即可完成注册。
          </p>
        </div>

        {loading && codes.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : codes.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12">
            <svg className="mb-3 h-10 w-10 text-muted-foreground/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
            <p className="text-sm text-muted-foreground">暂无注册码</p>
            <p className="mt-1 text-xs text-muted-foreground/70">在上方填写服务器信息后生成注册码</p>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {codes.map((code) => {
              const installCmd = getInstallCommand(code.code)
              const remaining = getRemainingTime(code.expires_at)
              const expired = isExpired(code.expires_at)

              return (
                <div key={code.code} className="p-4">
                  {/* 顶部：服务器名称 + 备注 + 倒计时 + 删除 */}
                  <div className="mb-3 flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <h3 className="truncate text-sm font-semibold text-foreground">
                          {code.display_name || '未命名'}
                        </h3>
                        {code.used && (
                          <span className="rounded bg-secondary px-1.5 py-0.5 text-xs text-secondary-foreground">
                            已使用
                          </span>
                        )}
                      </div>
                      {code.remark && (
                        <p className="mt-0.5 truncate text-xs text-muted-foreground">
                          备注：{code.remark}
                        </p>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <span className={`text-xs font-medium ${expired ? 'text-destructive' : 'text-success'}`}>
                        {remaining}
                      </span>
                      <button
                        onClick={() => handleDeleteCode(code.code)}
                        className="flex h-7 items-center rounded-md border border-border px-2 text-xs text-destructive transition-colors hover:bg-destructive/10"
                      >
                        删除
                      </button>
                    </div>
                  </div>

                  {/* 注册码 */}
                  <div className="mb-3 flex items-center gap-2">
                    <span className="text-xs text-muted-foreground">注册码：</span>
                    <code className="rounded bg-secondary px-2 py-1 text-sm font-mono font-bold text-secondary-foreground">
                      {code.code}
                    </code>
                    <button
                      onClick={() => handleCopy(code.code, `code-${code.code}`)}
                      className="flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs text-muted-foreground transition-colors hover:bg-accent"
                    >
                      {copied === `code-${code.code}` ? '已复制' : '复制码'}
                    </button>
                  </div>

                  {/* 一键安装命令 */}
                  <div>
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

        {loading && agents.length === 0 ? (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        ) : agents.length === 0 ? (
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
                  <th className="px-4 py-2 font-medium">显示名称</th>
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
                    <td className="px-4 py-3 font-medium">{agent.display_name || '-'}</td>
                    <td className="px-4 py-3 text-muted-foreground">{agent.hostname || '-'}</td>
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
                        onClick={() => handleDeleteAgent(agent.id, agent.display_name || agent.hostname)}
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
