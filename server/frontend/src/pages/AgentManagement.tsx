import { useEffect, useState, useCallback, useRef } from 'react'
import {
  generateRegisterCode,
  getRegisterCodes,
  deleteRegisterCode,
  getAgents,
  createAgent,
  getAgentToken,
  updateAgent,
  updateAgentMeta,
} from '@/lib/api'
import { useServerStore } from '@/store/useServerStore'
import type { RegisterCode, AgentInfo } from '@/types'
import { getFlagEmoji } from '@/lib/utils'

/** 币种选项 */
const CURRENCY_OPTIONS = ['CNY', 'USD', 'EUR', 'JPY', 'GBP', 'HKD', 'TWD', 'KRW', 'SGD']

/** 周期选项 */
const CYCLE_OPTIONS = [
  { value: 'monthly', label: '每月' },
  { value: 'yearly', label: '每年' },
  { value: 'quarterly', label: '每季' },
  { value: 'weekly', label: '每周' },
]

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

  // 编辑 Agent 状态
  const [editingAgent, setEditingAgent] = useState<AgentInfo | null>(null)
  const [editDisplayName, setEditDisplayName] = useState('')
  const [editTags, setEditTags] = useState('')
  const [editError, setEditError] = useState('')
  const [editSaving, setEditSaving] = useState(false)
  const [regenerating, setRegenerating] = useState(false)
  const [regeneratedCode, setRegeneratedCode] = useState<string | null>(null)

  // NodeGet 元数据编辑状态
  const [metaRegion, setMetaRegion] = useState('')
  const [metaCountryCode, setMetaCountryCode] = useState('')
  const [metaISP, setMetaISP] = useState('')
  const [metaExpiresAt, setMetaExpiresAt] = useState('')
  const [metaPriceAmount, setMetaPriceAmount] = useState('')
  const [metaPriceCurrency, setMetaPriceCurrency] = useState('CNY')
  const [metaPriceCycle, setMetaPriceCycle] = useState('monthly')
  const [metaTrafficQuotaGB, setMetaTrafficQuotaGB] = useState('')

  // 添加服务器（Komari 风格两步式）状态
  const [creating, setCreating] = useState(false)
  // 创建成功后展示的安装命令信息（null = 未进入第二步）
  const [createdAgent, setCreatedAgent] = useState<{
    agent_id: number
    display_name: string
    token: string
  } | null>(null)

  // Agent 列表"安装命令"弹窗状态（复用安装命令展示弹窗）
  const [cmdAgent, setCmdAgent] = useState<{ agent_id: number; display_name: string; token: string } | null>(null)
  const [cmdLoading, setCmdLoading] = useState(false)

  // 从 store 获取 fetchServers 和 deleteAgent，用于删除后刷新仪表盘
  const fetchServers = useServerStore((s) => s.fetchServers)
  const deleteAgentFromStore = useServerStore((s) => s.deleteAgent)

  // 跟踪复制按钮的定时器，卸载时清理
  const timeoutRefs = useRef<ReturnType<typeof setTimeout>[]>([])

  // 每秒触发一次重渲染，用于更新注册码倒计时（仅在有活跃注册码时运行）
  const [, setTick] = useState(0)
  const hasActiveCodes = codes.some((c) => !c.used && new Date(c.expires_at).getTime() > Date.now())
  useEffect(() => {
    if (!hasActiveCodes) return
    const interval = setInterval(() => {
      setTick((t) => t + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [hasActiveCodes])

  // 卸载时清理所有复制按钮的定时器
  useEffect(() => {
    return () => {
      timeoutRefs.current.forEach(clearTimeout)
      timeoutRefs.current = []
    }
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

  // 添加服务器（Komari 风格第一步：填写基本信息 → 直接创建记录并生成 Token）
  const handleCreateAgent = async () => {
    setFormError('')
    if (!displayName.trim()) {
      setFormError('请输入服务器名称')
      return
    }

    setCreating(true)
    try {
      const result = await createAgent(displayName.trim())
      setDisplayName('')
      setRemark('')
      setCreatedAgent(result)
      await loadData()
      await fetchServers()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : '添加服务器失败')
    } finally {
      setCreating(false)
    }
  }

  // 关闭安装命令弹窗（创建第二步 或 Agent 列表安装命令）
  const handleCloseInstallCmd = () => {
    setCreatedAgent(null)
    setCmdAgent(null)
  }

  // Agent 列表：获取 Token 并展示该 Agent 的安装命令（重装/换机场景）
  const handleShowInstallCmd = async (agent: AgentInfo) => {
    setCmdLoading(true)
    try {
      const result = await getAgentToken(agent.id)
      setCmdAgent(result)
    } catch (err) {
      alert(err instanceof Error ? err.message : '获取安装命令失败')
    } finally {
      setCmdLoading(false)
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
      // 使用 store 的 deleteAgent action，删除后会自动刷新仪表盘数据
      await deleteAgentFromStore(id)
      await loadData()
    } catch (err) {
      alert(err instanceof Error ? err.message : '删除 Agent 失败')
    }
  }

  // 打开编辑弹窗（初始化显示名称 + NodeGet 元数据表单）
  const handleOpenEdit = (agent: AgentInfo) => {
    setEditingAgent(agent)
    setEditDisplayName(agent.display_name || '')
    setEditTags(agent.tags || '')
    setMetaRegion(agent.region || '')
    setMetaCountryCode((agent.country_code || '').toUpperCase())
    setMetaISP(agent.isp || '')
    // RFC3339 -> YYYY-MM-DD（date input 格式），空 = 永不过期
    setMetaExpiresAt(agent.expires_at ? agent.expires_at.slice(0, 10) : '')
    setMetaPriceAmount(agent.price_amount ? String(agent.price_amount) : '')
    setMetaPriceCurrency(agent.price_currency || 'CNY')
    setMetaPriceCycle(agent.price_cycle || 'monthly')
    // 字节 -> GB（向下取整展示，0/空 = 不限）
    setMetaTrafficQuotaGB(
      agent.traffic_quota_bytes ? String(Math.round(agent.traffic_quota_bytes / 1024 ** 3)) : '',
    )
    setEditError('')
  }

  // 关闭编辑弹窗
  const handleCloseEdit = () => {
    setEditingAgent(null)
    setEditDisplayName('')
    setEditTags('')
    setMetaRegion('')
    setMetaCountryCode('')
    setMetaISP('')
    setMetaExpiresAt('')
    setMetaPriceAmount('')
    setMetaTrafficQuotaGB('')
    setEditError('')
    setRegeneratedCode(null)
  }

  // 重新生成安装命令（为已有 Agent 生成新注册码）
  const handleRegenerateCode = async () => {
    if (!editingAgent) return
    setRegenerating(true)
    setEditError('')
    try {
      const result = await generateRegisterCode(
        editingAgent.display_name || editingAgent.hostname || `Agent-${editingAgent.id}`,
        `重新生成 - ${editingAgent.hostname || ''}`,
      )
      setRegeneratedCode(result.code)
      await loadData()
    } catch (err) {
      setEditError(err instanceof Error ? err.message : '生成注册码失败')
    } finally {
      setRegenerating(false)
    }
  }

  // 保存编辑（显示名称 + 标签 + NodeGet 元数据）
  const handleSaveEdit = async () => {
    if (!editingAgent) return
    setEditError('')
    if (!editDisplayName.trim()) {
      setEditError('请输入显示名称')
      return
    }
    // 国家代码格式校验（填写时必须为 2 位字母）
    const cc = metaCountryCode.trim().toUpperCase()
    if (cc && !/^[A-Z]{2}$/.test(cc)) {
      setEditError('国家代码必须为 2 位字母（如 CN、US、JP），留空则从名称自动推断')
      return
    }
    // 费用与配额数值校验
    const priceAmount = metaPriceAmount.trim() ? parseFloat(metaPriceAmount) : 0
    if (isNaN(priceAmount) || priceAmount < 0) {
      setEditError('费用必须为非负数字')
      return
    }
    const quotaGB = metaTrafficQuotaGB.trim() ? parseFloat(metaTrafficQuotaGB) : 0
    if (isNaN(quotaGB) || quotaGB < 0) {
      setEditError('月流量配额必须为非负数字（GB）')
      return
    }

    setEditSaving(true)
    try {
      // 1. 保存显示名称 + 标签
      await updateAgent(editingAgent.id, {
        display_name: editDisplayName.trim(),
        tags: editTags.trim(),
      })
      // 2. 保存 NodeGet 元数据
      await updateAgentMeta(editingAgent.id, {
        region: metaRegion.trim(),
        country_code: cc,
        isp: metaISP.trim(),
        expires_at: metaExpiresAt.trim(),
        price_amount: priceAmount,
        price_currency: metaPriceCurrency,
        price_cycle: metaPriceCycle,
        traffic_quota_bytes: Math.round(quotaGB * 1024 ** 3),
      })
      handleCloseEdit()
      await loadData()
      // 同步刷新仪表盘数据，确保卡片显示最新元数据
      await fetchServers()
    } catch (err) {
      setEditError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setEditSaving(false)
    }
  }

  // 复制到剪贴板
  const setCopiedWithTimeout = (id: string) => {
    setCopied(id)
    const t = setTimeout(() => {
      setCopied(null)
      // 超时执行后从数组中移除自身，避免过期 ID 无限累积
      timeoutRefs.current = timeoutRefs.current.filter((ref) => ref !== t)
    }, 2000)
    timeoutRefs.current.push(t)
  }

  const handleCopy = async (text: string, id: string) => {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedWithTimeout(id)
    } catch {
      // 降级方案
      const textarea = document.createElement('textarea')
      textarea.value = text
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
      setCopiedWithTimeout(id)
    }
  }

  // 生成一键安装命令（对参数加单引号防止 shell 注入）
  const shellQuote = (s: string) => `'${String(s).replace(/'/g, `'\\''`)}'`
  const getInstallCommand = (code: string) => {
    return `curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-agent.sh | bash -s -- --server ${shellQuote(serverUrl)} --code ${shellQuote(code)}`
  }
  // Token 版一键安装命令（Komari 风格：后台直接创建的 Agent 用 Token 直连）
  const getTokenInstallCommand = (token: string) => {
    return `curl -fsSL https://raw.githubusercontent.com/YCJE/ServerProbe/master/scripts/install-agent.sh | bash -s -- --server ${shellQuote(serverUrl)} --token ${shellQuote(token)}`
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
    if (isNaN(expire)) return '无效日期'
    const diff = Math.floor((expire - now) / 1000)
    if (diff <= 0) return '已过期'
    const min = Math.floor(diff / 60)
    const sec = diff % 60
    return `${min}m ${sec}s`
  }

  // 判断是否过期
  const isExpired = (expiresAt: string) => {
    if (!expiresAt) return true
    const expire = new Date(expiresAt).getTime()
    return isNaN(expire) || expire <= Date.now()
  }

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-xl font-bold text-primary">Agent 管理</h1>
        <p className="mt-0.5 text-sm text-muted-foreground">
          先添加服务器信息，再复制一键安装命令到被监控服务器执行即可接入监控
        </p>
      </div>

      {/* 添加服务器表单（Komari 风格：先添加基本信息，再复制一键命令到被控服务器执行） */}
      <div className="card-soft">
        <div className="border-b border-dashed border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">添加服务器</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">
            第 1 步：填写服务器基本信息；第 2 步：复制一键安装命令到被监控服务器执行，Agent 将自动连接并上报数据
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
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !creating) handleCreateAgent()
                }}
                placeholder="例如：Web 服务器 01"
                className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
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
                className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
              />
            </div>
            <button
              onClick={handleCreateAgent}
              disabled={creating}
              className="flex h-10 shrink-0 items-center gap-1.5 rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {creating ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent" />
                  添加中...
                </>
              ) : (
                <>
                  <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                  </svg>
                  添加服务器
                </>
              )}
            </button>
            <button
              onClick={handleGenerateCode}
              disabled={generating}
              title="兼容旧流程：生成 15 分钟有效的一次性注册码"
              className="flex h-10 shrink-0 items-center gap-1.5 rounded-xl border border-border px-4 text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:opacity-50"
            >
              {generating ? '生成中...' : '生成注册码'}
            </button>
          </div>
          {formError && (
            <p className="mt-2 text-xs text-destructive">{formError}</p>
          )}
        </div>
      </div>

      {/* 注册码列表 */}
      <div className="card-soft">
        <div className="border-b border-dashed border-border px-4 py-3">
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
          <div className="divide-y divide-dashed divide-border">
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
                          <span className="badge-pill badge-warning">已使用</span>
                        )}
                      </div>
                      {code.remark && (
                        <p className="mt-0.5 truncate text-xs text-muted-foreground">
                          备注：{code.remark}
                        </p>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <span className={`text-xs font-medium tabular-nums ${expired ? 'text-destructive' : 'text-success'}`}>
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
                    <span className="badge-pill badge-primary font-mono font-bold">
                      {code.code}
                    </span>
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
                      <div className="flex-1 overflow-x-auto rounded-md bg-secondary/50 p-3 scrollbar-thin">
                        <code className="text-xs font-mono text-foreground break-all whitespace-pre-wrap">
                          {installCmd}
                        </code>
                      </div>
                      <button
                        onClick={() => handleCopy(installCmd, `cmd-${code.code}`)}
                        className="flex h-9 shrink-0 items-center gap-1 rounded-xl bg-primary px-3 text-xs font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
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
      <div className="card-soft overflow-hidden">
        <div className="border-b border-dashed border-border px-4 py-3">
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
          <div className="overflow-x-auto scrollbar-thin">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-secondary/30">
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">ID</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">显示名称</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">主机名</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">系统</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">架构</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">版本</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">状态</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">最后在线</th>
                  <th className="h-10 px-3 text-left font-medium text-muted-foreground">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-dashed divide-border">
                {agents.map((agent) => (
                  <tr key={agent.id} className="text-foreground transition-colors hover:bg-muted/50">
                    <td className="px-3 py-3 tabular-nums text-muted-foreground">{agent.id}</td>
                    <td className="px-3 py-3 font-medium">{agent.display_name || '-'}</td>
                    <td className="px-3 py-3 text-muted-foreground">{agent.hostname || '-'}</td>
                    <td className="px-3 py-3 text-muted-foreground">{agent.os || '-'}</td>
                    <td className="px-3 py-3 text-muted-foreground">{agent.arch || '-'}</td>
                    <td className="px-3 py-3 text-muted-foreground">{agent.agent_version || '-'}</td>
                    <td className="px-3 py-3">
                      <span className={`badge-pill ${agent.online ? 'badge-success' : 'badge-destructive'}`}>
                        <span className={`inline-block h-1.5 w-1.5 rounded-full ${agent.online ? 'bg-success' : 'bg-muted-foreground'}`} />
                        {agent.online ? '在线' : '离线'}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs tabular-nums text-muted-foreground">
                      {formatTime(agent.last_seen)}
                    </td>
                    <td className="px-3 py-3">
                      <div className="flex items-center gap-2">
                        <button
                          onClick={() => handleShowInstallCmd(agent)}
                          disabled={cmdLoading}
                          className="text-xs font-medium text-primary transition-colors hover:underline disabled:opacity-50"
                        >
                          安装命令
                        </button>
                        <button
                          onClick={() => handleOpenEdit(agent)}
                          className="text-xs font-medium text-primary transition-colors hover:underline"
                        >
                          编辑
                        </button>
                        <button
                          onClick={() => handleDeleteAgent(agent.id, agent.display_name || agent.hostname)}
                          className="text-xs font-medium text-destructive transition-colors hover:underline"
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 一键安装命令弹窗（添加服务器第 2 步 / Agent 列表重装命令） */}
      {(createdAgent || cmdAgent) && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={handleCloseInstallCmd}>
          <div
            className="w-full max-w-xl card-soft p-4 sm:p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <div>
                <h3 className="text-base font-semibold text-foreground">
                  {createdAgent ? '第 2 步：执行一键安装命令' : '一键安装命令'}
                </h3>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {createdAgent
                    ? `服务器 "${createdAgent.display_name}" 已创建，复制以下命令到被监控服务器上以 root 执行`
                    : `服务器 "${cmdAgent?.display_name || '-'}" 的安装命令（可用于重装或迁移）`}
                </p>
              </div>
              <button
                onClick={handleCloseInstallCmd}
                className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
              >
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* 步骤提示 */}
            {createdAgent && (
              <div className="mb-4 flex items-center gap-2">
                <span className="badge-pill badge-success">1. 填写基本信息 ✓</span>
                <svg className="h-3 w-3 text-muted-foreground" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                </svg>
                <span className="badge-pill badge-primary">2. 执行安装命令</span>
              </div>
            )}

            {/* 安装命令 */}
            <div>
              <label className="mb-1 block text-xs text-muted-foreground">
                一键安装命令 (粘贴到被监控服务器执行)
              </label>
              <div className="flex items-start gap-2">
                <div className="flex-1 overflow-x-auto rounded-md bg-secondary/50 p-3 scrollbar-thin">
                  <code className="text-xs font-mono text-foreground break-all whitespace-pre-wrap">
                    {getTokenInstallCommand((createdAgent || cmdAgent)!.token)}
                  </code>
                </div>
                <button
                  onClick={() => handleCopy(getTokenInstallCommand((createdAgent || cmdAgent)!.token), 'token-cmd')}
                  className="flex h-9 shrink-0 items-center gap-1 rounded-xl bg-primary px-3 text-xs font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
                >
                  {copied === 'token-cmd' ? (
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

            {/* 提示信息 */}
            <div className="mt-4 rounded-md border border-dashed border-border bg-secondary/30 p-3 text-xs text-muted-foreground">
              <p>· 命令执行后 Agent 会自动连接主控并开始上报数据（首次连接自动记录主机信息）</p>
              <p>· 安装完成后可在下方"已安装 Agent"列表查看，离线状态会在连接成功后变为在线</p>
              <p>· Token 即该服务器的身份凭证，请勿泄露给他人</p>
            </div>

            {/* 完成按钮 */}
            <div className="mt-4 flex justify-end">
              <button
                onClick={handleCloseInstallCmd}
                className="flex h-9 items-center rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
              >
                完成
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 编辑 Agent 弹窗 */}
      {editingAgent && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={handleCloseEdit}>
          <div
            className="max-h-[90vh] w-full max-w-lg overflow-y-auto card-soft p-4 sm:p-6 scrollbar-thin"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-4 flex items-center justify-between">
              <h3 className="text-base font-semibold text-foreground">编辑 Agent</h3>
              <button
                onClick={handleCloseEdit}
                className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent"
              >
                <svg className="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <div className="space-y-4">
              {/* 显示名称 */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  显示名称 <span className="text-destructive">*</span>
                </label>
                <input
                  type="text"
                  value={editDisplayName}
                  onChange={(e) => setEditDisplayName(e.target.value)}
                  placeholder="例如：Web 服务器 01"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                  autoFocus
                />
              </div>

              {/* 标签（逗号分隔） */}
              <div>
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  标签 <span className="text-muted-foreground/60">(逗号分隔，如 "生产,中转")</span>
                </label>
                <input
                  type="text"
                  value={editTags}
                  onChange={(e) => setEditTags(e.target.value)}
                  placeholder="例如：生产,中转"
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              {/* 只读信息 */}
              <div className="rounded-md border border-dashed border-border bg-secondary/30 p-3 text-xs text-muted-foreground">
                <div className="flex justify-between py-0.5">
                  <span>ID</span>
                  <span className="font-mono font-bold tabular-nums text-foreground">{editingAgent.id}</span>
                </div>
                <div className="flex justify-between py-0.5">
                  <span>主机名</span>
                  <span className="font-mono font-bold text-foreground">{editingAgent.hostname || '-'}</span>
                </div>
                <div className="flex justify-between py-0.5">
                  <span>系统</span>
                  <span className="font-bold text-foreground">{editingAgent.os || '-'}</span>
                </div>
                {(editingAgent.ipv4 || editingAgent.ipv6) && (
                  <div className="flex justify-between py-0.5">
                    <span>出口 IP</span>
                    <span className="truncate font-mono text-foreground">
                      {[editingAgent.ipv4, editingAgent.ipv6].filter(Boolean).join(' / ')}
                    </span>
                  </div>
                )}
              </div>

              {/* NodeGet 元数据 */}
              <div className="rounded-md border border-dashed border-border p-3">
                <h4 className="mb-3 text-xs font-semibold text-foreground">节点信息（位置 / 到期 / 费用 / 流量）</h4>
                <div className="grid grid-cols-2 gap-3">
                  {/* 位置 */}
                  <div>
                    <label className="mb-1 block text-xs text-muted-foreground">位置</label>
                    <input
                      type="text"
                      value={metaRegion}
                      onChange={(e) => setMetaRegion(e.target.value)}
                      placeholder="如：上海 / Tokyo"
                      className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none"
                    />
                  </div>
                  {/* 国家代码 */}
                  <div>
                    <label className="mb-1 block text-xs text-muted-foreground">
                      国家代码 {metaCountryCode && /^[A-Za-z]{2}$/.test(metaCountryCode) && getFlagEmoji(metaCountryCode.toUpperCase())}
                    </label>
                    <input
                      type="text"
                      value={metaCountryCode}
                      onChange={(e) => setMetaCountryCode(e.target.value.toUpperCase())}
                      placeholder="如：CN / US / JP"
                      maxLength={2}
                      className="h-9 w-full rounded-md border border-input bg-background px-2.5 font-mono text-sm uppercase text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none"
                    />
                  </div>
                  {/* 供应商 */}
                  <div>
                    <label className="mb-1 block text-xs text-muted-foreground">供应商</label>
                    <input
                      type="text"
                      value={metaISP}
                      onChange={(e) => setMetaISP(e.target.value)}
                      placeholder="如：Bandwagon / Oracle"
                      className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none"
                    />
                  </div>
                  {/* 到期时间 */}
                  <div>
                    <label className="mb-1 block text-xs text-muted-foreground">到期时间</label>
                    <input
                      type="date"
                      value={metaExpiresAt}
                      onChange={(e) => setMetaExpiresAt(e.target.value)}
                      className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm text-foreground focus:border-primary focus:outline-none"
                    />
                    <p className="mt-0.5 text-[10px] text-muted-foreground/70">留空 = 永不过期</p>
                  </div>
                  {/* 费用金额 */}
                  <div>
                    <label className="mb-1 block text-xs text-muted-foreground">费用金额</label>
                    <input
                      type="number"
                      min="0"
                      step="0.01"
                      value={metaPriceAmount}
                      onChange={(e) => setMetaPriceAmount(e.target.value)}
                      placeholder="0"
                      className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm tabular-nums text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none"
                    />
                  </div>
                  {/* 币种 + 周期 */}
                  <div className="grid grid-cols-2 gap-2">
                    <div>
                      <label className="mb-1 block text-xs text-muted-foreground">币种</label>
                      <select
                        value={metaPriceCurrency}
                        onChange={(e) => setMetaPriceCurrency(e.target.value)}
                        className="h-9 w-full cursor-pointer rounded-md border border-input bg-background px-1.5 text-sm text-foreground focus:border-primary focus:outline-none"
                      >
                        {CURRENCY_OPTIONS.map((c) => (
                          <option key={c} value={c}>{c}</option>
                        ))}
                      </select>
                    </div>
                    <div>
                      <label className="mb-1 block text-xs text-muted-foreground">周期</label>
                      <select
                        value={metaPriceCycle}
                        onChange={(e) => setMetaPriceCycle(e.target.value)}
                        className="h-9 w-full cursor-pointer rounded-md border border-input bg-background px-1.5 text-sm text-foreground focus:border-primary focus:outline-none"
                      >
                        {CYCLE_OPTIONS.map((c) => (
                          <option key={c.value} value={c.value}>{c.label}</option>
                        ))}
                      </select>
                    </div>
                  </div>
                  {/* 月流量配额 */}
                  <div className="col-span-2">
                    <label className="mb-1 block text-xs text-muted-foreground">
                      月流量配额 (GB) <span className="text-muted-foreground/60">(0 或留空 = 不限)</span>
                    </label>
                    <input
                      type="number"
                      min="0"
                      step="1"
                      value={metaTrafficQuotaGB}
                      onChange={(e) => setMetaTrafficQuotaGB(e.target.value)}
                      placeholder="如：1000"
                      className="h-9 w-full rounded-md border border-input bg-background px-2.5 text-sm tabular-nums text-foreground placeholder:text-muted-foreground/60 focus:border-primary focus:outline-none"
                    />
                  </div>
                </div>
              </div>

              {/* 重新生成安装命令 */}
              <div className="rounded-md border border-dashed border-border p-3">
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-xs font-medium text-muted-foreground">重新生成安装命令</span>
                  <button
                    onClick={handleRegenerateCode}
                    disabled={regenerating}
                    className="flex h-7 items-center gap-1 rounded-md border border-border px-2 text-xs text-primary transition-colors hover:bg-accent disabled:opacity-50"
                  >
                    {regenerating ? '生成中...' : '生成新命令'}
                  </button>
                </div>
                {regeneratedCode ? (
                  <div>
                    <div className="flex items-start gap-2">
                      <div className="flex-1 overflow-x-auto rounded-md bg-secondary/50 p-2 scrollbar-thin">
                        <code className="text-xs font-mono text-foreground break-all whitespace-pre-wrap">
                          {getInstallCommand(regeneratedCode)}
                        </code>
                      </div>
                      <button
                        onClick={() => handleCopy(getInstallCommand(regeneratedCode), `regen-${regeneratedCode}`)}
                        className="flex h-8 shrink-0 items-center gap-1 rounded-xl bg-primary px-2 text-xs font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
                      >
                        {copied === `regen-${regeneratedCode}` ? '已复制' : '复制'}
                      </button>
                    </div>
                    <p className="mt-1.5 text-xs text-muted-foreground">
                      注册码有效 15 分钟，在被监控服务器上执行即可重新安装 Agent
                    </p>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground/70">
                    如果 Agent 离线或信息变更，可重新生成安装命令
                  </p>
                )}
              </div>

              {editError && (
                <p className="text-xs text-destructive">{editError}</p>
              )}
            </div>

            {/* 操作按钮 */}
            <div className="mt-6 flex items-center justify-end gap-2">
              <button
                onClick={handleCloseEdit}
                className="flex h-10 items-center rounded-xl border border-border bg-secondary px-4 text-sm font-semibold text-foreground transition-colors hover:bg-accent"
              >
                取消
              </button>
              <button
                onClick={handleSaveEdit}
                disabled={editSaving}
                className="flex h-10 items-center rounded-xl bg-primary px-4 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
              >
                {editSaving ? '保存中...' : '保存'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
