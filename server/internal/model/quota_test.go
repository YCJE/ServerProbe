package model

import "testing"

func TestCalcQuotaUsedBytes(t *testing.T) {
	const rx = uint64(300)
	const tx = uint64(500)

	cases := []struct {
		quotaType string
		want      uint64
	}{
		{QuotaTypeSum, 800},
		{QuotaTypeUp, 500},
		{QuotaTypeDown, 300},
		{QuotaTypeMax, 500},
		{QuotaTypeMin, 300},
		{"", 800},      // 空值兜底为 sum
		{"bogus", 800}, // 未知口径兜底为 sum（存量数据兼容）
	}
	for _, tc := range cases {
		if got := CalcQuotaUsedBytes(tc.quotaType, rx, tx); got != tc.want {
			t.Errorf("CalcQuotaUsedBytes(%q, 300, 500) = %d, 期望 %d", tc.quotaType, got, tc.want)
		}
	}
}

func TestCalcQuotaUsedBytes_EqualAndZero(t *testing.T) {
	// rx == tx 时 max/min 均返回该值
	if got := CalcQuotaUsedBytes(QuotaTypeMax, 100, 100); got != 100 {
		t.Errorf("max(100,100) = %d, 期望 100", got)
	}
	if got := CalcQuotaUsedBytes(QuotaTypeMin, 100, 100); got != 100 {
		t.Errorf("min(100,100) = %d, 期望 100", got)
	}
	// 零流量
	if got := CalcQuotaUsedBytes(QuotaTypeSum, 0, 0); got != 0 {
		t.Errorf("sum(0,0) = %d, 期望 0", got)
	}
	// 一侧为零
	if got := CalcQuotaUsedBytes(QuotaTypeMin, 0, 200); got != 0 {
		t.Errorf("min(0,200) = %d, 期望 0", got)
	}
}

func TestValidQuotaTypes(t *testing.T) {
	for _, v := range []string{QuotaTypeSum, QuotaTypeUp, QuotaTypeDown, QuotaTypeMax, QuotaTypeMin} {
		if !ValidQuotaTypes[v] {
			t.Errorf("口径 %q 应在白名单内", v)
		}
	}
	for _, v := range []string{"", "total", "SUM", "avg"} {
		if ValidQuotaTypes[v] {
			t.Errorf("口径 %q 不应在白名单内", v)
		}
	}
}
