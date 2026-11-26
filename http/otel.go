package http

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

// HideQueryValues wraps the given provider, so that the spans of every tracer it gives record the
// query of a URL with the names of its parameters only, and every value hidden: in the url.query
// attribute, and in the query of the url.full attribute. A link in an email carries a secret in its
// query, and a form sent with its fields in the query can carry an email address, so neither may reach
// the telemetry, whether the request matched a route or not.
//
// Attributes given when a span starts are given again hidden, which replaces them in the span, though a
// sampler sees both.
func HideQueryValues(tp trace.TracerProvider) trace.TracerProvider {
	return &queryHidingProvider{tp: tp}
}

type queryHidingProvider struct {
	embedded.TracerProvider
	tp trace.TracerProvider
}

func (p *queryHidingProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return &queryHidingTracer{t: p.tp.Tracer(name, opts...), p: p}
}

type queryHidingTracer struct {
	embedded.Tracer
	t trace.Tracer
	p *queryHidingProvider
}

func (t *queryHidingTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	config := trace.NewSpanStartConfig(opts...)
	if hidden, ok := hideQueries(config.Attributes()); ok {
		opts = append(opts, trace.WithAttributes(hidden...))
	}
	ctx, span := t.t.Start(ctx, name, opts...)
	s := &queryHidingSpan{Span: span, p: t.p}
	return trace.ContextWithSpan(ctx, s), s
}

type queryHidingSpan struct {
	trace.Span
	p *queryHidingProvider
}

func (s *queryHidingSpan) SetAttributes(kv ...attribute.KeyValue) {
	if hidden, ok := hideQueries(kv); ok {
		kv = hidden
	}
	s.Span.SetAttributes(kv...)
}

func (s *queryHidingSpan) TracerProvider() trace.TracerProvider {
	return s.p
}

// hideQueries in the given attributes, and report whether any was there to hide.
func hideQueries(kv []attribute.KeyValue) ([]attribute.KeyValue, bool) {
	var found bool
	hidden := make([]attribute.KeyValue, 0, len(kv))
	for _, a := range kv {
		switch a.Key {
		case "url.query":
			found = true
			a = attribute.String(string(a.Key), hideQuery(a.Value.String()))
		case "url.full":
			if u, err := url.Parse(a.Value.String()); err == nil && u.RawQuery == "" {
				break
			} else if err == nil {
				u.RawQuery = hideQuery(u.RawQuery)
				a = attribute.String(string(a.Key), u.String())
			} else {
				a = attribute.String(string(a.Key), "hidden")
			}
			found = true
		}
		hidden = append(hidden, a)
	}
	return hidden, found
}

// parameterName that is kept in a hidden query, such as "after" or "token". Anything else could be a
// value without a name, and is hidden too.
var parameterName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,31}$`)

// hideQuery with the names of its parameters only, in the order they came, and "hidden" for each value,
// and for each part of it that is not a parameter with a name.
func hideQuery(q string) string {
	if q == "" {
		return ""
	}
	parts := strings.FieldsFunc(q, func(r rune) bool { return r == '&' || r == ';' })
	for i, part := range parts {
		name, _, hasValue := strings.Cut(part, "=")
		switch {
		case !parameterName.MatchString(name):
			parts[i] = "hidden"
		case hasValue:
			parts[i] = name + "=hidden"
		default:
			parts[i] = name
		}
	}
	return strings.Join(parts, "&")
}
