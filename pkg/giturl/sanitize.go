// Package giturl provides utilities for safely handling Git remote URLs.
package giturl

import (
	"net/url"
	"strings"

	giturls "github.com/whilp/git-urls"
)

// Sanitize removes embedded credentials while preserving safe SSH usernames.
// Malformed URLs are rejected rather than returned unchanged.
func Sanitize(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return sanitizeHTTP(raw)
	}

	parsed, err := giturls.Parse(raw)
	if err != nil {
		return ""
	}

	if keepSSHUserinfo(parsed) {
		return parsed.String()
	}

	if parsed.User == nil {
		return parsed.String()
	}

	username := parsed.User.Username()
	_, hasPassword := parsed.User.Password()
	if hasPassword && isSSHLike(parsed) && username != "" {
		parsed.User = url.User(username)
		return parsed.String()
	}

	parsed.User = nil
	return parsed.String()
}

func sanitizeHTTP(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	return parsed.String()
}

func keepSSHUserinfo(parsed *url.URL) bool {
	if parsed == nil || parsed.User == nil {
		return true
	}
	if !isSSHLike(parsed) {
		return false
	}
	_, hasPassword := parsed.User.Password()
	return !hasPassword
}

func isSSHLike(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "ssh", "git+ssh":
		return true
	default:
		return false
	}
}
