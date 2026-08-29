package model

// CalcQuotaUsedBytes 按配额口径计算已用流量字节数（P2：流量配额类型）
// 告警引擎（metric_traffic_quota）与前端流量进度条百分比共用此口径，保证两侧一致。
// 未知口径按默认 sum 处理（rx+tx，与历史行为一致）。
func CalcQuotaUsedBytes(quotaType string, rx, tx uint64) uint64 {
	switch quotaType {
	case QuotaTypeUp:
		return tx
	case QuotaTypeDown:
		return rx
	case QuotaTypeMax:
		if rx > tx {
			return rx
		}
		return tx
	case QuotaTypeMin:
		if rx < tx {
			return rx
		}
		return tx
	default: // sum（含空值/未知值兜底）
		return rx + tx
	}
}
