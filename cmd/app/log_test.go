package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestWithoutRecipients(t *testing.T) {
	t.Run("leaves out the attributes glue's email sender logs an address under, and keeps the rest", func(t *testing.T) {
		var b bytes.Buffer
		log := slog.New(withoutRecipients(slog.NewTextHandler(&b, nil))).With("component", "email.Sender", "recipient", "signe@example.com")

		log.Info("Not sending email, recipient is inactive", "recipient", `"Signe" <signe@example.com>`, "status code", 422)
		log.Info("Error sending email, got http status code", "body", "Invalid 'To' address: signe@example.com", "message", "signe@example.com")
		log.WithGroup("g").Info("In a group", "recipient", "signe@example.com", "kept", "yes")

		is := func(ok bool, msg string) {
			t.Helper()
			if !ok {
				t.Fatal(msg, b.String())
			}
		}
		is(!strings.Contains(b.String(), "signe@example.com"), "the logs hold the address")
		is(strings.Contains(b.String(), "component=email.Sender"), "the logs lost the component")
		is(strings.Contains(b.String(), "\"status code\"=422"), "the logs lost the status code")
		is(strings.Contains(b.String(), "g.kept=yes"), "the logs lost what was in a group")
		is(strings.Count(b.String(), "\n") == 3, "expected three lines")
	})
}
