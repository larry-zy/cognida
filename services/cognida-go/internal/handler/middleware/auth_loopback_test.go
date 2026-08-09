package middleware

import "testing"

// TestIsLoopbackAddr 覆盖 DEV_MODE 本机来源硬约束〔INF-3〕的判定逻辑。
func TestIsLoopbackAddr(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.5", true},
		{"::1", true},
		{"0:0:0:0:0:0:0:1", true},
		{"10.0.0.3", false},
		{"192.168.1.1", false},
		{"203.0.113.7", false},
		{"", false},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := isLoopbackAddr(c.ip); got != c.want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
}
