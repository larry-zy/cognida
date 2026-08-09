package datasource

import (
	"errors"
	"strings"
)

// sanitizeErr 错误脱敏：driver 错误可能携带 DSN/密码，回给上层前抹掉敏感串。
func sanitizeErr(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	return errors.New(msg)
}
