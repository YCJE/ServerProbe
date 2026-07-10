import { memo, useEffect, useMemo, useState } from 'react'
import type { PingResult } from '@/types'
import { parsePingData } from '@/lib/utils'

/** 单个时间点的延迟数据 */
export interface LatencyPoint {
  /** 时间戳（秒） */
  timestamp: number
  /** Ping 探测结果 */
  ping_data: PingResult[] | unknown
}

interface LatencyQualityBarProps {
  /** 历史数据点列表 */
  points: LatencyPoint[]
  /** 总时间范围（秒），默认 3600（1 小时） */
  timeRangeSeconds?: number
  /** 每个桶的时间跨度（秒），默认 60（每分钟一个桶） */
  bucketSeconds?: number
  /** 移动端桶数量，默认 30 */
  mobileBuckets?: number
  /** 自定义类名 */
  className?: string
}

/** 桶颜色配置（NodeGet 色系） */
const BUCKET_COLORS = {
  green: '#34C759',        // <=50ms
  lightGreen: '#5AC8FA',   // 50-100ms
  yellow: '#FFCC00',       // 100-180ms
  orange: '#FF9500',       // 180-300ms
  red: '#FF3B30',          // >300ms
  packetLoss: '#8B0000',   // 丢包深红
  empty: 'hsl(var(--muted) / 0.4)', // 无数据
} as const

/** 根据平均延迟和丢包率返回桶颜色 */
function getBucketColor(avgLatency: number, avgLoss: number, hasData: boolean): string {
  // 无数据时返回空颜色
  if (!hasData) return BUCKET_COLORS.empty
  // 丢包率 > 50% 视为严重丢包
  if (avgLoss > 50) return BUCKET_COLORS.packetLoss
  // 有数据时 avgLatency=0 归入绿色
  if (avgLatency <= 50) return BUCKET_COLORS.green
  if (avgLatency <= 100) return BUCKET_COLORS.lightGreen
  if (avgLatency <= 180) return BUCKET_COLORS.yellow
  if (avgLatency <= 300) return BUCKET_COLORS.orange
  return BUCKET_COLORS.red
}

/** 时间桶聚合结果 */
interface TimeBucket {
  /** 桶索引 */
  index: number
  /** 桶起始时间（秒） */
  startTime: number
  /** 桶结束时间（秒） */
  endTime: number
  /** 平均延迟（ms） */
  avgLatency: number
  /** 平均丢包率（%） */
  avgLoss: number
  /** 数据点数量 */
  count: number
  /** 颜色 */
  color: string
  /** 是否有数据 */
  hasData: boolean
}

/** 格式化时间为 HH:MM */
function formatTime(ts: number): string {
  return new Date(ts * 1000).toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  })
}

/**
 * 延迟质量条形图组件（NodeGet 风格）
 *
 * - 接收最近 1 小时的 PingResult 历史数据
 * - 按时间桶聚合（每分钟一个桶，共 60 个桶）
 * - 虚线边框容器，标题使用 primary 色
 * - 每个桶根据平均延迟着色：<=50ms 绿、50-100ms 浅绿、100-180ms 黄、180-300ms 橙、>300ms 红、丢包深红
 * - 悬停显示该时间段的详细信息（时间范围、平均延迟、丢包率）
 * - 响应式：移动端 30 桶
 */
function LatencyQualityBar({
  points,
  timeRangeSeconds = 3600,
  bucketSeconds = 60,
  mobileBuckets = 30,
  className = '',
}: LatencyQualityBarProps) {
  const [hoveredBucket, setHoveredBucket] = useState<{ view: 'desktop' | 'mobile'; index: number } | null>(null)

  // 每 60 秒刷新一次 "now" 引用，防止长时间挂载后时间范围漂移
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000))
  useEffect(() => {
    const timer = setInterval(() => setNowTick(Math.floor(Date.now() / 1000)), 60000)
    return () => clearInterval(timer)
  }, [])

  // 是否移动端视图（通过 CSS 媒体查询判断桶数量）
  // 我们渲染两组桶：桌面 60 个，移动 30 个，通过 CSS 控制显示
  const desktopBucketCount = Math.floor(timeRangeSeconds / bucketSeconds)
  const mobileBucketCount = mobileBuckets

  // 计算桶数据
  const buckets = useMemo<TimeBucket[]>(() => {
    if (!points || points.length === 0) return []

    const now = nowTick
    const startTime = now - timeRangeSeconds
    const result: TimeBucket[] = []

    // 初始化所有桶
    for (let i = 0; i < desktopBucketCount; i++) {
      const bucketStart = startTime + i * bucketSeconds
      const bucketEnd = bucketStart + bucketSeconds
      result.push({
        index: i,
        startTime: bucketStart,
        endTime: bucketEnd,
        avgLatency: 0,
        avgLoss: 0,
        count: 0,
        color: BUCKET_COLORS.empty,
        hasData: false,
      })
    }

    // 将数据点分配到桶中
    for (const point of points) {
      const ts = point.timestamp
      if (ts < startTime || ts > now) continue

      const bucketIdx = Math.floor((ts - startTime) / bucketSeconds)
      if (bucketIdx < 0 || bucketIdx >= desktopBucketCount) continue

      const bucket = result[bucketIdx]
      const pings = parsePingData(point.ping_data)
      if (pings.length === 0) continue

      // 计算该数据点的平均延迟和丢包率
      let latencySum = 0
      let lossSum = 0
      let validCount = 0
      for (const ping of pings) {
        if (ping.avg_latency != null && ping.avg_latency >= 0) {
          latencySum += ping.avg_latency
          lossSum += ping.loss || 0
          validCount++
        }
      }

      if (validCount > 0) {
        // 累加到桶中（用于计算加权平均）
        const pointAvgLatency = latencySum / validCount
        const pointAvgLoss = lossSum / validCount

        // 简单平均：将每个数据点的平均值累加，最后除以数据点数
        bucket.avgLatency = (bucket.avgLatency * bucket.count + pointAvgLatency) / (bucket.count + 1)
        bucket.avgLoss = (bucket.avgLoss * bucket.count + pointAvgLoss) / (bucket.count + 1)
        bucket.count++
        bucket.hasData = true
      }
    }

    // 计算每个桶的颜色
    for (const bucket of result) {
      if (bucket.hasData) {
        bucket.color = getBucketColor(bucket.avgLatency, bucket.avgLoss, bucket.hasData)
      }
    }

    return result
  }, [points, timeRangeSeconds, bucketSeconds, desktopBucketCount, nowTick])

  // 移动端桶聚合
  const mobileBucketsData = useMemo<TimeBucket[]>(() => {
    if (buckets.length === 0) return []

    const result: TimeBucket[] = []
    const bucketsPerMobile = desktopBucketCount / mobileBucketCount

    for (let i = 0; i < mobileBucketCount; i++) {
      const startIdx = Math.floor(i * bucketsPerMobile)
      const endIdx = Math.floor((i + 1) * bucketsPerMobile)
      const subBuckets = buckets.slice(startIdx, endIdx)

      const dataBuckets = subBuckets.filter((b) => b.hasData)
      if (dataBuckets.length === 0) {
        result.push({
          index: i,
          startTime: subBuckets[0]?.startTime || 0,
          endTime: subBuckets[subBuckets.length - 1]?.endTime || 0,
          avgLatency: 0,
          avgLoss: 0,
          count: 0,
          color: BUCKET_COLORS.empty,
          hasData: false,
        })
      } else {
        // 加权平均
        const totalCount = dataBuckets.reduce((sum, b) => sum + b.count, 0)
        const weightedLatency = dataBuckets.reduce((sum, b) => sum + b.avgLatency * b.count, 0) / totalCount
        const weightedLoss = dataBuckets.reduce((sum, b) => sum + b.avgLoss * b.count, 0) / totalCount
        result.push({
          index: i,
          startTime: subBuckets[0].startTime,
          endTime: subBuckets[subBuckets.length - 1].endTime,
          avgLatency: weightedLatency,
          avgLoss: weightedLoss,
          count: totalCount,
          color: getBucketColor(weightedLatency, weightedLoss, true),
          hasData: true,
        })
      }
    }

    return result
  }, [buckets, desktopBucketCount, mobileBucketCount])

  // 渲染桶列表
  const renderBuckets = (bucketList: TimeBucket[], isMobile: boolean) => {
    const view = isMobile ? 'mobile' : 'desktop'
    return bucketList.map((bucket) => {
      const isHovered = hoveredBucket !== null && hoveredBucket.view === view && hoveredBucket.index === bucket.index
      return (
        <div
          key={`${isMobile ? 'm' : 'd'}-${bucket.index}`}
          className="relative flex-1 cursor-pointer rounded-[3px] transition-all"
          style={{
            height: isMobile ? 28 : 36,
            backgroundColor: bucket.color,
            opacity: hoveredBucket !== null && !isHovered ? 0.5 : 1,
            transform: isHovered ? 'scaleY(1.1)' : 'scaleY(1)',
            transition: 'opacity 0.15s, transform 0.15s',
          }}
          onMouseEnter={() => setHoveredBucket({ view, index: bucket.index })}
          onMouseLeave={() => setHoveredBucket(null)}
        />
      )
    })
  }

  // 当前悬停的桶信息（根据 view 选择对应数据源）
  const hoveredBucketData = useMemo(() => {
    if (hoveredBucket === null) return null
    const source = hoveredBucket.view === 'desktop' ? buckets : mobileBucketsData
    return source[hoveredBucket.index] || null
  }, [hoveredBucket, buckets, mobileBucketsData])

  if (buckets.length === 0) {
    return (
      <div className={`flex items-center justify-center rounded-md border-dashed border border-border/80 p-4 text-sm text-muted-foreground ${className}`}>
        暂无延迟历史数据
      </div>
    )
  }

  return (
    <div className={`relative rounded-md border-dashed border border-border/80 p-4 ${className}`}>
      {/* 标题 + 图例 */}
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-bold text-primary">延迟质量分布</span>
        <div className="flex flex-wrap items-center gap-2 text-[10px] text-muted-foreground">
          <LegendItem color={BUCKET_COLORS.green} label="≤50ms" />
          <LegendItem color={BUCKET_COLORS.lightGreen} label="50-100" />
          <LegendItem color={BUCKET_COLORS.yellow} label="100-180" />
          <LegendItem color={BUCKET_COLORS.orange} label="180-300" />
          <LegendItem color={BUCKET_COLORS.red} label=">300" />
          <LegendItem color={BUCKET_COLORS.packetLoss} label="丢包" />
        </div>
      </div>

      {/* 桶容器 - 桌面端 60 桶 */}
      <div className="hidden sm:block">
        <div className="flex items-end gap-0.5">
          {renderBuckets(buckets, false)}
        </div>
      </div>

      {/* 桶容器 - 移动端 30 桶 */}
      <div className="sm:hidden">
        <div className="flex items-end gap-0.5">
          {renderBuckets(mobileBucketsData, true)}
        </div>
      </div>

      {/* 时间轴标签 */}
      <div className="mt-1.5 flex justify-between text-[10px] text-muted-foreground">
        <span>{formatTime(buckets[0]?.startTime || 0)}</span>
        <span>{formatTime(buckets[Math.floor(desktopBucketCount / 2)]?.startTime || 0)}</span>
        <span>{formatTime(buckets[desktopBucketCount - 1]?.endTime || 0)}</span>
      </div>

      {/* 悬停 Tooltip */}
      {hoveredBucketData && (
        <div className="pointer-events-none absolute -top-2 left-1/2 z-10 -translate-x-1/2 -translate-y-full rounded-sm border border-border bg-card px-3 py-2.5 text-xs shadow-tooltip ring-1 ring-black/5">
          <div className="whitespace-nowrap font-medium text-foreground">
            {formatTime(hoveredBucketData.startTime)} - {formatTime(hoveredBucketData.endTime)}
          </div>
          {hoveredBucketData.hasData ? (
            <>
              <div className="mt-1 whitespace-nowrap text-muted-foreground">
                平均延迟: <span className="font-medium text-foreground">{hoveredBucketData.avgLatency.toFixed(1)} ms</span>
              </div>
              <div className="whitespace-nowrap text-muted-foreground">
                丢包率: <span className="font-medium text-foreground">{hoveredBucketData.avgLoss.toFixed(1)}%</span>
              </div>
              <div className="whitespace-nowrap text-muted-foreground">
                采样数: <span className="font-medium text-foreground">{hoveredBucketData.count}</span>
              </div>
            </>
          ) : (
            <div className="mt-1 whitespace-nowrap text-muted-foreground">无数据</div>
          )}
        </div>
      )}
    </div>
  )
}

/** 图例项 */
function LegendItem({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1">
      <span
        className="inline-block h-2 w-2 rounded-sm"
        style={{ backgroundColor: color }}
      />
      <span>{label}</span>
    </span>
  )
}

export default memo(LatencyQualityBar)
