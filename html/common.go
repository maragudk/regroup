package html

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	. "maragu.dev/gomponents"
	data "maragu.dev/gomponents-datastar"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
	"maragu.dev/httph"
)

var hashOnce sync.Once
var appCSSPath, appJSPath string
var datastarJSPath string

// Page with the given body in a plain column, for pages with little on them, such as an error.
// It links back to the group.
func Page(props PageProps, body ...Node) Node {
	return document(props,
		Main(Class(wrap+" py-10 lg:py-14 [&_h1]:mb-6 [&_h1]:font-serif [&_h1]:text-[40px]/none [&_h1]:font-[620] [&_h1]:tracking-[-0.015em] [&_h1]:text-balance lg:[&_h1]:text-[64px]/none"),
			Group(body),
			P(Class("mt-8"), a(Href("/"), Text("Back to the group"))),
		),
	)
}

// wrap is the column every page keeps its content in, so that everything starts on one left edge.
const wrap = "mx-auto w-full max-w-[1440px] px-5 sm:px-8 lg:px-[72px]"

// document with the given body directly in it.
//
// Every stylesheet, script, and font is loaded from the app's own origin. If the request has a content
// security policy nonce from [httph.NonceFromContext], Datastar is told to use it, so that it needs no
// 'unsafe-eval' in the policy.
func document(props PageProps, body ...Node) Node {
	hashOnce.Do(func() {
		appCSSPath = getHashedPath("public/styles/app.css")
		appJSPath = getHashedPath("public/scripts/app.js")

		datastarJSPath = getHashedPath("public/scripts/datastar.js")
	})

	var nonce string
	if props.Ctx != nil {
		nonce = httph.NonceFromContext(props.Ctx)
	}

	return HTML5(HTML5Props{
		Title:       props.Title,
		Description: props.Description,
		Language:    "en",
		Head: Group{
			// The group is private, so no page is for a search engine.
			Meta(Name("robots"), Content("noindex")),
			Link(Rel("preload"), Href("/styles/fonts/Fraunces.woff2"), As("font"), Type("font/woff2"), CrossOrigin("anonymous")),
			Link(Rel("preload"), Href("/styles/fonts/SourceSans3-Regular.woff2"), As("font"), Type("font/woff2"), CrossOrigin("anonymous")),
			Link(Rel("stylesheet"), Href(appCSSPath)),
			Script(Type("module"), Src(datastarJSPath), Defer()),
			Script(Src(appJSPath), Defer()),
		},
		HTMLAttrs: Group{
			Class("scheme-light"),
			Iff(nonce != "", func() Node { return data.Nonce(nonce) }),
		},
		Body: Group{Class("bg-white font-sans text-[17px]/[1.6] text-stone-900 antialiased"),
			Group(body),
		},
	})
}

func a(children ...Node) Node {
	return A(Class("text-stone-900 underline decoration-stone-400 decoration-1 underline-offset-4 hover:decoration-stone-900"), Group(children))
}

func getHashedPath(path string) string {
	externalPath := strings.TrimPrefix(path, "public")
	ext := filepath.Ext(path)
	if ext == "" {
		panic("no extension found")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%v.x%v", strings.TrimSuffix(externalPath, ext), ext)
	}

	return fmt.Sprintf("%v.%x%v", strings.TrimSuffix(externalPath, ext), sha256.Sum256(data), ext)
}
