package api

import (
	"testing"
)

func TestParseHistoryRange(t *testing.T) {
	const now = int64(1_700_000_000)

	tests := []struct {
		name     string
		input    string
		wantDiff int64  // start 相对 now 的偏移（秒）
		wantHr   bool   // 是否路由到小时聚合层
		wantItv  string // 数据粒度标识
	}{
		{"1h 默认", "1h", 3600, false, "5m"},
		{"6h", "6h", 6 * 3600, false, "5m"},
		{"12h", "12h", 12 * 3600, false, "5m"},
		{"1d", "1d", 24 * 3600, false, "5m"},
		{"2d", "2d", 2 * 24 * 3600, false, "5m"},
		{"3d", "3d", 3 * 24 * 3600, false, "5m"},
		{"7d 走小时层", "7d", 7 * 24 * 3600, true, "1h"},
		{"30d 走小时层", "30d", 30 * 24 * 3600, true, "1h"},
		{"90d 走小时层", "90d", 90 * 24 * 3600, true, "1h"},
		{"1y 走小时层", "1y", 365 * 24 * 3600, true, "1h"},
		{"未知值回退 1h", "garbage", 3600, false, "5m"},
		{"空值回退 1h", "", 3600, false, "5m"},
		{"realtime 回退 1h", "realtime", 3600, false, "5m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := parseHistoryRange(tt.input, now)
			if got := now - spec.start; got != tt.wantDiff {
				t.Errorf("range=%q start 偏移 = %d, want %d", tt.input, got, tt.wantDiff)
			}
			if spec.hourly != tt.wantHr {
				t.Errorf("range=%q hourly = %v, want %v", tt.input, spec.hourly, tt.wantHr)
			}
			if spec.interval != tt.wantItv {
				t.Errorf("range=%q interval = %q, want %q", tt.input, spec.interval, tt.wantItv)
			}
		})
	}
}

func TestDownsampleHistory(t *testing.T) {
	makeRecords := func(n int) []int {
		recs := make([]int, n)
		for i := range recs {
			recs[i] = i
		}
		return recs
	}

	t.Run("点数不超过上限时原样返回", func(t *testing.T) {
		records := makeRecords(50)
		got := downsampleHistory(records, 100)
		if len(got) != 50 {
			t.Errorf("len = %d, want 50", len(got))
		}
	})

	t.Run("抽稀保留首尾点且不超上限", func(t *testing.T) {
		const maxPoints = 100
		records := makeRecords(1000)
		got := downsampleHistory(records, maxPoints)
		if len(got) > maxPoints+1 {
			t.Errorf("len = %d, 超过上限 %d", len(got), maxPoints+1)
		}
		if len(got) == 0 || got[0] != 0 {
			t.Errorf("首点丢失: got[0] = %v", got[0])
		}
		if got[len(got)-1] != 999 {
			t.Errorf("尾点丢失: got[last] = %v, want 999", got[len(got)-1])
		}
	})

	t.Run("抽稀保持顺序且不重复", func(t *testing.T) {
		records := makeRecords(500)
		got := downsampleHistory(records, 50)
		prev := -1
		for _, v := range got {
			if v <= prev {
				t.Fatalf("顺序错误或重复: %d after %d", v, prev)
			}
			prev = v
		}
	})

	t.Run("空切片", func(t *testing.T) {
		got := downsampleHistory([]int{}, 100)
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}
