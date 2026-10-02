package collection

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	reScript   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script\s*>`)
	reStyle    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style\s*>`)
	reNoScript = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript\s*>`)
	reTag      = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace    = regexp.MustCompile(`[\t\r\n ]+`)
)

// TextNormalizer is intentionally small and deterministic; it can be swapped
// for a specialist extractor without changing the collection workflow.
type TextNormalizer struct{}

func (TextNormalizer) Normalize(contentType string, body []byte) (string, error) {
	if !strings.HasPrefix(strings.ToLower(contentType), "text/html") && !strings.HasPrefix(strings.ToLower(contentType), "text/plain") {
		return "", fmt.Errorf("unsupported textual content type %q", contentType)
	}
	text := string(body)
	if strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		text = reScript.ReplaceAllString(text, " ")
		text = reStyle.ReplaceAllString(text, " ")
		text = reNoScript.ReplaceAllString(text, " ")
		text = reTag.ReplaceAllString(text, " ")
	}
	text = strings.TrimSpace(reSpace.ReplaceAllString(html.UnescapeString(text), " "))
	if text == "" {
		return "", fmt.Errorf("empty document")
	}
	return text, nil
}
