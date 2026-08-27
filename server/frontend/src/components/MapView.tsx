import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import ReactECharts from 'echarts-for-react'
import * as echarts from 'echarts'
import { feature } from 'topojson-client'
import worldTopo from 'world-atlas/countries-110m.json'
import type { ServerData } from '@/types'
import { getCountryCode, getFlagEmoji } from '@/lib/utils'
import { resolveServerCoord } from '@/lib/countryCoords'

/** 世界地图 GeoJSON 注册（模块级一次执行，内嵌资源不走 CDN） */
const worldGeoJSON = feature(
  worldTopo as Parameters<typeof feature>[0],
  (worldTopo as { objects: { countries: never } }).objects.countries,
)
echarts.registerMap('world', worldGeoJSON as unknown as Parameters<typeof echarts.registerMap>[1])

interface MapViewProps {
  servers: ServerData[]
  /** 详情页链接基础路径（"/admin" 或 ""） */
  basePath: string
}

/** 同一位置聚合的光点 */
interface MapPoint {
  key: string
  name: string
  coord: [number, number]
  servers: ServerData[]
  allOnline: boolean
}

/** 地图视图（ECharts 世界地图 + effectScatter 光点，NodeGet 风格） */
export default function MapView({ servers, basePath }: MapViewProps) {
  const navigate = useNavigate()
  // 当前选中的位置（点击光点展开服务器列表）
  const [selectedKey, setSelectedKey] = useState<string | null>(null)

  // 按坐标聚合服务器（同位置多台合并为一个光点）
  const points = useMemo<MapPoint[]>(() => {
    const map = new Map<string, MapPoint>()
    for (const server of servers) {
      const cc = getCountryCode(server)
      const coord = resolveServerCoord(server.region, cc)
      if (!coord) continue
      const key = `${coord[0].toFixed(1)},${coord[1].toFixed(1)}`
      const name = server.region || cc
      const existing = map.get(key)
      if (existing) {
        existing.servers.push(server)
        existing.allOnline = existing.allOnline && server.online
        // 位置名取更具体的（region 优先于国家代码）
        if (server.region && !existing.name.match(/[a-z]/i)) existing.name = server.region
      } else {
        map.set(key, {
          key,
          name,
          coord,
          servers: [server],
          allOnline: server.online,
        })
      }
    }
    return [...map.values()].sort((a, b) => b.servers.length - a.servers.length)
  }, [servers])

  const selectedPoint = selectedKey ? points.find((p) => p.key === selectedKey) : null

  const option = useMemo(
    () => ({
      backgroundColor: 'transparent',
      tooltip: {
        trigger: 'item',
        backgroundColor: 'rgba(30, 30, 36, 0.95)',
        borderColor: 'rgba(255,255,255,0.1)',
        textStyle: { color: '#e5e7eb', fontSize: 12 },
        formatter: (params: { data?: { name?: string; servers?: ServerData[] } }) => {
          const data = params.data
          if (!data?.servers) return ''
          const list = data.servers
            .slice(0, 8)
            .map(
              (s) =>
                `${s.online ? '🟢' : '⚪'} ${s.display_name || s.hostname} · CPU ${(
                  s.cpu || 0
                ).toFixed(0)}%`,
            )
            .join('<br/>')
          const more =
            data.servers.length > 8 ? `<br/>... 共 ${data.servers.length} 台` : ''
          return `<strong>${data.name}（${data.servers.length} 台）</strong><br/>${list}${more}`
        },
      },
      geo: {
        map: 'world',
        roam: true,
        zoom: 1.2,
        scaleLimit: { min: 1, max: 8 },
        itemStyle: {
          areaColor: 'rgba(100, 116, 139, 0.18)',
          borderColor: 'rgba(148, 163, 184, 0.35)',
          borderWidth: 0.5,
        },
        emphasis: {
          disabled: true,
        },
        select: {
          disabled: true,
        },
      },
      series: [
        {
          type: 'effectScatter',
          coordinateSystem: 'geo',
          data: points.map((p) => ({
            name: p.name,
            value: [p.coord[0], p.coord[1], p.servers.length],
            servers: p.servers,
            pointKey: p.key,
            itemStyle: {
              color: p.allOnline ? '#4ade80' : '#9ca3af',
              shadowBlur: 8,
              shadowColor: p.allOnline ? 'rgba(74, 222, 128, 0.5)' : 'rgba(156, 163, 175, 0.4)',
            },
          })),
          symbolSize: (val: number[]) => Math.min(10 + (val[2] || 1) * 4, 26),
          rippleEffect: {
            brushType: 'stroke',
            scale: 2.6,
            period: 4,
          },
          encode: { value: 2 },
          label: { show: false },
          zlevel: 1,
        },
      ],
    }),
    [points],
  )

  const handleChartClick = (params: { data?: { pointKey?: string } }) => {
    const key = params.data?.pointKey
    setSelectedKey((prev) => (prev === key ? null : key ?? null))
  }

  return (
    <div className="space-y-4">
      <div className="card-soft overflow-hidden p-2">
        <ReactECharts
          option={option}
          style={{ height: 480 }}
          onEvents={{ click: handleChartClick }}
          opts={{ renderer: 'canvas' }}
        />
      </div>

      {/* 选中位置的节点列表 */}
      {selectedPoint && (
        <div className="card-soft p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="text-sm font-bold text-foreground">
              {selectedPoint.name}（{selectedPoint.servers.length} 台）
            </h3>
            <button
              onClick={() => setSelectedKey(null)}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              收起
            </button>
          </div>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {selectedPoint.servers.map((s) => {
              const cc = getCountryCode(s)
              return (
                <button
                  key={s.id}
                  onClick={() => navigate(`${basePath}/server/${s.id}`)}
                  className="flex items-center gap-2 rounded-md border border-border bg-card/90 px-3 py-2 text-left transition-colors hover:bg-accent"
                >
                  <span className="text-sm leading-none">{cc ? getFlagEmoji(cc) : ''}</span>
                  <span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                    {s.display_name || s.hostname}
                  </span>
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                    CPU {(s.cpu || 0).toFixed(0)}%
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      )}

      {/* 无法定位的提示 */}
      {points.length === 0 && servers.length > 0 && (
        <div className="card-soft flex flex-col items-center justify-center py-10">
          <p className="text-sm font-medium text-foreground">暂无可定位的服务器</p>
          <p className="mt-1 text-xs text-muted-foreground">
            请在 Agent 管理页设置国家代码（country_code）或位置（region）后再查看地图
          </p>
        </div>
      )}
    </div>
  )
}
