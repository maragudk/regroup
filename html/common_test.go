package html_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	gluehttp "maragu.dev/glue/http"
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

// TestFavicon guards the app's icon: a plain square in the primary orange, as SVG for the pages to link
// to, and as an ICO file at the path a browser asks for when a page names none.
func TestFavicon(t *testing.T) {
	// orange is Tailwind's orange 500, oklch(70.5% 0.213 47.604), in sRGB, as Tailwind gives it too.
	orange := color.NRGBA{R: 0xff, G: 0x69, B: 0x00, A: 0xff}

	t.Run("every page links to the SVG icon", func(t *testing.T) {
		for _, page := range []Node{
			html.Page(html.PageProps{Title: "Test"}),
			html.GroupPage(html.GroupPageProps{GroupName: "The Loft"}),
			html.PrivatePage(html.PrivatePageProps{}),
		} {
			is.True(t, strings.Contains(render(t, page), `<link rel="icon" href="/favicon.svg" type="image/svg+xml">`))
		}
	})

	t.Run("the SVG icon is a plain square in orange", func(t *testing.T) {
		svg, err := os.ReadFile("../public/favicon.svg")
		is.NotError(t, err)
		is.Equal(t, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#ff6900"/></svg>`+"\n", string(svg))
	})

	t.Run("the ICO icon is one square image, every pixel of it orange", func(t *testing.T) {
		ico, err := os.ReadFile("../public/favicon.ico")
		is.NotError(t, err)

		// The header says it is an icon with one image, and where that image is.
		is.EqualSlice(t, []byte{0, 0, 1, 0, 1, 0}, ico[:6], "the header of an icon with one image")
		size, offset := binary.LittleEndian.Uint32(ico[14:18]), binary.LittleEndian.Uint32(ico[18:22])
		is.Equal(t, len(ico), int(offset+size))

		img, err := png.Decode(bytes.NewReader(ico[offset:]))
		is.NotError(t, err)

		// The bits per pixel in the header are those of the image: 24 for a PNG of red, green, and blue.
		is.Equal(t, 24, int(binary.LittleEndian.Uint16(ico[12:14])))
		is.Equal(t, byte(2), ico[offset+25], "the colour type of the PNG")
		bounds := img.Bounds()
		is.Equal(t, bounds.Dx(), bounds.Dy())
		is.Equal(t, int(ico[6]), bounds.Dx())
		is.Equal(t, int(ico[7]), bounds.Dy())
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				is.Equal(t, orange, color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA))
			}
		}
	})

	t.Run("both are served from the app's own public files, at the root, by the static files of every glue app", func(t *testing.T) {
		t.Chdir("..")
		mux := gluehttp.NewRouter(gluehttp.NewRouterOpts{}).Mux
		gluehttp.Static(mux)

		for _, path := range []string{"/favicon.svg", "/favicon.ico"} {
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
			is.Equal(t, http.StatusOK, res.Code, path)

			data, err := os.ReadFile(filepath.Join("public", path))
			is.NotError(t, err)
			is.True(t, bytes.Equal(data, res.Body.Bytes()), path)
		}
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
		"Fraunces.ttf":              "Fraunces-OFL.txt",
		"SourceSans3-Regular.woff2": "SourceSans3-LICENSE.md",
	}

	// types of the font files, by their extension: the media type a page preloads them as, and the first
	// bytes of a file of that kind.
	types := map[string]struct{ media, magic string }{
		".ttf":   {media: "font/ttf", magic: "\x00\x01\x00\x00"},
		".woff2": {media: "font/woff2", magic: "wOF2"},
	}

	t.Run("each font and its licence is the file that was fetched, byte for byte", func(t *testing.T) {
		// Each is the file from the font's own repository, unchanged: for Fraunces, its variable TrueType
		// font with all four axes. See the fonts target in the Makefile.
		for name, sum := range map[string]string{
			"Fraunces.ttf":              "0776a870a0856b296e11639505ac0cf9be5e7800bb1849dfa21a1bd182455fc0",
			"Fraunces-OFL.txt":          "bdf4c22802eaf804f998195871c6b8938aac2ac14b2d78a8bd66a6f1eced833b",
			"SourceSans3-Regular.woff2": "53492fb3a0def77354f166a55d09b63a10855e91c206c7620a81cf56e97f8ec3",
			"SourceSans3-LICENSE.md":    "56af9b9c6715597e458284a474dc118a50a4150e9d547c70f7b4a33c3e6a9328",
		} {
			data, err := os.ReadFile(filepath.Join("../public/styles/fonts", name))
			is.NotError(t, err)
			is.Equal(t, sum, fmt.Sprintf("%x", sha256.Sum256(data)), name)
		}
	})

	t.Run("each font is a TrueType or WOFF2 file, as its name says, with the text of its SIL Open Font License beside it", func(t *testing.T) {
		for font, licence := range fonts {
			data, err := os.ReadFile(filepath.Join("../public/styles/fonts", font))
			is.NotError(t, err)
			typ, ok := types[filepath.Ext(font)]
			is.True(t, ok, font)
			is.Equal(t, typ.magic, string(data[:4]), font)

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

		preloads := regexp.MustCompile(`<link rel="preload" href="([^"]*)" as="font" type="([^"]*)" crossorigin="anonymous">`).FindAllStringSubmatch(page, -1)
		is.Equal(t, len(fonts), len(preloads))
		for _, preload := range preloads {
			dir, font := filepath.Split(preload[1])
			is.Equal(t, "/styles/fonts/", dir)
			_, ok := fonts[font]
			is.True(t, ok, font)
			is.Equal(t, types[filepath.Ext(font)].media, preload[2], font)
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
			is.True(t, regexp.MustCompile(`^"fonts/[A-Za-z0-9-]+\.(?:ttf|woff2)"$`).MatchString(u[1]), u[1])

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
