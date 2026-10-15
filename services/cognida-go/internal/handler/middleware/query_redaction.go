package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const redactedQueryValue = "[REDACTED]"

// redactSensitiveQuery is shared by the audit sink and request logger. It only
// changes their output, never the live URL used by SSE authentication or routes.
func redactSensitiveQuery(raw string) string {
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		// Partial parsing can discard a sensitive key. Never log the raw fallback.
		return redactedQueryValue
	}
	for key, entries := range values {
		normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
		switch normalized {
		case "token", "accesstoken", "refreshtoken", "idtoken", "authtoken", "apikey", "password", "passwd", "secret", "clientsecret", "authorization", "signature":
			for i := range entries {
				entries[i] = redactedQueryValue
			}
		}
	}
	return values.Encode()
}

func redactedRequestLog(param gin.LogFormatterParams) string {
	path, raw, _ := strings.Cut(param.Path, "?")
	if param.Request != nil && param.Request.URL != nil {
		path, raw = param.Request.URL.EscapedPath(), param.Request.URL.RawQuery
	}
	if raw != "" {
		path += "?" + redactSensitiveQuery(raw)
	}
	return fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %-7s %q\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"), param.StatusCode,
		param.Latency, param.ClientIP, param.Method, path, param.ErrorMessage)
}
