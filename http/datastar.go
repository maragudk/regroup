package http

import (
	"bytes"
	"net/http"
	"strings"

	. "maragu.dev/gomponents"
)

// patch is one change that Datastar makes to the page a request came from.
type patch struct {
	// selector of the element to change, in CSS. If empty, each of the elements replaces the element on
	// the page with the same ID.
	selector string

	// mode says where the elements go: "inner" replaces what is in the selected element, "prepend" puts
	// them first in it, and "append" last. If empty, the elements replace what they match.
	mode string

	// elements to put on the page.
	elements Node
}

// isDatastar reports whether Datastar sent the request, and so expects patches in answer, not a page.
func isDatastar(r *http.Request) bool {
	return r.Header.Get("Datastar-Request") == "true"
}

// writePatches as the answer to a request from Datastar.
func writePatches(w http.ResponseWriter, patches ...patch) error {
	events, err := encodePatches(patches)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/event-stream")
	_, err = w.Write(events)
	return err
}

// encodePatches as a stream of server-sent events, one datastar-patch-elements event for each patch.
//
// A line of a server-sent event ends at a line feed, a carriage return, or both. So that nothing in the
// elements can end a line early and start a field or an event of its own, every carriage return in them
// becomes a line feed first, which is what a browser reading HTML does with one anyway. Each line of the
// elements is then sent as a data line of its own.
func encodePatches(patches []patch) ([]byte, error) {
	var b bytes.Buffer
	for _, p := range patches {
		var elements strings.Builder
		if err := p.elements.Render(&elements); err != nil {
			return nil, err
		}

		b.WriteString("event: datastar-patch-elements\n")
		if p.selector != "" {
			b.WriteString("data: selector " + p.selector + "\n")
		}
		if p.mode != "" {
			b.WriteString("data: mode " + p.mode + "\n")
		}
		html := strings.ReplaceAll(strings.ReplaceAll(elements.String(), "\r\n", "\n"), "\r", "\n")
		for line := range strings.SplitSeq(html, "\n") {
			b.WriteString("data: elements " + line + "\n")
		}
		b.WriteString("\n")
	}
	return b.Bytes(), nil
}
