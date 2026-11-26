package html

import (
	"maragu.dev/glue/html"

	. "maragu.dev/gomponents"
)

type PageProps = html.PageProps

type PageFunc = html.PageFunc

// ErrorPage for the request the given props describe.
func ErrorPage(props PageProps) Node {
	return html.ErrorPage(Page, props)
}

// NotFoundPage for the request the given props describe.
func NotFoundPage(props PageProps) Node {
	return html.NotFoundPage(Page, props)
}
