package http_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"maragu.dev/is"

	apphttp "app/http"
)

// hiddenQuery is what a query looks like once its values are hidden.
var hiddenQuery = regexp.MustCompile(`^((hidden|[A-Za-z][A-Za-z0-9_.-]{0,31}(=hidden)?)(&(hidden|[A-Za-z][A-Za-z0-9_.-]{0,31}(=hidden)?))*)?$`)

// startSpan with the given attributes through a provider wrapped by [apphttp.HideQueryValues], set the
// given attributes on it, and return what was recorded.
func startSpan(t *testing.T, start, set []attribute.KeyValue) []attribute.KeyValue {
	t.Helper()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := apphttp.HideQueryValues(tp).Tracer("test")

	ctx, span := tracer.Start(context.Background(), "GET", trace.WithAttributes(start...))
	trace.SpanFromContext(ctx).SetAttributes(set...)
	span.End()

	ended := sr.Ended()
	is.Equal(t, 1, len(ended))
	return ended[0].Attributes()
}

func TestHideQueryValues(t *testing.T) {
	t.Run("hides the values of the query, keeping the names, whether set at the start or later, and through the span in the context", func(t *testing.T) {
		attrs := startSpan(t,
			[]attribute.KeyValue{attribute.String("url.query", "token=secret"), attribute.String("url.full", "https://example.com/a?email=signe%40example.com&x"), attribute.String("url.path", "/sign-in/link")},
			[]attribute.KeyValue{attribute.String("url.query", "token=secret;name=Signe&Token=x&=y&after&"+strings.Repeat("s", 43)), attribute.Int("http.response.status_code", 405)},
		)

		got := map[attribute.Key]string{}
		for _, a := range attrs {
			got[a.Key] = a.Value.String()
			is.True(t, !strings.Contains(a.Value.String(), "secret") && !strings.Contains(a.Value.String(), "sss"), string(a.Key))
			is.True(t, !strings.Contains(a.Value.String(), "signe"), string(a.Key))
		}
		// A part without a value is kept as a name, unless it is longer than a name, such as a token alone.
		is.Equal(t, "token=hidden&name=hidden&Token=hidden&hidden&after&hidden", got["url.query"])
		is.Equal(t, "https://example.com/a?email=hidden&x", got["url.full"])
		is.Equal(t, "/sign-in/link", got["url.path"])
		is.Equal(t, "405", got["http.response.status_code"])
	})

	t.Run("leaves spans without a query as they are", func(t *testing.T) {
		attrs := startSpan(t, []attribute.KeyValue{attribute.String("url.full", "https://example.com/a")}, []attribute.KeyValue{attribute.String("user.id", "u_1")})
		is.Equal(t, 2, len(attrs))
		is.Equal(t, "https://example.com/a", attrs[0].Value.String())
		is.Equal(t, "u_1", attrs[1].Value.String())
	})
}

func FuzzHideQueryValues(f *testing.F) {
	f.Add("token=AAAA")
	f.Add("token=a;b=c&&d")
	f.Add("=x")
	f.Add("%zz=1&a")
	f.Add("")

	f.Fuzz(func(t *testing.T, q string) {
		attrs := startSpan(t, nil, []attribute.KeyValue{attribute.String("url.query", q)})
		hidden := attrs[0].Value.String()
		if !hiddenQuery.MatchString(hidden) {
			t.Fatalf("hid %q as %q", q, hidden)
		}
	})
}
