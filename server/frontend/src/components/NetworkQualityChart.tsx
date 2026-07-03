import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, MouseEvent } from 'react'

export interface ChartSeries {
  name: string
  /** 线条颜色，如 '#5AC8FA' */
  color: string
  /** 数据点，null表示缺失 */
  data: (number | null)[]
  /** 当前丢包率（0-100），可选 */
  loss?: number
}

interface NetworkQualityChartProps {
  timestamps: number[]
  series: ChartSeries[]
  height?: number
  showGrid?: boolean
  showLegend?: boolean
  timeRange?: string
}

/** Catmull-Rom 转 Bezier 平滑路径 */
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

/** 将带 null 的序列切分为连续段 */
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
const AXIS_FILL = 'rgba(255,255,255,0.45)'

/** 纯 SVG 面积折线图：用于详情页网络延迟趋势（平滑曲线 + 渐变面积 + 悬停 tooltip） */
export default function NetworkQualityChart({ timestamps, series, height = 200, showGrid = true, showLegend = true, timeRange }: NetworkQualityChartProps) {
  const uid = useId()
  const wrapRef = useRef<HTMLDivElement>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const rafRef = useRef(0)
  const [containerW, setContainerW] = useState(640)
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)
  const [hoverX, setHoverX] = useState(0)
  // 被隐藏的系列名称集合（点击图例切换）
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
  const padL = 52, padR = 16, padT = 16, padB = 38, pointSpacing = 10
  // 数据少时填满容器，数据多时横向滚动
  const chartWidth = Math.max(containerW, (n > 1 ? (n - 1) * pointSpacing : 0) + padL + padR)
  const innerW = Math.max(0, chartWidth - padL - padR)
  const innerH = Math.max(0, height - padT - padB)

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
    const areas = segs.map((seg) => seg.length < 2 ? '' : `${createSmoothPath(seg)} L ${seg[seg.length - 1].x} ${bottomY} L ${seg[0].x} ${bottomY} Z`)
    return { name: s.name, color: s.color, data: s.data, lines, areas, gradId: `nqc-grad-${uid.replace(/[^a-zA-Z0-9]/g, '')}-${idx}` }
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
      if (wrapRef.current) setHoverX(clientX - wrapRef.current.getBoundingClientRect().left)
    })
  }

  // 清理未完成的 rAF，避免组件卸载后 setState
  useEffect(() => () => { if (rafRef.current) cancelAnimationFrame(rafRef.current) }, [])

  // 点击图例切换系列显示/隐藏
  const toggleSeries = (name: string) => {
    setHiddenSeries((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  const hoverTs = hoverIdx !== null ? timestamps[hoverIdx] : null
  // tooltip 横向定位夹在容器内，避免溢出
  const tipLeft = Math.min(Math.max(hoverX, 70), Math.max(70, containerW - 70))

  return (
    <div className="w-full">
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
                <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ backgroundColor: hidden ? '#6b7280' : s.color }} />
                <span className={hidden ? 'text-gray-500 line-through' : 'text-gray-400'}>{s.name}</span>
                {loss !== undefined && (
                  <span className={hasLoss ? 'text-amber-400' : 'text-gray-500'}>{loss.toFixed(1)}%</span>
                )}
              </div>
            )
          })}
          {timeRange && <span className="ml-auto text-xs text-gray-500">{timeRange}</span>}
        </div>
      )}
      <div ref={wrapRef} className="relative w-full overflow-x-auto">
        <svg ref={svgRef} width={chartWidth} height={height} viewBox={`0 0 ${chartWidth} ${height}`} preserveAspectRatio="xMidYMid meet">
          <defs>
            {seriesRender.map((s) => (
              <linearGradient key={s.gradId} id={s.gradId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={s.color} stopOpacity={0.3} />
                <stop offset="100%" stopColor={s.color} stopOpacity={0} />
              </linearGradient>
            ))}
          </defs>
          {/* 网格线（虚线） */}
          {showGrid && yTicks.map((t, i) => (
            <line key={`g${i}`} x1={padL} y1={t.y} x2={chartWidth - padR} y2={t.y} stroke="rgba(255,255,255,0.05)" strokeWidth={1} strokeDasharray="3 3" />
          ))}
          {/* Y 轴标题（旋转）与刻度 */}
          <text x={12} y={padT + innerH / 2} fontSize={11} fill={AXIS_FILL} textAnchor="middle" transform={`rotate(-90 12 ${padT + innerH / 2})`}>延迟 (ms)</text>
          {yTicks.map((t, i) => (
            <text key={`y${i}`} x={padL - 6} y={t.y + 3} textAnchor="end" fontSize={10} fill={AXIS_FILL}>{Math.round(t.v)}</text>
          ))}
          {/* X 轴时间标签（智能间隔） */}
          {timestamps.map((ts, i) => i % xStep === 0 ? (
            <text key={`x${i}`} x={xFor(i)} y={bottomY + 14} textAnchor="middle" fontSize={10} fill={AXIS_FILL}>{fmtTime(ts)}</text>
          ) : null)}
          {/* X 轴标题 */}
          <text x={padL + innerW / 2} y={height - 6} textAnchor="middle" fontSize={11} fill={AXIS_FILL}>时间</text>
          {/* 面积 + 折线（隐藏的系列不渲染） */}
          {seriesRender.filter((s) => !hiddenSeries.has(s.name)).map((s) => (
            <g key={s.name}>
              {s.areas.map((d, i) => (d ? <path key={`a${i}`} d={d} fill={`url(#${s.gradId})`} /> : null))}
              {s.lines.map((d, i) => (d ? <path key={`l${i}`} d={d} fill="none" stroke={s.color} strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" /> : null))}
            </g>
          ))}
          {/* 悬停垂直辅助线 + 数据点 */}
          {hoverIdx !== null && (
            <line x1={xFor(hoverIdx)} y1={padT} x2={xFor(hoverIdx)} y2={bottomY} stroke="rgba(255,255,255,0.25)" strokeWidth={1} strokeDasharray="3 3" />
          )}
          {hoverIdx !== null && seriesRender.filter((s) => !hiddenSeries.has(s.name)).map((s) => {
            const v = s.data[hoverIdx]
            if (v === null || v === undefined || Number.isNaN(v)) return null
            return <circle key={s.name} cx={xFor(hoverIdx)} cy={yFor(v)} r={3} fill={s.color} stroke="#fff" strokeWidth={1} />
          })}
          <rect x={padL} y={padT} width={Math.max(0, innerW)} height={Math.max(0, innerH)} fill="transparent" onMouseMove={handleMove} onMouseLeave={() => setHoverIdx(null)} />
        </svg>
        {/* Tooltip */}
        {hoverIdx !== null && hoverTs !== null && (
          <div className="pointer-events-none absolute top-1 z-10 rounded-md border border-white/10 bg-gray-900/95 px-2.5 py-1.5 text-xs text-gray-200 shadow-lg" style={{ left: tipLeft, transform: 'translateX(-50%)' }}>
            <div className="mb-1 font-medium text-gray-300">{fmtTime(hoverTs)}</div>
            {series.filter((s) => !hiddenSeries.has(s.name)).map((s) => {
              const v = s.data[hoverIdx]
              const loss = s.loss
              const hasLoss = loss !== undefined && loss > 0
              return (
                <div key={s.name} className="flex items-center gap-1.5 whitespace-nowrap">
                  <span className="inline-block h-2 w-2 rounded-full" style={{ backgroundColor: s.color }} />
                  <span className="text-gray-400">{s.name}</span>
                  <span className="ml-auto flex items-center gap-2 pl-2">
                    {loss !== undefined && <span className={hasLoss ? 'text-amber-400' : 'text-gray-500'}>丢包 {loss.toFixed(1)}%</span>}
                    <span className="font-medium">{v != null ? `${v.toFixed(1)} ms` : '-'}</span>
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
