import { useCallback, useEffect, useState } from 'react'
import {
  getSettings,
  updateSettings,
  getDBStats,
  downloadDBBackup,
  cleanupDBData,
  compactDB,
} from '@/lib/api'
import { useSiteSettingsStore } from '@/store/useSiteSettingsStore'
import type { SystemSettings, DBStats } from '@/types'

/** 历史范围选项 */
const HISTORY_RANGE_OPTIONS = [
  { value: '1h', label: '1 小时' },
  { value: '6h', label: '6 小时' },
  { value: '12h', label: '12 小时' },
  { value: '1d', label: '1 天' },
  { value: '2d', label: '2 天' },
  { value: '3d', label: '3 天' },
]

const DEFAULT_SETTINGS: SystemSettings = {
  site_title: '',
  site_description: '',
  announcement: '',
  custom_footer: '',
  default_history_range: '1h',
  offline_grace_seconds: 90,
  retention_days: 4,
  max_chart_points: 800,
}

/** 字节格式化 */
function formatBytes(bytes: number): string {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

/** 数字格式化 */
function formatNumber(n: number): string {
  if (!n) return '0'
  return n.toLocaleString('zh-CN')
}

/** 区块容器 */
function Section({ title, description, children }: {
  title: string
  description?: string
  children: React.ReactNode
}) {
  return (
    <section className="card-soft p-5">
      <div className="mb-4">
        <h2 className="text-sm font-semibold text-foreground">{title}</h2>
        {description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  )
}

/** 表单字段标签 */
function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-xs font-medium text-foreground">{label}</label>
      {children}
      {hint && <p className="text-[11px] text-muted-foreground">{hint}</p>}
    </div>
  )
}

/** 站点设置页（Komari 风格：站点信息 + 数据加载 + 数据库管理） */
export default function SettingsPage() {
  const [settings, setSettings] = useState<SystemSettings>(DEFAULT_SETTINGS)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [saveMsg, setSaveMsg] = useState('')
  const [saveError, setSaveError] = useState('')

  const [dbStats, setDbStats] = useState<DBStats | null>(null)
  const [dbLoading, setDbLoading] = useState(false)
  const [dbBusy, setDbBusy] = useState<'cleanup' | 'compact' | null>(null)
  const [dbMsg, setDbMsg] = useState('')
  const [dbError, setDbError] = useState('')
  const [cleanupDays, setCleanupDays] = useState(30)

  const loadSettings = useCallback(async () => {
    try {
      const data = await getSettings()
      setSettings(data)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : '加载设置失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const loadDBStats = useCallback(async () => {
    setDbLoading(true)
    try {
      const stats = await getDBStats()
      setDbStats(stats)
    } catch (err) {
      setDbError(err instanceof Error ? err.message : '获取数据库统计失败')
    } finally {
      setDbLoading(false)
    }
  }, [])

  useEffect(() => {
    loadSettings()
    loadDBStats()
  }, [loadSettings, loadDBStats])

  const handleSave = async () => {
    setSaving(true)
    setSaveMsg('')
    setSaveError('')
    try {
      await updateSettings(settings)
      // 刷新站点设置缓存，站点标题/公告/页脚立即生效
      await useSiteSettingsStore.getState().refresh()
      setSaveMsg('设置已保存，运行时参数已实时生效')
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleCleanup = async () => {
    setDbBusy('cleanup')
    setDbMsg('')
    setDbError('')
    try {
      const res = await cleanupDBData(cleanupDays)
      setDbMsg(`${res.message}：删除 ${formatNumber(res.deleted_records)} 条指标记录、${formatNumber(res.deleted_alerts)} 条告警历史`)
      await loadDBStats()
    } catch (err) {
      setDbError(err instanceof Error ? err.message : '清理失败')
    } finally {
      setDbBusy(null)
    }
  }

  const handleCompact = async () => {
    setDbBusy('compact')
    setDbMsg('')
    setDbError('')
    try {
      const res = await compactDB()
      setDbMsg(`${res.message}，当前大小 ${formatBytes(res.db_size_bytes)}`)
      await loadDBStats()
    } catch (err) {
      setDbError(err instanceof Error ? err.message : '压缩失败')
    } finally {
      setDbBusy(null)
    }
  }

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="card-soft h-40 animate-pulse" />
        <div className="card-soft h-40 animate-pulse" />
        <div className="card-soft h-56 animate-pulse" />
      </div>
    )
  }

  return (
    <div className="space-y-5">
      {/* 页头 */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold tracking-tight text-foreground">站点设置</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            站点信息、数据加载参数与数据库管理
          </p>
        </div>
        <button onClick={handleSave} disabled={saving} className="btn-primary">
          {saving ? '保存中…' : '保存设置'}
        </button>
      </div>

      {saveMsg && (
        <div className="rounded-md border border-success/30 bg-success/10 px-4 py-2.5 text-sm text-success">
          {saveMsg}
        </div>
      )}
      {saveError && (
        <div className="rounded-md border border-destructive/30 bg-destructive/10 px-4 py-2.5 text-sm text-destructive">
          {saveError}
        </div>
      )}

      {/* 站点信息 */}
      <Section title="站点信息" description="公开页与管理端展示的站点基础信息">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="站点标题" hint="浏览器标签与页面顶部标题（≤100 字符）">
            <input
              className="input-base"
              value={settings.site_title}
              maxLength={100}
              placeholder="Server Probe"
              onChange={(e) => setSettings({ ...settings, site_title: e.target.value })}
            />
          </Field>
          <Field label="站点描述" hint="副标题（≤300 字符）">
            <input
              className="input-base"
              value={settings.site_description}
              maxLength={300}
              placeholder="安全优先、只读架构的服务器监控探针系统"
              onChange={(e) => setSettings({ ...settings, site_description: e.target.value })}
            />
          </Field>
          <Field label="公告" hint="公开页顶部公告条，留空不显示（≤1000 字符，支持换行）">
            <textarea
              className="input-base min-h-[72px] resize-y"
              value={settings.announcement}
              maxLength={1000}
              placeholder="例如：本站维护中，数据可能延迟"
              onChange={(e) => setSettings({ ...settings, announcement: e.target.value })}
            />
          </Field>
          <Field label="自定义页脚" hint="页脚附加文本，留空不显示（≤500 字符）">
            <input
              className="input-base"
              value={settings.custom_footer}
              maxLength={500}
              placeholder="例如：© 2026 example.com"
              onChange={(e) => setSettings({ ...settings, custom_footer: e.target.value })}
            />
          </Field>
        </div>
      </Section>

      {/* 数据加载设置 */}
      <Section title="数据加载" description="探针数据加载与展示参数，保存后实时生效">
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Field label="默认历史范围" hint="服务器详情页打开时的默认图表范围">
            <select
              className="input-base"
              value={settings.default_history_range}
              onChange={(e) => setSettings({ ...settings, default_history_range: e.target.value })}
            >
              {HISTORY_RANGE_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </Field>
          <Field label="离线宽限期（秒）" hint="Agent 超过该时长无心跳判为离线（30-86400）">
            <input
              type="number"
              className="input-base font-mono"
              min={30}
              max={86400}
              value={settings.offline_grace_seconds}
              onChange={(e) => setSettings({ ...settings, offline_grace_seconds: Number(e.target.value) || 90 })}
            />
          </Field>
          <Field label="数据保留天数" hint="历史指标自动清理周期（1-3650 天）">
            <input
              type="number"
              className="input-base font-mono"
              min={1}
              max={3650}
              value={settings.retention_days}
              onChange={(e) => setSettings({ ...settings, retention_days: Number(e.target.value) || 4 })}
            />
          </Field>
          <Field label="图表最大点数" hint="单次加载的数据点上限，超出自动抽稀（100-2000）">
            <input
              type="number"
              className="input-base font-mono"
              min={100}
              max={2000}
              value={settings.max_chart_points}
              onChange={(e) => setSettings({ ...settings, max_chart_points: Number(e.target.value) || 800 })}
            />
          </Field>
        </div>
      </Section>

      {/* 数据库管理 */}
      <Section
        title="数据库管理"
        description="备份下载、历史数据清理与数据库压缩优化（Komari 风格运维能力）"
      >
        {/* 统计 */}
        <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[
            { label: '数据库大小', value: dbLoading ? '…' : formatBytes(dbStats?.db_size_bytes || 0) },
            { label: 'WAL 大小', value: dbLoading ? '…' : formatBytes(dbStats?.wal_size_bytes || 0) },
            { label: '指标记录', value: dbLoading ? '…' : formatNumber(dbStats?.metric_records || 0) },
            { label: '告警历史', value: dbLoading ? '…' : formatNumber(dbStats?.alert_history || 0) },
            { label: 'Agent 数', value: dbLoading ? '…' : formatNumber(dbStats?.agents || 0) },
            { label: '流量记录', value: dbLoading ? '…' : formatNumber(dbStats?.traffic_records || 0) },
            { label: '服务监控', value: dbLoading ? '…' : formatNumber(dbStats?.service_monitors || 0) },
            { label: 'SSL 监控', value: dbLoading ? '…' : formatNumber(dbStats?.ssl_monitors || 0) },
          ].map((item) => (
            <div key={item.label} className="rounded-md border border-border bg-muted/40 p-3">
              <p className="text-[11px] text-muted-foreground">{item.label}</p>
              <p className="mt-0.5 font-mono text-sm font-medium text-foreground">{item.value}</p>
            </div>
          ))}
        </div>

        {dbMsg && (
          <div className="mb-4 rounded-md border border-success/30 bg-success/10 px-4 py-2.5 text-sm text-success">
            {dbMsg}
          </div>
        )}
        {dbError && (
          <div className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 px-4 py-2.5 text-sm text-destructive">
            {dbError}
          </div>
        )}

        {/* 操作区 */}
        <div className="grid gap-4 lg:grid-cols-3">
          {/* 备份下载 */}
          <div className="rounded-md border border-border p-4">
            <h3 className="text-sm font-medium text-foreground">备份下载</h3>
            <p className="mt-1 text-xs text-muted-foreground">
              使用 SQLite VACUUM INTO 生成一致性快照，不影响在线服务
            </p>
            <button onClick={downloadDBBackup} className="btn-outline mt-3 w-full">
              下载数据库备份
            </button>
          </div>

          {/* 数据清理 */}
          <div className="rounded-md border border-border p-4">
            <h3 className="text-sm font-medium text-foreground">数据清理</h3>
            <p className="mt-1 text-xs text-muted-foreground">
              删除指定天数之前的指标记录与告警历史，不可恢复
            </p>
            <div className="mt-3 flex gap-2">
              <input
                type="number"
                className="input-base w-24 font-mono"
                min={1}
                max={3650}
                value={cleanupDays}
                onChange={(e) => setCleanupDays(Number(e.target.value) || 30)}
              />
              <button
                onClick={handleCleanup}
                disabled={dbBusy !== null}
                className="btn-danger flex-1"
              >
                {dbBusy === 'cleanup' ? '清理中…' : '清理数据'}
              </button>
            </div>
          </div>

          {/* 压缩优化 */}
          <div className="rounded-md border border-border p-4">
            <h3 className="text-sm font-medium text-foreground">压缩优化</h3>
            <p className="mt-1 text-xs text-muted-foreground">
              VACUUM 回收空闲页，清理或大量删除后可减小体积
            </p>
            <button
              onClick={handleCompact}
              disabled={dbBusy !== null}
              className="btn-outline mt-3 w-full"
            >
              {dbBusy === 'compact' ? '压缩中…' : '压缩数据库'}
            </button>
          </div>
        </div>
      </Section>
    </div>
  )
}
