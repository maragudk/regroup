package main

import (
	"context"
	"log/slog"
	"slices"
)

// recipientKeys are the attributes that glue's email sender logs a recipient's address under: the
// address itself, and what Postmark answered, which can repeat it.
var recipientKeys = []string{"recipient", "body", "message"}

// withoutRecipients is a [slog.Handler] that leaves out the attributes that glue's email sender may log an
// email address under, and hands everything else to the given handler. No log line may hold an address.
func withoutRecipients(h slog.Handler) slog.Handler {
	return &recipientHider{h: h}
}

type recipientHider struct {
	h slog.Handler
}

func (r *recipientHider) Enabled(ctx context.Context, level slog.Level) bool {
	return r.h.Enabled(ctx, level)
}

func (r *recipientHider) Handle(ctx context.Context, rec slog.Record) error {
	kept := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		if !slices.Contains(recipientKeys, a.Key) {
			kept.AddAttrs(a)
		}
		return true
	})
	return r.h.Handle(ctx, kept)
}

func (r *recipientHider) WithAttrs(attrs []slog.Attr) slog.Handler {
	attrs = slices.DeleteFunc(slices.Clone(attrs), func(a slog.Attr) bool {
		return slices.Contains(recipientKeys, a.Key)
	})
	return &recipientHider{h: r.h.WithAttrs(attrs)}
}

func (r *recipientHider) WithGroup(name string) slog.Handler {
	return &recipientHider{h: r.h.WithGroup(name)}
}
