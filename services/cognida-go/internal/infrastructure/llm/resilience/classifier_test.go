package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	domainllm "cognida/internal/model/llm"
)

func TestClassify_HTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		want domainllm.ErrorClass
	}{
		{429, domainllm.ClassRateLimited},
		{408, domainllm.ClassTransient},
		{500, domainllm.ClassTransient},
		{503, domainllm.ClassTransient},
		{400, domainllm.ClassTerminal},
		{401, domainllm.ClassTerminal},
		{404, domainllm.ClassTerminal},
	}
	for _, c := range cases {
		if got := Classify(c.code, nil); got != c.want {
			t.Errorf("Classify(%d)=%s want %s", c.code, got, c.want)
		}
	}
}

func TestClassify_ContextErrors(t *testing.T) {
	if got := Classify(0, context.Canceled); got != domainllm.ClassCanceled {
		t.Errorf("canceled: got %s", got)
	}
	if got := Classify(0, context.DeadlineExceeded); got != domainllm.ClassTransient {
		t.Errorf("deadline: got %s want transient", got)
	}
	// ctx 取消优先于状态码
	if got := Classify(400, context.Canceled); got != domainllm.ClassCanceled {
		t.Errorf("canceled over status: got %s", got)
	}
}

func TestClassify_TransientNetErr(t *testing.T) {
	for _, msg := range []string{
		"read tcp 1.2.3.4:443: connection reset by peer",
		"dial tcp: connection refused",
		"write: broken pipe",
		"net/http: TLS handshake timeout",
		"dial tcp: lookup api.x.com: no such host",
	} {
		if got := Classify(0, errors.New(msg)); got != domainllm.ClassTransient {
			t.Errorf("%q: got %s want transient", msg, got)
		}
	}
}

func TestClassify_UnknownIsTerminal(t *testing.T) {
	if got := Classify(0, errors.New("totally unexpected")); got != domainllm.ClassTerminal {
		t.Errorf("unknown: got %s want terminal", got)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	if got := ParseRetryAfter("5", 30*time.Second, time.Unix(0, 0)); got != 5*time.Second {
		t.Errorf("got %v want 5s", got)
	}
}

func TestParseRetryAfter_Cap(t *testing.T) {
	if got := ParseRetryAfter("120", 30*time.Second, time.Unix(0, 0)); got != 30*time.Second {
		t.Errorf("got %v want capped 30s", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(10 * time.Second).UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	if got := ParseRetryAfter(future, 30*time.Second, now); got != 10*time.Second {
		t.Errorf("got %v want 10s", got)
	}
}

func TestParseRetryAfter_Empty(t *testing.T) {
	if got := ParseRetryAfter("", 30*time.Second, time.Unix(0, 0)); got != 0 {
		t.Errorf("got %v want 0", got)
	}
	if got := ParseRetryAfter("garbage", 30*time.Second, time.Unix(0, 0)); got != 0 {
		t.Errorf("garbage: got %v want 0", got)
	}
}
