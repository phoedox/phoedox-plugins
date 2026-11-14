// Package integ holds helpers shared by the integration plugins. It has no
// build tags so the logic stays testable natively; fetch_wasm.go adds the
// host calls for wasip1.
package integ

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify turns a name into a slug part (at most 60 characters).
func Slugify(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}

// BaseURL checks a configured address (http or https, no query) and drops a
// trailing slash.
func BaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.User != nil {
		return "", fmt.Errorf("the setting url must be an address like https://host:port")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// StripMask turns "10.0.0.1/24" into "10.0.0.1".
func StripMask(ip string) string {
	if i := strings.IndexByte(ip, '/'); i >= 0 {
		return ip[:i]
	}
	return ip
}

// GiB rounds bytes to tenths of a GiB.
func GiB(b float64) float64 { return math.Round(b/(1<<30)*10) / 10 }

// Truthy reads a yes/no setting.
func Truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "ja":
		return true
	}
	return false
}
