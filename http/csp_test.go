package http_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"maragu.dev/httph"
	"maragu.dev/is"

	apphttp "app/http"
)

func TestCSP(t *testing.T) {
	for _, allowUnsafeInline := range []bool{false, true} {
		for _, allowUnsafeEval := range []bool{false, true} {
			t.Run(fmt.Sprintf("allows no other origin in any directive, with unsafe inline %v and unsafe eval %v", allowUnsafeInline, allowUnsafeEval), func(t *testing.T) {
				for directive, sources := range policy(t, allowUnsafeInline, allowUnsafeEval) {
					for source := range strings.FieldsSeq(sources) {
						is.True(t, slices.Contains([]string{"'self'", "'none'", "'unsafe-inline'", "'unsafe-eval'"}, source),
							"unexpected source in "+directive+": "+source)
					}
				}
			})
		}
	}

	t.Run("denies everything by default and allows the app's own origin for what it needs", func(t *testing.T) {
		directives := policy(t, false, false)

		is.Equal(t, "'none'", directives["default-src"])
		for _, directive := range []string{"connect-src", "font-src", "img-src", "manifest-src", "script-src", "style-src"} {
			is.Equal(t, "'self'", directives[directive], directive)
		}
	})

	tests := []struct {
		allowUnsafeInline, allowUnsafeEval bool
		scriptSrc, styleSrc                string
	}{
		{false, false, "'self'", "'self'"},
		{true, false, "'self' 'unsafe-inline'", "'self' 'unsafe-inline'"},
		{false, true, "'self' 'unsafe-eval'", "'self'"},
		{true, true, "'self' 'unsafe-inline' 'unsafe-eval'", "'self' 'unsafe-inline'"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("adds unsafe inline to scripts and styles, and unsafe eval to scripts only, with unsafe inline %v and unsafe eval %v", test.allowUnsafeInline, test.allowUnsafeEval), func(t *testing.T) {
			directives := policy(t, test.allowUnsafeInline, test.allowUnsafeEval)

			is.Equal(t, test.scriptSrc, directives["script-src"], "script-src")
			is.Equal(t, test.styleSrc, directives["style-src"], "style-src")
		})
	}
}

// policy served with [apphttp.CSP], as a map from directive name to its sources.
func policy(t *testing.T, allowUnsafeInline, allowUnsafeEval bool) map[string]string {
	t.Helper()

	h := httph.ContentSecurityPolicy(apphttp.CSP(allowUnsafeInline, allowUnsafeEval))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	csp := res.Header().Get("Content-Security-Policy")
	is.True(t, csp != "", "no Content-Security-Policy header")

	directives := map[string]string{}
	for directive := range strings.SplitSeq(csp, ";") {
		name, sources, _ := strings.Cut(strings.TrimSpace(directive), " ")
		directives[name] = sources
	}
	return directives
}
