import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, MouseEvent } from 'react'

export interface ChartSeries {
  name: string
  /** 线条颜色，如 '#5AC8FA' */
  color: string
  /** 延迟数据点，null表示缺失 */
  data: (number | null)[]
  /** 当前丢包率（0-100），可选（用于图例显示最新值） */
  loss?: number
  /** 每个时间点的丢包率（0-100），可选（用于 Tooltip 展示） */
  lossData?: (number | null)[]
}

interface NetworkQualityChartProps {
  timestamps: number[]
  series: ChartSeries[]
  height?: number
  showGrid?: boolean
  showLegend?: boolean
  timeRange?: string
}

/** Catmull-Rom 转 Bezier 平滑路径（monotone 风格，同 NodeGet/Recharts 曲线） */
function createSmoothPath(points: { x: number; y: number }[]): string {
  if (points.length < 2) return points.length === 1 ? `M ${points[0].x} ${points[0].y}` : ''
  let path = `M ${points[0].x} ${points[0].y}`
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i - 1] || points[i], p1 = points[i], p2 = points[i + 1], p3 = points[i + 2] || p2
    const cp1x = p1.x + (p2.x - p0.x) / 6, cp1y = p1.y + (p2.y - p0.y) / 6
    const cp2x = p2.x - (p3.x - p1.x) / 6, cp2y = p2.y - (p3.y - p1.y) / 6
    path += ` C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${p2.x} ${p2.y}`
  }
  return path
}

/** 将带 null 的序列切分为连续段（离线/丢包导致的空档断开绘制） */
function toSegments(values: (number | null)[], xFor: (i: number) => number, yFor: (v: number) => number) {
  const segs: { x: number; y: number }[][] = []
  let cur: { x: number; y: number }[] = []
  values.forEach((v, i) => {
    if (v === null || v === undefined || Number.isNaN(v)) { if (cur.length) segs.push(cur); cur = [] }
    else cur.push({ x: xFor(i), y: yFor(v) })
  })
  if (cur.length) segs.push(cur)
  return segs
}

/** 取友好的 Y 轴上限值 */
function niceCeil(v: number): number {
  if (v <= 0) return 10
  const exp = Math.pow(10, Math.floor(Math.log10(v))), n = v / exp
  return (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * exp
}

const fmtTime = (ts: number) => new Date(ts * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })

/** 长范围（跨度 > 48h）时 X 轴与 Tooltip 改用月-日标签，小时:分钟无区分度 */
const fmtDate = (ts: number) => new Date(ts * 1000).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
const fmtDateTime = (ts: number) =>
  `${fmtDate(ts)} ${new Date(ts * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}`

/**
 * 延迟趋势折线图（NodeGet NodeDetail 风格）
 * 单区平滑折线 + 悬浮十字线 Tooltip + 图例切换；丢包率在 Tooltip 与统计表中展示
 * 所有颜色使用 CSS 变量，自动跟随深色/浅色主题
 */
export default function NetworkQualityChart({ timestamps, series, height = 280, showGrid = true, showLegend = true, timeRange }: NetworkQualityChartProps) {
  const uid = useId()
  const wrapRef = useRef<HTMLDivElement>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const rafRef = useRef(0)
  const [containerW, setContainerW] = useState(640)
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)
  const [hoverX, setHoverX] = useState(0)
  const [hiddenSeries, setHiddenSeries] = useState<Set<string>>(() => new Set())

  // 监听容器宽度以实现响应式
  useLayoutEffect(() => {
    const el = wrapRef.current
    if (!el) return
    const update = () => setContainerW(el.clientWidth)
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const n = timestamps.length
  // 长范围（≥7d 小时聚合数据）切换为月-日标签
  const longSpan = n > 1 && timestamps[n - 1] - timestamps[0] > 48 * 3600
  const labelFor = (ts: number) => (longSpan ? fmtDate(ts) : fmtTime(ts))
  const tipLabelFor = (ts: number) => (longSpan ? fmtDateTime(ts) : fmtTime(ts))
  const padL = 46, padR = 16, padT = 14, padB = 24, pointSpacing = 10
  const chartWidth = Math.max(containerW, (n > 1 ? (n - 1) * pointSpacing : 0) + padL + padR)
  const innerW = Math.max(0, chartWidth - padL - padR)
  const innerH = Math.max(0, height - padT - padB)

  // Y 轴（仅统计未隐藏系列，隐藏后自动缩放）
  const yMax = useMemo(() => {
    let m = 0
    for (const s of series) {
      if (hiddenSeries.has(s.name)) continue
      for (const v of s.data) if (v !== null && v !== undefined && v > m) m = v
    }
    return niceCeil(m || 10)
  }, [series, hiddenSeries])

  const xFor = (i: number) => (n <= 1 ? padL : padL + (i / (n - 1)) * innerW)
  const yFor = (v: number) => padT + innerH - (v / yMax) * innerH
  const bottomY = padT + innerH
  const yTicks = [0, 0.25, 0.5, 0.75, 1].map((t) => ({ v: t * yMax, y: yFor(t * yMax) }))
  const xStep = Math.max(1, Math.ceil(n / Math.max(2, Math.floor(innerW / 70))))

  const seriesRender = useMemo(() => series.map((s, idx) => {
    const segs = toSegments(s.data, xFor, yFor)
    const lines = segs.map(createSmoothPath)
    return { name: s.name, color: s.color, data: s.data, lines, key: `nqc-${uid.replace(/[^a-zA-Z0-9]/g, '')}-${idx}` }
  }), [series, yMax, chartWidth, n, innerW, innerH, uid])

  const handleMove = (e: MouseEvent<SVGRectElement>) => {
    const svg = svgRef.current
    if (!svg || n === 0 || innerW <= 0) return
    const clientX = e.clientX
    if (rafRef.current) cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(() => {
      const rect = svg.getBoundingClientRect()
      const sx = (clientX - rect.left) * (chartWidth / rect.width)
      const idx = n <= 1 ? 0 : Math.round(((sx - padL) / innerW) * (n - 1))
      setHoverIdx(Math.max(0, Math.min(n - 1, idx)))
      if (wrapRef.current) {
        const wrapRect = wrapRef.current.getBoundingClientRect()
        setHoverX(clientX - wrapRect.left + wrapRef.current.scrollLeft)
      }
    })
  }

  useEffect(() => () => { if (rafRef.current) cancelAnimationFrame(rafRef.current) }, [])

  // 数据点数量变化时 clamp hoverIdx，而非重置为 null（避免 WS 推送时 tooltip 闪烁消失）
  useEffect(() => {
    setHoverIdx((prev) => {
      if (prev === null) return prev
      if (n === 0) return null
      return prev >= n ? n - 1 : prev
    })
  }, [n])

  const toggleSeries = (name: string) => {
    setHiddenSeries((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const hoverTs = hoverIdx !== null ? timestamps[hoverIdx] : null
  // Tooltip clamp：考虑滚动位置，确保 tooltip 始终在可见区域内
  const scrollLeft = wrapRef.current?.scrollLeft || 0
  const tipLeft = Math.min(Math.max(hoverX, scrollLeft + 70), Math.max(scrollLeft + 70, scrollLeft + containerW - 70))
  const visibleSeries = series.filter((s) => !hiddenSeries.has(s.name))

  // 空数据占位
  if (n === 0) {
    return (
      <div className="w-full">
        {(showLegend || timeRange) && (
          <div className="mb-2 flex flex-wrap items-center gap-3">
            {showLegend && series.map((s) => (
              <div key={s.name} className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ backgroundColor: s.color }} />
                <span>{s.name}</span>
              </div>
            ))}
            {timeRange && <span className="ml-auto text-xs text-muted-foreground/60">{timeRange}</span>}
          </div>
        )}
        <div className="flex items-center justify-center text-sm text-muted-foreground" style={{ height }}>
          暂无数据
        </div>
      </div>
    )
  }

  return (
    <div className="w-full">
      {/* 图例（可点击切换显隐）+ 时间范围 */}
      {(showLegend || timeRange) && (
        <div className="mb-2 flex flex-wrap items-center gap-3">
          {showLegend && series.map((s) => {
            const hidden = hiddenSeries.has(s.name)
            const loss = s.loss
            const hasLoss = loss !== undefined && loss > 0
            return (
              <div
                key={s.name}
                role="button"
                tabIndex={0}
                title="点击切换显示/隐藏"
                onClick={() => toggleSeries(s.name)}
                onKeyDown={(e: KeyboardEvent<HTMLDivElement>) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleSeries(s.name) } }}
                className="flex cursor-pointer select-none items-center gap-1.5 text-xs transition-opacity hover:opacity-80"
                style={{ opacity: hidden ? 0.4 : 1 }}
              >
                <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ backgroundColor: hidden ? 'hsl(var(--muted-foreground))' : s.color }} />
                <span className={hidden ? 'text-muted-foreground line-through' : 'text-muted-foreground'}>{s.name}</span>
                {loss !== undefined && (
                  <span className={hasLoss ? 'text-warning font-medium' : 'text-muted-foreground/60'}>{loss.toFixed(1)}%</span>
                )}
              </div>
            )
          })}
          {timeRange && <span className="ml-auto text-xs text-muted-foreground/60">{timeRange}</span>}
        </div>
      )}

      <div ref={wrapRef} className="relative w-full overflow-x-auto">
        <svg ref={svgRef} width={chartWidth} height={height} viewBox={`0 0 ${chartWidth} ${height}`} preserveAspectRatio="xMidYMid meet">
          {/* Y 轴网格线与刻度（Recharts 风格：纯数字刻度，无轴标题） */}
          {showGrid && yTicks.map((t, i) => (
            <line key={`g${i}`} x1={padL} y1={t.y} x2={chartWidth - padR} y2={t.y} stroke="hsl(var(--border))" strokeWidth={1} strokeDasharray="3 3" />
          ))}
          {yTicks.map((t, i) => (
            <text key={`y${i}`} x={padL - 6} y={t.y + 3} textAnchor="end" fontSize={10} fill="hsl(var(--muted-foreground))">{Math.round(t.v)}</text>
          ))}

          {/* 平滑折线（NodeGet LineChart 风格：纯线条，数据空档处断开） */}
          {seriesRender.filter((s) => !hiddenSeries.has(s.name)).map((s) => (
            <g key={s.key}>
              {s.lines.map((d, i) => (d ? <path key={`l${i}`} d={d} fill="none" stroke={s.color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" /> : null))}
            </g>
          ))}

          {/* X 轴时间标签 */}
          {timestamps.map((ts, i) => i % xStep === 0 ? (
            <text key={`x${i}`} x={xFor(i)} y={bottomY + 14} textAnchor="middle" fontSize={10} fill="hsl(var(--muted-foreground))">{labelFor(ts)}</text>
          ) : null)}

          {/* 悬浮十字线 + 数据点 */}
          {hoverIdx !== null && (
            <>
              <line x1={xFor(hoverIdx)} y1={padT} x2={xFor(hoverIdx)} y2={bottomY} stroke="hsl(var(--muted-foreground) / 0.3)" strokeWidth={1} strokeDasharray="3 3" />
              {seriesRender.filter((s) => !hiddenSeries.has(s.name)).map((s) => {
                const v = s.data[hoverIdx]
                if (v === null || v === undefined || Number.isNaN(v)) return null
                return <circle key={`hp-${s.key}`} cx={xFor(hoverIdx)} cy={yFor(v)} r={3.5} fill={s.color} stroke="hsl(var(--card))" strokeWidth={1.5} />
              })}
            </>
          )}

          {/* 鼠标交互区域 */}
          <rect x={padL} y={padT} width={Math.max(0, innerW)} height={Math.max(0, bottomY - padT)} fill="transparent" onMouseMove={handleMove} onMouseLeave={() => { if (rafRef.current) cancelAnimationFrame(rafRef.current); setHoverIdx(null) }} />
        </svg>

        {/* Tooltip — 时间 + 各目标延迟/丢包（Tailwind CSS 类自动跟随主题） */}
        {hoverIdx !== null && hoverTs != null && (
          <div className="pointer-events-none absolute top-1 z-10 rounded-md border border-border bg-card px-2.5 py-1.5 text-xs shadow-lg" style={{ left: tipLeft, transform: 'translateX(-50%)' }}>
            <div className="mb-1 font-medium text-foreground">{tipLabelFor(hoverTs)}</div>
            {visibleSeries.map((s) => {
              const v = s.data[hoverIdx]
              const lossV = s.lossData ? s.lossData[hoverIdx] : s.loss
              const hasLoss = lossV !== undefined && lossV !== null && lossV > 0
              return (
                <div key={s.name} className="flex items-center gap-1.5 whitespace-nowrap py-0.5">
                  <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: s.color }} />
                  <span className="text-muted-foreground">{s.name}</span>
                  <span className="ml-auto flex items-center gap-2 pl-3">
                    {lossV !== undefined && lossV !== null && (
                      <span className={hasLoss ? 'text-warning font-medium' : 'text-muted-foreground/60'}>丢包 {lossV.toFixed(1)}%</span>
                    )}
                    <span className="font-medium text-foreground">{v != null ? `${v.toFixed(1)} ms` : '-'}</span>
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
