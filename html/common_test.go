package html_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
	"maragu.dev/httph"
	"maragu.dev/is"

	"app/html"
)

func TestPage(t *testing.T) {
	t.Run("loads the Datastar script as a module", func(t *testing.T) {
		tag := regexp.MustCompile(`<script[^>]*src="/scripts/datastar\.[^"]+\.js"[^>]*>`).FindString(render(t, html.Page(html.PageProps{Title: "Test"})))

		is.True(t, tag != "", "no Datastar script tag")
		is.True(t, strings.Contains(tag, `type="module"`), "Datastar script tag is not a module: "+tag)
	})

	t.Run("loads every script from a path on the app's own origin, and has no inline scripts", func(t *testing.T) {
		page := render(t, html.Page(html.PageProps{Title: "Test"}))
		srcs := regexp.MustCompile(`<script[^>]*\ssrc="([^"]*)"`).FindAllStringSubmatch(page, -1)

		is.True(t, len(srcs) > 0, "no script tags with a src found")
		is.Equal(t, strings.Count(page, "<script"), len(srcs), "number of script tags compared to script tags with a src")
		for _, src := range srcs {
			path := src[1]
			is.True(t, strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.HasPrefix(path, `/\`),
				"script src is not an absolute path on the app's own origin: "+path)
		}
	})

	t.Run("gives Datastar the nonce of the request's content security policy, and none when the request has none", func(t *testing.T) {
		var ctx context.Context
		h := httph.ContentSecurityPolicy(func(opts *httph.ContentSecurityPolicyOptions) {
			opts.ScriptNonce = true
		})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			ctx = r.Context()
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		nonce := httph.NonceFromContext(ctx)
		is.True(t, nonce != "", "no nonce")

		page := render(t, html.Page(html.PageProps{Title: "Test", Ctx: ctx}))
		is.True(t, strings.Contains(page, `<html lang="en" class="scheme-light" data-nonce="`+nonce+`">`), page)

		page = render(t, html.Page(html.PageProps{Title: "Test", Ctx: t.Context()}))
		is.True(t, !strings.Contains(page, "data-nonce"), page)

		page = render(t, html.Page(html.PageProps{Title: "Test"}))
		is.True(t, !strings.Contains(page, "data-nonce"), page)
	})

	t.Run("has the given title and body, and a link back to the group", func(t *testing.T) {
		page := render(t, html.Page(html.PageProps{Title: "Not found"}, H1(Text("Not found"))))

		is.True(t, strings.Contains(page, "<title>Not found</title>"), page)
		is.True(t, strings.Contains(page, "<h1>Not found</h1>"), page)
		is.True(t, regexp.MustCompile(`<a [^>]*href="/"[^>]*>Back to the group</a>`).MatchString(page), page)
	})

	t.Run("asks search engines to leave the page alone, since the group is private", func(t *testing.T) {
		page := render(t, html.Page(html.PageProps{Title: "Test"}))

		is.True(t, strings.Contains(page, `<meta name="robots" content="noindex">`), page)
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

// TestFonts guards the promise that no page asks a font host for anything: the fonts are files of the
// app's own, each with its licence beside it, and the stylesheet and the pages name nothing else.
func TestFonts(t *testing.T) {
	fonts := map[string]string{
		"Fraunces.woff2":            "Fraunces-OFL.txt",
		"SourceSans3-Regular.woff2": "SourceSans3-LICENSE.md",
	}

	t.Run("each font and its licence is the file that was fetched, byte for byte", func(t *testing.T) {
		// Source Sans 3 and both licences are the files from the fonts' own repositories. Fraunces is
		// its variable TrueType font from there, in a WOFF2 container. See the fonts target in the Makefile.
		for name, sum := range map[string]string{
			"Fraunces.woff2":            "4454fbcc79aa12f2e6ce407cbabdc64896afed32d494d4671602a436e84e7d44",
			"Fraunces-OFL.txt":          "bdf4c22802eaf804f998195871c6b8938aac2ac14b2d78a8bd66a6f1eced833b",
			"SourceSans3-Regular.woff2": "53492fb3a0def77354f166a55d09b63a10855e91c206c7620a81cf56e97f8ec3",
			"SourceSans3-LICENSE.md":    "56af9b9c6715597e458284a474dc118a50a4150e9d547c70f7b4a33c3e6a9328",
		} {
			data, err := os.ReadFile(filepath.Join("../public/styles/fonts", name))
			is.NotError(t, err)
			is.Equal(t, sum, fmt.Sprintf("%x", sha256.Sum256(data)), name)
		}
	})

	t.Run("each font is a WOFF2 file with the text of its SIL Open Font License beside it", func(t *testing.T) {
		for font, licence := range fonts {
			data, err := os.ReadFile(filepath.Join("../public/styles/fonts", font))
			is.NotError(t, err)
			is.Equal(t, "wOF2", string(data[:4]), font)

			text, err := os.ReadFile(filepath.Join("../public/styles/fonts", licence))
			is.NotError(t, err)
			is.True(t, strings.Contains(string(text), "SIL OPEN FONT LICENSE Version 1.1"), licence)
			is.True(t, strings.Contains(string(text), "Copyright"), licence)
		}
	})

	t.Run("there is no font file that nothing uses", func(t *testing.T) {
		var found []string
		err := filepath.WalkDir("../public", func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch filepath.Ext(path) {
			case ".woff", ".woff2", ".ttf", ".otf", ".eot":
				found = append(found, filepath.Base(path))
			}
			return nil
		})
		is.NotError(t, err)
		is.Equal(t, len(fonts), len(found), strings.Join(found, ", "))
		for _, font := range found {
			_, ok := fonts[font]
			is.True(t, ok, font)
		}
	})

	t.Run("the pages preload exactly these fonts, from the path the stylesheet loads them from", func(t *testing.T) {
		page := render(t, html.Page(html.PageProps{Title: "Test"}))

		preloads := regexp.MustCompile(`<link rel="preload" href="([^"]*)" as="font" type="font/woff2" crossorigin="anonymous">`).FindAllStringSubmatch(page, -1)
		is.Equal(t, len(fonts), len(preloads))
		for _, preload := range preloads {
			dir, font := filepath.Split(preload[1])
			is.Equal(t, "/styles/fonts/", dir)
			_, ok := fonts[font]
			is.True(t, ok, font)
		}
	})
}

// TestStylesheet guards the same promise for the stylesheet. The built stylesheet is not in the
// repository, so this checks everything it is built from: its source, and the Go files that Tailwind
// reads class names from. The Dockerfile checks the built stylesheet itself.
func TestStylesheet(t *testing.T) {
	t.Run("its source loads nothing but the app's own fonts, and imports nothing but Tailwind", func(t *testing.T) {
		source, err := os.ReadFile("../tailwind.css")
		is.NotError(t, err)

		urls := regexp.MustCompile(`url\(([^)]*)\)`).FindAllStringSubmatch(string(source), -1)
		is.Equal(t, 2, len(urls))
		for _, u := range urls {
			is.True(t, regexp.MustCompile(`^"fonts/[A-Za-z0-9-]+\.woff2"$`).MatchString(u[1]), u[1])

			_, err := os.Stat(filepath.Join("../public/styles", strings.Trim(u[1], `"`)))
			is.NotError(t, err)
		}

		imports := regexp.MustCompile(`@import[^;]*;`).FindAllString(string(source), -1)
		is.EqualSlice(t, []string{`@import "tailwindcss";`}, imports)

		is.True(t, !strings.Contains(string(source), "//"), "the source names a scheme or a host")
	})

	t.Run("the pages name the accent colour only as primary, so that it can be changed in one place", func(t *testing.T) {
		source, err := os.ReadFile("../tailwind.css")
		is.NotError(t, err)
		for _, shade := range []string{"50", "100", "200", "300", "400", "500", "600", "700", "800", "900", "950"} {
			is.True(t, strings.Contains(string(source), "--color-primary-"+shade+": var(--color-orange-"+shade+");"), shade)
		}

		colours := regexp.MustCompile(`\b(?:red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|slate|gray|zinc|neutral)-\d{2,3}\b`)
		var files int
		for _, dir := range []string{"../html", "../preview"} {
			paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
			is.NotError(t, err)
			for _, path := range paths {
				if strings.HasSuffix(path, "_test.go") {
					continue
				}
				files++

				source, err := os.ReadFile(path)
				is.NotError(t, err)
				is.EqualSlice(t, nil, colours.FindAllString(string(source), -1), path)
			}
		}
		is.True(t, files > 3, "expected to have read the Go files")
	})

	t.Run("no class in the Go files that Tailwind reads can put a URL or an import into it", func(t *testing.T) {
		var files int
		for _, dir := range []string{"../html", "../preview", "../http", "../cmd/app"} {
			paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
			is.NotError(t, err)
			for _, path := range paths {
				if strings.HasSuffix(path, "_test.go") {
					continue
				}
				files++

				source, err := os.ReadFile(path)
				is.NotError(t, err)
				is.True(t, !strings.Contains(string(source), "url("), path+" has a url( in it")
				is.True(t, !strings.Contains(string(source), "@import"), path+" has an @import in it")
			}
		}
		is.True(t, files > 5, "expected to have read the Go files")
	})
}

func render(t *testing.T, n Node) string {
	t.Helper()

	var b strings.Builder
	err := n.Render(&b)
	is.NotError(t, err)
	return b.String()
}
