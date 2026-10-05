// Package redact provides helpers for logging broker authentication traffic
// without leaking secrets (tokens, codes, cookies) or personal identifiers
// (CPR numbers, MitID user IDs) into application logs.
package redact

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Placeholder replaces sensitive values.
const Placeholder = "[REDACTED]"

// maxBodyLen caps the length of a sanitized body included in logs or errors.
const maxBodyLen = 512

// sensitiveKeys are matched against lowercased JSON keys and URL query
// parameter names. A key is sensitive if it contains any of these substrings.
var sensitiveKeys = []string{
	"token", "jwt", "secret", "password", "passwd",
	"cpr", "nationalid", "national_id", "identityclaim", "username", "userid",
	"csrf", "ntag", "cookie", "session", "ticket", "nonce",
	"signature", "flowkey", "eafehash", "securitycontext", "salt", "encauth",
	"channelbinding", "authorization", "authcode", "aux",
	"clientkey", "accountkey",
}

// sensitiveExactKeys are matched against lowercased keys in full, for names
// that are too short or generic to match as substrings.
var sensitiveExactKeys = map[string]bool{
	"code": true, "state": true, "response": true, "verifier": true, "code_verifier": true,
	"m1": true, "m2": true, "randoma": true, "randomb": true,
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	if sensitiveExactKeys[k] {
		return true
	}
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// Secret describes a secret value without revealing it.
func Secret(s string) string {
	if s == "" {
		return "<empty>"
	}
	return fmt.Sprintf("%s(len=%d)", Placeholder, len(s))
}

// URL returns u with every query parameter and fragment value redacted,
// keeping the scheme, host, path and parameter names for debugging.
func URL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return "<unparseable URL>"
	}
	parsed.User = nil
	if parsed.RawQuery != "" {
		q := parsed.Query()
		for k := range q {
			q[k] = []string{Placeholder}
		}
		parsed.RawQuery = q.Encode()
	}
	if parsed.Fragment != "" {
		parsed.Fragment = Placeholder
		parsed.RawFragment = ""
	}
	return parsed.String()
}

// Body returns a log-safe description of an HTTP response body. JSON bodies
// have sensitive fields masked and are truncated; non-JSON bodies (e.g. HTML
// pages that may embed CSRF tokens or codes) are reduced to their length.
func Body(b []byte) string {
	if len(b) == 0 {
		return "<empty body>"
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Sprintf("<non-JSON body, %d bytes>", len(b))
	}
	out, err := json.Marshal(mask(v))
	if err != nil {
		return fmt.Sprintf("<body, %d bytes>", len(b))
	}
	if len(out) > maxBodyLen {
		return fmt.Sprintf("%s...(truncated, %d bytes)", out[:maxBodyLen], len(b))
	}
	return string(out)
}

func mask(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, val := range t {
			if isSensitiveKey(k) && val != nil {
				t[k] = Placeholder
			} else {
				t[k] = mask(val)
			}
		}
		return t
	case []interface{}:
		for i, val := range t {
			t[i] = mask(val)
		}
		return t
	default:
		return v
	}
}
