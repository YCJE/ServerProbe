import { useEffect } from 'react'
import { create } from 'zustand'
import { getPublicSettings } from '@/lib/api'

/** 默认站点设置（后端未配置时兜底） */
export const DEFAULT_SITE_SETTINGS = {
  site_title: '服务器探针',
  site_description: '实时监控服务器状态',
  announcement: '',
  custom_footer: '',
  default_history_range: '1h',
}

/** 有效历史范围值（与后端 settings.go 的 validHistoryRanges 保持一致） */
const VALID_HISTORY_RANGES = ['1h', '6h', '12h', '1d', '2d', '3d']

/** 站点设置 Store：站点标题/描述/公告/自定义页脚/默认历史范围（来自后台"站点设置"） */
interface SiteSettingsStoreState {
  siteTitle: string
  siteDescription: string
  announcement: string
  customFooter: string
  /** 详情页默认历史范围（后台可配置） */
  defaultHistoryRange: string
  loaded: boolean
  fetchSettings: () => Promise<void>
  /** 强制重新拉取（后台保存设置后调用，使站点标题/公告立即生效） */
  refresh: () => Promise<void>
}

export const useSiteSettingsStore = create<SiteSettingsStoreState>((set, get) => ({
  siteTitle: DEFAULT_SITE_SETTINGS.site_title,
  siteDescription: DEFAULT_SITE_SETTINGS.site_description,
  announcement: '',
  customFooter: '',
  defaultHistoryRange: DEFAULT_SITE_SETTINGS.default_history_range,
  loaded: false,

  fetchSettings: async () => {
    if (get().loaded) return
    try {
      const s = await getPublicSettings()
      const range = VALID_HISTORY_RANGES.includes(s.default_history_range)
        ? s.default_history_range
        : DEFAULT_SITE_SETTINGS.default_history_range
      set({
        siteTitle: s.site_title || DEFAULT_SITE_SETTINGS.site_title,
        siteDescription: s.site_description || DEFAULT_SITE_SETTINGS.site_description,
        announcement: s.announcement || '',
        customFooter: s.custom_footer || '',
        defaultHistoryRange: range,
        loaded: true,
      })
    } catch {
      // 公开设置拉取失败时保持默认值，不标记 loaded 以便重试
    }
  },

  refresh: async () => {
    set({ loaded: false })
    await get().fetchSettings()
  },
}))

/** 站点设置 hook：未加载过时自动拉取一次，并同步 document.title */
export function useSiteSettings() {
  const siteTitle = useSiteSettingsStore((s) => s.siteTitle)
  const siteDescription = useSiteSettingsStore((s) => s.siteDescription)
  const announcement = useSiteSettingsStore((s) => s.announcement)
  const customFooter = useSiteSettingsStore((s) => s.customFooter)
  const defaultHistoryRange = useSiteSettingsStore((s) => s.defaultHistoryRange)
  const loaded = useSiteSettingsStore((s) => s.loaded)
  const fetchSettings = useSiteSettingsStore((s) => s.fetchSettings)

  useEffect(() => {
    if (!loaded) {
      void fetchSettings()
    }
  }, [loaded, fetchSettings])

  // 站点标题同步到浏览器标签页
  useEffect(() => {
    document.title = siteTitle
  }, [siteTitle])

  return { siteTitle, siteDescription, announcement, customFooter, defaultHistoryRange }
}
