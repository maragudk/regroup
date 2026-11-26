package http_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"maragu.dev/httph"
	"maragu.dev/is"

	apphttp "app/http"
)

func TestCSP(t *testing.T) {
	for _, allowUnsafeInline := range []bool{false, true} {
		name := "allows no other origin in any directive, and never the evaluation of strings as scripts, with unsafe inline "
		t.Run(name+map[bool]string{false: "off", true: "on"}[allowUnsafeInline], func(t *testing.T) {
			directives, nonce := policy(t, allowUnsafeInline)

			for directive, sources := range directives {
				for source := range strings.FieldsSeq(sources) {
					allowed := source == "'self'" || source == "'none'" ||
						(source == "'nonce-"+nonce+"'" && directive == "script-src") ||
						(source == "'unsafe-inline'" && directive == "style-src" && allowUnsafeInline)
					is.True(t, allowed, "unexpected source in "+directive+": "+source)
				}
			}
		})
	}

	t.Run("denies everything by default and allows the app's own origin for what it needs", func(t *testing.T) {
		directives, _ := policy(t, false)

		is.Equal(t, "'none'", directives["default-src"])
		for _, directive := range []string{"connect-src", "font-src", "img-src", "manifest-src", "style-src"} {
			is.Equal(t, "'self'", directives[directive], directive)
		}
	})

	t.Run("allows scripts from the app's own origin and with the nonce of the response, which is new each time", func(t *testing.T) {
		directives, nonce := policy(t, false)
		is.True(t, regexp.MustCompile(`^[A-Za-z0-9]{20,}$`).MatchString(nonce), nonce)
		is.Equal(t, "'self' 'nonce-"+nonce+"'", directives["script-src"])

		_, other := policy(t, false)
		is.True(t, nonce != other, "the same nonce twice")
	})

	t.Run("adds unsafe inline to styles only", func(t *testing.T) {
		directives, nonce := policy(t, true)

		is.Equal(t, "'self' 'unsafe-inline'", directives["style-src"])
		is.Equal(t, "'self' 'nonce-"+nonce+"'", directives["script-src"])
	})
}

// policy served with [apphttp.CSP], as a map from directive name to its sources, and the nonce the
// handler behind it was given.
func policy(t *testing.T, allowUnsafeInline bool) (map[string]string, string) {
	t.Helper()

	var nonce string
	h := httph.ContentSecurityPolicy(apphttp.CSP(allowUnsafeInline))(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		nonce = httph.NonceFromContext(r.Context())
	}))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	return directives(t, res.Header().Get("Content-Security-Policy")), nonce
}

// directives of the given Content-Security-Policy header, as a map from directive name to its sources.
func directives(t *testing.T, csp string) map[string]string {
	t.Helper()

	is.True(t, csp != "", "no Content-Security-Policy header")

	directives := map[string]string{}
	for directive := range strings.SplitSeq(csp, ";") {
		name, sources, _ := strings.Cut(strings.TrimSpace(directive), " ")
		directives[name] = sources
	}
	return directives
}
