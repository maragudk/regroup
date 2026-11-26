package main

import (
	"context"
	"log/slog"
	"strings"
)

// withoutAddresses is a [slog.Handler] that hides the value of every attribute that could hold an email
// address, because it holds an @, and hands the rest to the given handler unchanged. No log line may
// hold an address, and a logger given to a dependency can log whatever it is handed.
func withoutAddresses(h slog.Handler) slog.Handler {
	return &addressHider{h: h}
}

type addressHider struct {
	h slog.Handler
}

func (a *addressHider) Enabled(ctx context.Context, level slog.Level) bool {
	return a.h.Enabled(ctx, level)
}

func (a *addressHider) Handle(ctx context.Context, rec slog.Record) error {
	kept := slog.NewRecord(rec.Time, rec.Level, hideAddress(rec.Message), rec.PC)
	rec.Attrs(func(attr slog.Attr) bool {
		kept.AddAttrs(hideAddresses(attr))
		return true
	})
	return a.h.Handle(ctx, kept)
}

func (a *addressHider) WithAttrs(attrs []slog.Attr) slog.Handler {
	hidden := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		hidden = append(hidden, hideAddresses(attr))
	}
	return &addressHider{h: a.h.WithAttrs(hidden)}
}

func (a *addressHider) WithGroup(name string) slog.Handler {
	return &addressHider{h: a.h.WithGroup(name)}
}

// hideAddresses in the attribute's value, and in the attributes of a group.
func hideAddresses(attr slog.Attr) slog.Attr {
	v := attr.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		var attrs []any
		for _, a := range v.Group() {
			attrs = append(attrs, hideAddresses(a))
		}
		return slog.Group(attr.Key, attrs...)
	}
	return slog.String(attr.Key, hideAddress(v.String()))
}

// hideAddress is "hidden" if the text could hold an address, and the text otherwise.
func hideAddress(s string) string {
	if strings.Contains(s, "@") {
		return "hidden"
	}
	return s
}
