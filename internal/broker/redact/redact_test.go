package redact

import (
	"strings"
	"testing"
)

func TestSecret(t *testing.T) {
	if got := Secret(""); got != "<empty>" {
		t.Errorf("Secret(\"\") = %q", got)
	}
	got := Secret("0101901234")
	if strings.Contains(got, "0101901234") || !strings.Contains(got, "len=10") {
		t.Errorf("Secret leaked value or missing length: %q", got)
	}
}

func TestURL(t *testing.T) {
	in := "https://www.nordnet.dk/login?code=abc123secret&state=xyz#frag"
	got := URL(in)
	for _, leak := range []string{"abc123secret", "xyz", "frag"} {
		if strings.Contains(got, leak) {
			t.Errorf("URL leaked %q: %s", leak, got)
		}
	}
	for _, keep := range []string{"https://www.nordnet.dk/login", "code=", "state="} {
		if !strings.Contains(got, keep) {
			t.Errorf("URL dropped %q: %s", keep, got)
		}
	}
	if got := URL("https://example.com/path"); got != "https://example.com/path" {
		t.Errorf("URL changed query-less URL: %s", got)
	}
}

func TestBodyMasksSensitiveJSON(t *testing.T) {
	in := `{"jwt":"eyJhbGciOi.payload.sig","providedCpr":"0101901234","success":false,` +
		`"remainingAttempts":2,"errorCode":"control.bad","m2":{"value":"deadbeef"},` +
		`"items":[{"access_token":"at-1","expires_in":1200}],"session_id":"s-1","state":null}`
	got := Body([]byte(in))
	for _, leak := range []string{"eyJhbGciOi", "0101901234", "deadbeef", "at-1", "s-1"} {
		if strings.Contains(got, leak) {
			t.Errorf("Body leaked %q: %s", leak, got)
		}
	}
	for _, keep := range []string{`"success":false`, `"remainingAttempts":2`, `"errorCode":"control.bad"`, `"expires_in":1200`, `"state":null`} {
		if !strings.Contains(got, keep) {
			t.Errorf("Body dropped %q: %s", keep, got)
		}
	}
}

func TestBodyNonJSONAndTruncation(t *testing.T) {
	html := []byte(`<div data-csrf="tok" data-cpr="0101901234"></div>`)
	got := Body(html)
	if strings.Contains(got, "tok") || strings.Contains(got, "0101901234") {
		t.Errorf("Body leaked HTML content: %s", got)
	}
	if Body(nil) != "<empty body>" {
		t.Errorf("Body(nil) = %q", Body(nil))
	}
	long := []byte(`{"msg":"` + strings.Repeat("x", 2000) + `"}`)
	if got := Body(long); len(got) > maxBodyLen+64 || !strings.Contains(got, "truncated") {
		t.Errorf("Body not truncated: len=%d", len(got))
	}
}
