package html_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"maragu.dev/is"

	"app/html"
)

func TestPage(t *testing.T) {
	t.Run("loads the Datastar script as a module", func(t *testing.T) {
		tag := regexp.MustCompile(`<script[^>]*src="/scripts/datastar\.[^"]+\.js"[^>]*>`).FindString(render(t))

		is.True(t, tag != "", "no Datastar script tag")
		is.True(t, strings.Contains(tag, `type="module"`), "Datastar script tag is not a module: "+tag)
	})

	t.Run("loads every script from a path on the app's own origin, and has no inline scripts", func(t *testing.T) {
		page := render(t)
		srcs := regexp.MustCompile(`<script[^>]*\ssrc="([^"]*)"`).FindAllStringSubmatch(page, -1)

		is.True(t, len(srcs) > 0, "no script tags with a src found")
		is.Equal(t, strings.Count(page, "<script"), len(srcs), "number of script tags compared to script tags with a src")
		for _, src := range srcs {
			path := src[1]
			is.True(t, strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.HasPrefix(path, `/\`),
				"script src is not an absolute path on the app's own origin: "+path)
		}
	})

	t.Run("renders the Datastar smoke test attribute, with the expression HTML-escaped", func(t *testing.T) {
		want := `data-init="console.log(&#39;Datastar loaded&#39;)"`

		is.True(t, strings.Contains(render(t), want), "no "+want)
	})
}

// TestDatastarVersion guards the two independent pins of the Datastar version against each other:
// the Go helpers emit attributes that only the matching client runtime understands.
func TestDatastarVersion(t *testing.T) {
	t.Run("the vendored bundle is the version the Makefile pins", func(t *testing.T) {
		makefile, err := os.ReadFile("../Makefile")
		is.NotError(t, err)

		match := regexp.MustCompile(`datastar@(\S+)/bundles/datastar\.js`).FindSubmatch(makefile)
		is.True(t, match != nil, "no Datastar bundle URL in the Makefile")

		bundle, err := os.ReadFile("../public/scripts/datastar.js")
		is.NotError(t, err)

		banner, _, _ := strings.Cut(string(bundle), "\n")
		is.Equal(t, "// Datastar v"+string(match[1]), banner)
	})
}

func render(t *testing.T) string {
	t.Helper()

	var b strings.Builder
	err := html.Page(html.PageProps{Title: "Test"}).Render(&b)
	is.NotError(t, err)
	return b.String()
}
