package pagination

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// 游标编解码往返：非零游标可无损还原。
func TestCursor_RoundTrip(t *testing.T) {
	in := Cursor{Sort: "2026-08-10T12:00:00Z", ID: 42}
	enc := in.Encode()
	if enc == "" {
		t.Fatalf("非零游标不应编码为空串")
	}
	// 不透明：不应把内部字段明文暴露给客户端。
	if strings.Contains(enc, "42") || strings.Contains(enc, "2026") {
		t.Fatalf("游标应为不透明串，疑似明文泄漏：%s", enc)
	}
	out, err := Decode(enc)
	if err != nil {
		t.Fatalf("解码不应出错：%v", err)
	}
	if out != in {
		t.Fatalf("往返不一致：%+v vs %+v", out, in)
	}
}

// 空串解码为零游标（首页语义），且不报错。
func TestCursor_DecodeEmptyIsFirstPage(t *testing.T) {
	c, err := Decode("")
	if err != nil {
		t.Fatalf("空串不应报错：%v", err)
	}
	if !c.IsZero() {
		t.Fatalf("空串应解码为零游标（首页），得 %+v", c)
	}
}

// 零游标编码为空串。
func TestCursor_ZeroEncodesEmpty(t *testing.T) {
	if s := (Cursor{}).Encode(); s != "" {
		t.Fatalf("零游标应编码为空串，得 %q", s)
	}
}

// 被篡改/非法的游标串必须报错，不得被当成首页静默吞掉。
func TestCursor_DecodeTamperedErrors(t *testing.T) {
	cases := []string{
		"!!!not-base64!!!",       // 非法 base64
		"YWJjZGVم",               // 含非 URL-safe 字符
		"eyJub3QiOiJjdXJzb3Ii",   // 合法 base64 但非 Cursor JSON 结构片段
	}
	for _, s := range cases {
		_, err := Decode(s)
		if err == nil {
			// 允许「合法 base64 且恰好是合法 JSON 对象」解出零值，但不能 panic；
			// 这里的样本均为非法编码或截断 JSON，必须报错。
			t.Fatalf("非法游标串应报错：%q", s)
		}
		// 畸形游标须挂 ErrInvalidCursor 链，供 handler 经 errors.Is 映射为 400。
		if !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("畸形游标 %q 的错误应包裹 ErrInvalidCursor，得：%v", s, err)
		}
	}
}

func TestNormalizeLimit(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, DefaultLimit},
		{-5, DefaultLimit},
		{1, 1},
		{50, 50},
		{MaxLimit, MaxLimit},
		{MaxLimit + 1, MaxLimit},
		{100000, MaxLimit},
	}
	for _, tc := range cases {
		if got := NormalizeLimit(tc.in); got != tc.want {
			t.Fatalf("NormalizeLimit(%d)=%d，期望 %d", tc.in, got, tc.want)
		}
	}
}

// keyset 谓词：Desc 取更旧一侧，Asc 取更新一侧，参数三元组顺序为 (v, v, id)。
func TestKeysetPredicate(t *testing.T) {
	ts := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	descClause, descArgs := KeysetPredicate("created_at", "id", ts, 42, Desc)
	if !strings.Contains(descClause, "created_at < ?") || !strings.Contains(descClause, "id < ?") {
		t.Fatalf("Desc 谓词方向错误：%s", descClause)
	}
	ascClause, _ := KeysetPredicate("created_at", "id", ts, 42, Asc)
	if !strings.Contains(ascClause, "created_at > ?") || !strings.Contains(ascClause, "id > ?") {
		t.Fatalf("Asc 谓词方向错误：%s", ascClause)
	}
	if len(descArgs) != 3 {
		t.Fatalf("参数应为三元组 (v,v,id)，得 %d 个", len(descArgs))
	}
	if descArgs[0] != any(ts) || descArgs[1] != any(ts) {
		t.Fatalf("前两参数应为排序值，得 %+v", descArgs[:2])
	}
	if descArgs[2] != any(int64(42)) {
		t.Fatalf("末参数应为 tie-breaker id，得 %v", descArgs[2])
	}
}
