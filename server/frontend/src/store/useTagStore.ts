import { useEffect } from 'react'
import { create } from 'zustand'
import type { Tag } from '@/types'
import { getPublicTags, createTag, updateTag, deleteTag } from '@/lib/api'

/** 标签 Store：标签列表 + 名称→颜色映射（ServerCard/Dashboard 徽章取色） */
interface TagStoreState {
  tags: Tag[]
  /** 名称 → 颜色映射（含管理端设置的色值） */
  colorMap: Record<string, string>
  loading: boolean
  loaded: boolean
  fetchTags: () => Promise<void>
  addTag: (data: { name: string; color?: string }) => Promise<Tag>
  editTag: (id: number, data: Partial<{ name: string; color: string }>) => Promise<void>
  removeTag: (id: number) => Promise<void>
}

/** 由标签列表构建颜色映射 */
function buildColorMap(tags: Tag[]): Record<string, string> {
  const map: Record<string, string> = {}
  for (const t of tags) {
    map[t.name] = t.color
  }
  return map
}

export const useTagStore = create<TagStoreState>((set, get) => ({
  tags: [],
  colorMap: {},
  loading: false,
  loaded: false,

  fetchTags: async () => {
    // 并发去重：多张卡片同批挂载时只发起一次请求；已加载过直接跳过
    if (get().loading || get().loaded) return
    set({ loading: true })
    try {
      const res = await getPublicTags()
      const tags = res.tags || []
      set({ tags, colorMap: buildColorMap(tags), loading: false, loaded: true })
    } catch {
      set({ loading: false })
    }
  },

  addTag: async (data) => {
    const res = await createTag(data)
    const tags = [...get().tags, res.tag]
    set({ tags, colorMap: buildColorMap(tags) })
    return res.tag
  },

  editTag: async (id, data) => {
    await updateTag(id, data)
    const tags = get().tags.map((t) => (t.id === id ? { ...t, ...data } : t))
    set({ tags, colorMap: buildColorMap(tags) })
  },

  removeTag: async (id) => {
    await deleteTag(id)
    const tags = get().tags.filter((t) => t.id !== id)
    set({ tags, colorMap: buildColorMap(tags) })
  },
}))

/** 获取标签颜色的 hook（未加载过时自动拉取一次） */
export function useTagColors(): Record<string, string> {
  const loaded = useTagStore((s) => s.loaded)
  const fetchTags = useTagStore((s) => s.fetchTags)
  const colorMap = useTagStore((s) => s.colorMap)

  // 惰性加载：首次使用时拉取标签列表
  useEffect(() => {
    if (!loaded && !useTagStore.getState().loading) {
      void fetchTags()
    }
  }, [loaded, fetchTags])

  return colorMap
}
