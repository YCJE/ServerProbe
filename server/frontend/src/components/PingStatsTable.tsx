import { getLossColor } from '@/lib/utils'

/** 延迟统计表行（NodeGet 风格：来源名 / 平均 / 最低 / 最高 / 抖动 / 丢包率） */
export interface PingStatRow {
  name: string
  color: string
  samples: number
  avg: number | null
  min: number | null
  max: number | null
  jitter: number | null
  loss: number | null
}

/**
 * 延迟统计表（NodeGet NodeDetail 风格）
 *
 * 图表下方的目标统计表格：每个探测目标在所选时间范围内的
 * 平均/最低/最高延迟、抖动、丢包率与样本数。
 */
export default function PingStatsTable({ stats }: { stats: PingStatRow[] }) {
  if (stats.length === 0) return null

  const fmtLatency = (v: number | null) =>
    v !== null ? `${v.toFixed(1)} ms` : '---'

  return (
    <div className="mt-4 border-t border-dashed border-border/60 pt-3">
      <h4 className="mb-2 text-xs uppercase tracking-wide text-muted-foreground">
        延迟统计
      </h4>
      <div className="overflow-x-auto scrollbar-thin">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="py-2 pl-1 pr-4 text-left text-xs font-medium text-muted-foreground">目标</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-muted-foreground">平均延迟</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-muted-foreground">最低</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-muted-foreground">最高</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-muted-foreground">抖动</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-muted-foreground">丢包率</th>
              <th className="py-2 pl-4 pr-1 text-right text-xs font-medium text-muted-foreground">样本数</th>
            </tr>
          </thead>
          <tbody>
            {stats.map((row) => (
              <tr key={row.name} className="border-b border-border/40 last:border-0">
                <td className="py-2 pl-1 pr-4">
                  <span className="flex items-center gap-2">
                    <span
                      className="h-2 w-2 shrink-0 rounded-full"
                      style={{ backgroundColor: row.color }}
                    />
                    <span className="max-w-[180px] truncate text-foreground">{row.name}</span>
                  </span>
                </td>
                <td className="px-4 py-2 text-right font-bold tabular-nums text-foreground">
                  {fmtLatency(row.avg)}
                </td>
                <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                  {fmtLatency(row.min)}
                </td>
                <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                  {fmtLatency(row.max)}
                </td>
                <td className="px-4 py-2 text-right tabular-nums text-muted-foreground">
                  {fmtLatency(row.jitter)}
                </td>
                <td
                  className={`px-4 py-2 text-right font-bold tabular-nums ${getLossColor(row.loss ?? -1)}`}
                >
                  {row.loss !== null ? `${row.loss.toFixed(1)}%` : '---'}
                </td>
                <td className="py-2 pl-4 pr-1 text-right tabular-nums text-muted-foreground">
                  {row.samples}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
