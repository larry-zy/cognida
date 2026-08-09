package llm

import (
	"errors"
	"testing"
	"time"
)

func TestClassifyHTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		want ErrorClass
	}{
		{429, ClassRateLimited},
		{408, ClassTransient},
		{500, ClassTransient},
		{502, ClassTransient},
		{503, ClassTransient},
		{504, ClassTransient},
		{400, ClassTerminal},
		{401, ClassTerminal},
		{403, ClassTerminal},
		{404, ClassTerminal},
		{422, ClassTerminal},
		{200, ""},
		{204, ""},
	}
	for _, c := range cases {
		if got := ClassifyHTTPStatus(c.code); got != c.want {
			t.Errorf("ClassifyHTTPStatus(%d) = %q, want %q", c.code, got, c.want)
		}
	}
}

func TestErrorClassRetryableCountable(t *testing.T) {
	for _, c := range []ErrorClass{ClassTransient, ClassRateLimited} {
		if !c.Retryable() {
			t.Errorf("%s should be retryable", c)
		}
		if !c.Countable() {
			t.Errorf("%s should be countable", c)
		}
	}
	for _, c := range []ErrorClass{ClassTerminal, ClassCanceled} {
		if c.Retryable() {
			t.Errorf("%s should not be retryable", c)
		}
		if c.Countable() {
			t.Errorf("%s should not be countable", c)
		}
	}
}

func TestAPIErrorUnwrap(t *testing.T) {
	base := errors.New("underlying boom")
	ae := &APIError{Provider: ProviderOpenAI, Model: "gpt-4o", StatusCode: 500, Class: ClassTransient, Err: base}

	if !errors.Is(ae, base) {
		t.Fatal("errors.Is should reach underlying error via Unwrap")
	}
	got, ok := AsAPIError(ae)
	if !ok || got != ae {
		t.Fatal("AsAPIError should extract the *APIError")
	}
	wrapped := errors.New("outer: " + ae.Error())
	_ = wrapped
}

func TestAPIErrorMessageSanitized(t *testing.T) {
	ae := &APIError{
		Provider:   ProviderOpenAI,
		Model:      "gpt-4o",
		StatusCode: 401,
		Class:      ClassTerminal,
		Detail:     SummarizeDetail("bad key Authorization: Bearer sk-secret-abcdef123456 rejected"),
	}
	msg := ae.Error()
	if wantContains(msg, "sk-secret") || wantContains(msg, "Bearer sk-secret") {
		t.Fatalf("error message leaked secret: %s", msg)
	}
	if !wantContains(msg, "REDACTED") {
		t.Fatalf("expected redaction marker in: %s", msg)
	}
}

func TestRedactSecrets(t *testing.T) {
	in := "call failed Authorization: Bearer sk-1234567890abcdef and token sk-abcdefgh12345"
	out := RedactSecrets(in)
	if wantContains(out, "sk-1234567890abcdef") {
		t.Errorf("bearer token not redacted: %s", out)
	}
	if wantContains(out, "sk-abcdefgh12345") {
		t.Errorf("sk key not redacted: %s", out)
	}
}

func TestResilienceConfigWithDefaults(t *testing.T) {
	cfg := ResilienceConfig{Enabled: true}.WithDefaults()
	d := DefaultResilienceConfig()
	if cfg.MaxAttempts != d.MaxAttempts || cfg.BaseBackoff != d.BaseBackoff ||
		cfg.MaxBackoff != d.MaxBackoff || cfg.RetryAfterCap != d.RetryAfterCap ||
		cfg.FailThreshold != d.FailThreshold || cfg.Cooldown != d.Cooldown ||
		cfg.HalfOpenProbes != d.HalfOpenProbes {
		t.Fatalf("WithDefaults did not fill zero fields: %+v", cfg)
	}

	// 已设置的字段不被覆盖
	custom := ResilienceConfig{Enabled: true, MaxAttempts: 7, Cooldown: time.Minute}.WithDefaults()
	if custom.MaxAttempts != 7 || custom.Cooldown != time.Minute {
		t.Fatalf("WithDefaults overwrote explicit fields: %+v", custom)
	}
}

func TestTargetKey(t *testing.T) {
	if got := TargetKey(ProviderOpenAI, "gpt-4o"); got != "openai/gpt-4o" {
		t.Fatalf("TargetKey = %q", got)
	}
}

func wantContains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
