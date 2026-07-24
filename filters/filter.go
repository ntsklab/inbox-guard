package filters

import (
	"fmt"
	"net/http"
	"strings"
)

// Filter is the interface for all spam detection filters.
// Return a non-empty reason string if the request should be blocked.
type Filter interface {
	Check(content, actor string, r *http.Request) string
}

// Reason builds a human-readable reason string.
func Reason(name string, args ...any) string {
	var b strings.Builder
	b.WriteString(name)
	for i := 0; i < len(args); i += 2 {
		b.WriteString(" ")
		b.WriteString(fmt.Sprint(args[i]))
		b.WriteString("=")
		b.WriteString(fmt.Sprint(args[i+1]))
	}
	return b.String()
}
