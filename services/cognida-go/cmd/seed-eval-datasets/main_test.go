package main

import "testing"

func TestFirstIntToken(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"目前系统内共有 60,000 笔订单。", 60000, true},
		{"共有 5,432 笔订单处于已取消（cancelled）状态。", 5432, true},
		{"共有 7 单。", 7, true},
		{"没有任何数字。", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := firstIntToken(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("firstIntToken(%q) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestFindGoldenCount(t *testing.T) {
	records := []recordLine{
		{Question: "系统里目前一共有多少笔订单？", ReferenceAnswer: "目前系统内共有 60,000 笔订单。"},
		{Question: "有多少笔订单处于已取消（cancelled）状态？", ReferenceAnswer: "共有 5,432 笔订单处于已取消（cancelled）状态。"},
	}
	for _, c := range ecommerceChecks {
		got, ok := findGoldenCount(records, c.questionSub)
		if !ok {
			t.Errorf("findGoldenCount 未命中 %q (%s)", c.questionSub, c.label)
			continue
		}
		if got <= 0 {
			t.Errorf("findGoldenCount(%s) = %d, 期望正整数", c.label, got)
		}
	}
	if _, ok := findGoldenCount(records, "不存在的题面"); ok {
		t.Error("findGoldenCount 应对不存在子串返回 false")
	}
}

// TestManifestHasNoXlam 守卫 A1：xLAM 已从 seed 清单剔除，不得再出现。
func TestManifestHasNoXlam(t *testing.T) {
	entries, err := loadManifest()
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	for _, e := range entries {
		if e.DatasetID == "hf_xlam_agent" {
			t.Errorf("manifest 仍含已剔除的 hf_xlam_agent")
		}
		// 嵌入的 records 文件应都能加载（无 orphan/缺失）。
		if _, err := loadRecords(e.RecordsFile); err != nil {
			t.Errorf("loadRecords(%q): %v", e.RecordsFile, err)
		}
	}
}
