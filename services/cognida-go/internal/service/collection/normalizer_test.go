package collection

import "testing"

func TestTextNormalizerRemovesMarkupAndExecutableContentDeterministically(t *testing.T) {
	n := TextNormalizer{}
	input := []byte(`<html><head><style>.x{display:none}</style><script>alert(1)</script></head><body><h1>A &amp; B</h1><p> Useful text </p></body></html>`)
	got, err := n.Normalize("text/html; charset=utf-8", input)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got != "A & B Useful text" {
		t.Fatalf("normalized text = %q", got)
	}
	again, err := n.Normalize("text/html; charset=utf-8", input)
	if err != nil || again != got {
		t.Fatalf("normalization must be deterministic: %q, %v", again, err)
	}
}

func TestTextNormalizerRejectsUnsupportedOrEmptyContent(t *testing.T) {
	n := TextNormalizer{}
	if _, err := n.Normalize("application/pdf", []byte("x")); err == nil {
		t.Fatal("expected unsupported type rejection")
	}
	if _, err := n.Normalize("text/html", []byte("<script>x</script>")); err == nil {
		t.Fatal("expected empty content rejection")
	}
}
