package context

import "testing"

func TestApproxTokenCounter_Count(t *testing.T) {
	c := ApproxTokenCounter{}
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},          // 非空至少 1
		{"abcd", 1},       // 4*0.25=1
		{"你好", 1},         // 2*0.7=1.4→1
		{"你好世界啊", 3},     // 5*0.7=3.5→3
		{"abcdefgh", 2},   // 8*0.25=2
	}
	for _, tc := range cases {
		if got := c.Count(tc.in); got != tc.want {
			t.Errorf("Count(%q)=%d, want %d", tc.in, got, tc.want)
		}
	}
}
