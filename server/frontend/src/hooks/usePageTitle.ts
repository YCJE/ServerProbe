import { useEffect } from 'react'
import { setRoutePageTitle } from '@/store/useSiteSettingsStore'

/**
 * 路由级页面标题：document.title = `${pageName} - ${siteTitle}`
 * 站点标题变化时自动重新拼接；离开页面时还原为纯站点标题
 */
export function usePageTitle(pageName: string) {
  useEffect(() => {
    setRoutePageTitle(pageName)
    return () => setRoutePageTitle('')
  }, [pageName])
}
