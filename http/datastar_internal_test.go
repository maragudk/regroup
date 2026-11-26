package http

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"hegel.dev/go/hegel"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
	"maragu.dev/is"
)

// DecodedPatch is a patch as Datastar reads it from a stream of server-sent events.
type DecodedPatch struct {
	Selector string
	Mode     string
	Elements string
}

// DecodePatches from a stream of server-sent events, by the rules a browser reads one with: a line ends
// at a line feed, a carriage return, or both; a field is the text before the first colon, and its value
// what comes after it, less one leading space; and an empty line ends an event. Each event must be a
// datastar-patch-elements event, with nothing in its data but a selector, a mode, and elements.
func DecodePatches(stream string) ([]DecodedPatch, error) {
	var patches []DecodedPatch
	var event string
	var data []string
	var open bool

	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(stream, "\r\n", "\n"), "\r", "\n"), "\n")
	for _, line := range lines {
		if line == "" {
			if !open {
				continue
			}
			if event != "datastar-patch-elements" {
				return nil, fmt.Errorf("unexpected event %q", event)
			}

			var p DecodedPatch
			var elements []string
			for _, d := range data {
				key, value, _ := strings.Cut(d, " ")
				switch key {
				case "selector":
					p.Selector = value
				case "mode":
					p.Mode = value
				case "elements":
					elements = append(elements, value)
				default:
					return nil, fmt.Errorf("unexpected data line %q", d)
				}
			}
			p.Elements = strings.Join(elements, "\n")
			patches = append(patches, p)
			event, data, open = "", nil, false
			continue
		}

		open = true
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		default:
			return nil, fmt.Errorf("unexpected line %q", line)
		}
	}
	if open {
		return nil, fmt.Errorf("the stream ends in the middle of an event")
	}
	return patches, nil
}

func TestEncodePatches(t *testing.T) {
	t.Run("writes each patch as an event with its selector, its mode, and its elements line by line", func(t *testing.T) {
		events, err := encodePatches([]patch{
			{elements: P(ID("a"), Text("one\ntwo"))},
			{selector: "#timeline", mode: "prepend", elements: P(Text("three"))},
		})
		is.NotError(t, err)

		expected := "event: datastar-patch-elements\n" +
			"data: elements <p id=\"a\">one\n" +
			"data: elements two</p>\n" +
			"\n" +
			"event: datastar-patch-elements\n" +
			"data: selector #timeline\n" +
			"data: mode prepend\n" +
			"data: elements <p>three</p>\n" +
			"\n"
		is.Equal(t, expected, string(events))
	})

	t.Run("keeps a carriage return in the text from starting a line of its own", func(t *testing.T) {
		events, err := encodePatches([]patch{{elements: P(Text("a\revent: datastar-patch-elements\rdata: elements <script>alert(1)</script>\r\rb"))}})
		is.NotError(t, err)
		is.True(t, !strings.Contains(string(events), "\r"), string(events))

		patches, err := DecodePatches(string(events))
		is.NotError(t, err)
		is.Equal(t, 1, len(patches))
		is.Equal(t, "<p>a\nevent: datastar-patch-elements\ndata: elements &lt;script&gt;alert(1)&lt;/script&gt;\n\nb</p>", patches[0].Elements)
	})

	t.Run("refuses a selector or a mode with a line break in it, which would start a line of its own", func(t *testing.T) {
		for _, p := range []patch{
			{selector: "#a\ndata: mode inner", elements: P()},
			{selector: "#a\revent: x", elements: P()},
			{selector: "#a", mode: "append\ndata: selector body", elements: P()},
			{mode: "append\r", elements: P()},
		} {
			_, err := encodePatches([]patch{p})
			is.True(t, err != nil, "expected an error for "+p.selector+p.mode)
		}
	})

	t.Run("any text in a patch comes out as that text in one element of one event, whatever it holds", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			text := hegel.Draw(ht, hostileText())
			selector := hegel.Draw(ht, hegel.SampledFrom([]string{"", "#timeline", "#day-2031-03-14-posts"}))
			mode := hegel.Draw(ht, hegel.SampledFrom([]string{"", "inner", "prepend", "append"}))

			events, err := encodePatches([]patch{{selector: selector, mode: mode, elements: P(Text(text))}})
			if err != nil {
				ht.Fatalf("encoding %q: %v", text, err)
			}

			patches, err := DecodePatches(string(events))
			if err != nil {
				ht.Fatalf("decoding the events for %q: %v\n%s", text, err, events)
			}
			if len(patches) != 1 {
				ht.Fatalf("expected one patch for %q, got %v", text, len(patches))
			}
			if patches[0].Selector != selector || patches[0].Mode != mode {
				ht.Fatalf("expected selector %q and mode %q for %q, got %q and %q", selector, mode, text, patches[0].Selector, patches[0].Mode)
			}

			nodes, err := html.ParseFragment(strings.NewReader(patches[0].Elements), &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"})
			if err != nil {
				ht.Fatalf("parsing the elements for %q: %v", text, err)
			}
			if len(nodes) != 1 || nodes[0].Type != html.ElementNode || nodes[0].Data != "p" {
				ht.Fatalf("expected one p element for %q, got %q", text, patches[0].Elements)
			}

			// A browser reading HTML turns every carriage return into a line feed too, and the text of an
			// element is written with the replacement character for a NUL.
			expected := strings.NewReplacer("\r\n", "\n", "\r", "\n", "\x00", "\ufffd").Replace(text)
			var got string
			for c := nodes[0].FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.TextNode {
					ht.Fatalf("%q put something other than text on the page: %q", text, patches[0].Elements)
				}
				got += c.Data
			}
			if got != expected {
				ht.Fatalf("expected the text %q, got %q", expected, got)
			}
		})
	})
}

// hostileText made of pieces that try to end a line, start a field or an event, or open an element,
// mixed with any other text.
func hostileText() hegel.Generator[string] {
	piece := hegel.OneOf(
		hegel.SampledFrom([]string{
			"\r", "\n", "\r\n", "\n\n", "\r\r", " ", ":", "event: datastar-patch-elements", "data: elements <b>bold</b>",
			"data: selector body", "data: mode inner", "id: 1", "retry: 1", ": comment",
			"<script>alert(1)</script>", "<img src=x onerror=alert(1)>", "</p>", "<p>", "&amp;", "&", "<", ">", "\"", "'",
			"\u2028", "\u2029", "\u0085", "\ufeff", "\x00", "\v", "\f",
		}),
		hegel.Text(),
	)

	return hegel.Composite(func(tc hegel.TestCase) string {
		return strings.Join(hegel.Draw(tc, hegel.Lists(piece)), "")
	})
}

func TestWritePatches(t *testing.T) {
	t.Run("answers with a stream of server-sent events", func(t *testing.T) {
		res := httptest.NewRecorder()

		err := writePatches(res, patch{elements: P(ID("a"), Text("one"))})
		is.NotError(t, err)
		is.Equal(t, "text/event-stream", res.Header().Get("Content-Type"))

		patches, err := DecodePatches(res.Body.String())
		is.NotError(t, err)
		is.Equal(t, 1, len(patches))
		is.Equal(t, `<p id="a">one</p>`, patches[0].Elements)
	})
}

func TestIsDatastar(t *testing.T) {
	t.Run("is true only for a request with Datastar's header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/posts", nil)
		is.True(t, !isDatastar(req))

		req.Header.Set("Datastar-Request", "true")
		is.True(t, isDatastar(req))
	})
}
