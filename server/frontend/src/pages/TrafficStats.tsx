import { useEffect, useState, useCallback, useMemo } from 'react'
import { useServerStore } from '@/store/useServerStore'
import { getTraffic } from '@/lib/api'
import type { TrafficResponse, MonthlyTraffic, TrafficRecord } from '@/types'
import Skeleton from '@/components/Skeleton'
import EmptyState from '@/components/EmptyState'
import { usePageTitle } from '@/hooks/usePageTitle'

/** 格式化字节数（1024 进制，保留 2 位小数） */
function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let unitIndex = 0
  let value = bytes
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex++
  }
  return `${value.toFixed(2)} ${units[unitIndex]}`
}

/** 格式化日期 (YYYY-MM-DD -> MM-DD) */
function formatDate(date: string): string {
  const parts = date.split('-')
  if (parts.length >= 3) return `${parts[1]}-${parts[2]}`
  return date
}

/** 流量统计页 */
export default function TrafficStats() {
  usePageTitle('流量统计')
  const servers = useServerStore((s) => s.servers)

  const [selectedAgentId, setSelectedAgentId] = useState<number | ''>('')
  const [todayTraffic, setTodayTraffic] = useState<TrafficResponse | null>(null)
  const [monthTraffic, setMonthTraffic] = useState<MonthlyTraffic | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  /** 默认选择第一个 Agent */
  useEffect(() => {
    if (selectedAgentId === '' && servers.length > 0) {
      setSelectedAgentId(servers[0].id)
    }
  }, [servers, selectedAgentId])

  /** 加载流量数据 */
  const loadTraffic = useCallback(async (agentId: number) => {
    setLoading(true)
    setError('')
    try {
      const [todayRes, monthRes] = await Promise.all([
        getTraffic(agentId, 'today'),
        getTraffic(agentId, 'month'),
      ])
      // range=today 返回 TrafficResponse，range=month 返回 MonthlyTraffic
      setTodayTraffic(todayRes as TrafficResponse)
      setMonthTraffic(monthRes as MonthlyTraffic)
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载流量数据失败')
      setTodayTraffic(null)
      setMonthTraffic(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (selectedAgentId !== '') {
      loadTraffic(selectedAgentId)
    }
  }, [selectedAgentId, loadTraffic])

  /** 月度每日最大值（用于柱状图缩放） */
  const maxDailyBytes = useMemo(() => {
    if (!monthTraffic?.records || monthTraffic.records.length === 0) return 0
    return monthTraffic.records.reduce((max, r) => {
      const total = r.rx_bytes + r.tx_bytes
      return total > max ? total : max
    }, 0)
  }, [monthTraffic])

  /** 选中 Agent 的显示名称 */
  const selectedServer = useMemo(() => {
    return servers.find((s) => s.id === selectedAgentId)
  }, [servers, selectedAgentId])

  return (
    <div className="space-y-6">
      {/* 页面标题 */}
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-foreground">流量统计</h1>
        <p className="mt-0.5 text-sm text-muted-foreground">
          查看各 Agent 的当日和当月流量使用情况
        </p>
      </div>

      {/* Agent 选择器 */}
      <div className="card-soft p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
          <label className="text-sm font-medium text-foreground">选择 Agent</label>
          <select
            value={selectedAgentId}
            onChange={(e) => setSelectedAgentId(e.target.value ? Number(e.target.value) : '')}
            className="h-10 w-full max-w-xs rounded-md border border-input bg-background px-3 text-sm shadow-sm text-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
          >
            {servers.length === 0 ? (
              <option value="">暂无 Agent</option>
            ) : (
              servers.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.display_name || s.hostname} (#{s.id})
                </option>
              ))
            )}
          </select>
          {selectedServer && (
            <span className="text-xs text-muted-foreground">
              {selectedServer.online ? (
                <span className="text-emerald-500">● 在线</span>
              ) : (
                <span className="text-red-500">● 离线</span>
              )}
            </span>
          )}
        </div>
      </div>

      {error && (
        <div className="rounded-md border border-dashed border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {selectedAgentId === '' && servers.length === 0 && (
        <div className="card-soft overflow-hidden">
          <EmptyState
            icon={
              <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
              </svg>
            }
            title="暂无 Agent 数据"
            description="请先添加 Agent 后再查看流量统计"
          />
        </div>
      )}

      {loading && <Skeleton variant="section" height={360} />}

      {!loading && selectedAgentId !== '' && (
        <>
          {/* 当日流量概览 */}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div className="card-soft p-4">
              <div className="text-xs font-medium text-muted-foreground">当日接收 (RX)</div>
              <div className="mt-2 text-2xl font-semibold text-success tabular-nums">
                {formatBytes(todayTraffic?.traffic?.rx_bytes || 0)}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                {todayTraffic?.date || '-'}
              </div>
            </div>
            <div className="card-soft p-4">
              <div className="text-xs font-medium text-muted-foreground">当日发送 (TX)</div>
              <div className="mt-2 text-2xl font-semibold text-warning tabular-nums">
                {formatBytes(todayTraffic?.traffic?.tx_bytes || 0)}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                {todayTraffic?.date || '-'}
              </div>
            </div>
            <div className="card-soft p-4">
              <div className="text-xs font-medium text-muted-foreground">当日合计</div>
              <div className="mt-2 text-2xl font-semibold text-foreground tabular-nums">
                {formatBytes((todayTraffic?.traffic?.rx_bytes || 0) + (todayTraffic?.traffic?.tx_bytes || 0))}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                RX + TX
              </div>
            </div>
          </div>

          {/* 当月流量明细 */}
          <div className="card-soft overflow-hidden">
            <div className="border-b border-dashed border-border px-4 py-3">
              <h2 className="text-sm font-semibold text-foreground">
                当月每日流量明细
                {monthTraffic && (
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    共 {monthTraffic.records?.length || 0} 天
                  </span>
                )}
              </h2>
            </div>

            {monthTraffic && monthTraffic.records && monthTraffic.records.length > 0 ? (
              <>
                {/* 柱状图（CSS 实现） */}
                <div className="border-b border-dashed border-border px-4 py-4">
                  <div className="mb-2 flex items-center gap-4 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1.5">
                      <span className="inline-block h-2.5 w-2.5 rounded-sm bg-success" />
                      接收 (RX)
                    </span>
                    <span className="flex items-center gap-1.5">
                      <span className="inline-block h-2.5 w-2.5 rounded-sm bg-warning" />
                      发送 (TX)
                    </span>
                  </div>
                  <div className="flex h-40 items-end gap-1 overflow-x-auto scrollbar-thin">
                    {monthTraffic.records.map((record: TrafficRecord) => {
                      const total = record.rx_bytes + record.tx_bytes
                      const heightPercent = maxDailyBytes > 0 ? (total / maxDailyBytes) * 100 : 0
                      const rxPercent = total > 0 ? (record.rx_bytes / total) * 100 : 0
                      return (
                        <div
                          key={record.id || record.date}
                          className="group relative flex min-w-[18px] flex-1 flex-col justify-end"
                          style={{ height: '100%' }}
                          title={`${formatDate(record.date)}: RX ${formatBytes(record.rx_bytes)} / TX ${formatBytes(record.tx_bytes)}`}
                        >
                          <div
                            className="w-full overflow-hidden rounded-t-sm transition-all"
                            style={{ height: `${Math.max(heightPercent, 2)}%` }}
                          >
                            <div
                              className="bg-warning transition-opacity group-hover:opacity-80"
                              style={{ height: `${100 - rxPercent}%` }}
                            />
                            <div
                              className="bg-success transition-opacity group-hover:opacity-80"
                              style={{ height: `${rxPercent}%` }}
                            />
                          </div>
                          <span className="mt-1 text-center text-[9px] tabular-nums text-muted-foreground/60">
                            {formatDate(record.date).slice(-2)}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                </div>

                {/* 明细表格 */}
                <div className="max-h-80 overflow-y-auto scrollbar-thin">
                  <table className="w-full text-sm">
                    <thead className="sticky top-0 z-10">
                      <tr className="border-b border-border bg-secondary/80 backdrop-blur">
                        <th className="h-10 px-3 text-left font-medium text-muted-foreground">日期</th>
                        <th className="h-10 px-3 text-right font-medium text-muted-foreground">接收 (RX)</th>
                        <th className="h-10 px-3 text-right font-medium text-muted-foreground">发送 (TX)</th>
                        <th className="h-10 px-3 text-right font-medium text-muted-foreground">合计</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-dashed divide-border">
                      {[...monthTraffic.records].reverse().map((record: TrafficRecord) => (
                        <tr key={record.id || record.date} className="text-foreground transition-colors hover:bg-muted/50">
                          <td className="px-3 py-2.5 tabular-nums">{record.date}</td>
                          <td className="px-3 py-2.5 text-right tabular-nums text-success">
                            {formatBytes(record.rx_bytes)}
                          </td>
                          <td className="px-3 py-2.5 text-right tabular-nums text-warning">
                            {formatBytes(record.tx_bytes)}
                          </td>
                          <td className="px-3 py-2.5 text-right tabular-nums font-medium">
                            {formatBytes(record.rx_bytes + record.tx_bytes)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                    <tfoot className="sticky bottom-0 z-10">
                      <tr className="border-t-2 border-border bg-secondary/80 backdrop-blur font-semibold">
                        <td className="px-3 py-3 text-foreground">月汇总</td>
                        <td className="px-3 py-3 text-right tabular-nums text-success">
                          {formatBytes(monthTraffic.total.rx_bytes)}
                        </td>
                        <td className="px-3 py-3 text-right tabular-nums text-warning">
                          {formatBytes(monthTraffic.total.tx_bytes)}
                        </td>
                        <td className="px-3 py-3 text-right tabular-nums text-primary">
                          {formatBytes(monthTraffic.total.rx_bytes + monthTraffic.total.tx_bytes)}
                        </td>
                      </tr>
                    </tfoot>
                  </table>
                </div>
              </>
            ) : (
              <EmptyState
                icon={
                  <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                  </svg>
                }
                title="暂无当月流量数据"
              />
            )}
          </div>
        </>
      )}
    </div>
  )
}
