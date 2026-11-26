package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maragu.dev/glue/email"
	"maragu.dev/glue/email/postmark"

	"app/html"
)

func TestWithoutAddresses(t *testing.T) {
	t.Run("hides every value that could hold an address, in groups and in attributes given before, and keeps the rest", func(t *testing.T) {
		var b bytes.Buffer
		log := slog.New(withoutAddresses(slog.NewTextHandler(&b, nil))).With("component", "email.Sender", "to", "signe@example.com")

		log.Info("Not sending email", "recipient", `"Signe" <signe@example.com>`, "status code", 422)
		log.Info("Sent to signe@example.com", slog.Group("g", "address", "signe@example.com", "kept", "yes"))

		got := b.String()
		if strings.Contains(got, "signe@example.com") {
			t.Fatal("the logs hold the address:", got)
		}
		for _, kept := range []string{"component=email.Sender", `"status code"=422`, "g.kept=yes", "recipient=hidden", "msg=hidden"} {
			if !strings.Contains(got, kept) {
				t.Fatalf("the logs lost %v: %v", kept, got)
			}
		}
	})

	t.Run("keeps addresses out of what glue's email sender logs when Postmark refuses an email or fails", func(t *testing.T) {
		for _, answer := range []struct {
			status int
			body   string
		}{
			{status: 422, body: `{"ErrorCode":406,"Message":"Address signe@example.com is inactive."}`},
			{status: 422, body: `{"ErrorCode":300,"Message":"Invalid 'To' address: 'signe@example.com'."}`},
			{status: 500, body: "Could not send to signe@example.com"},
		} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(answer.status)
				_, _ = io.WriteString(w, answer.body)
			}))
			var b bytes.Buffer
			sender := postmark.NewSender(postmark.NewSenderOptions{
				BaseURL:                   "https://loft.example.com",
				EndpointURL:               server.URL,
				Emails:                    html.EmailTemplates(),
				Log:                       slog.New(withoutAddresses(slog.NewTextHandler(&b, nil))),
				ReplyToEmailAddress:       "reply@example.com",
				TransactionalEmailAddress: "from@example.com",
			})

			_ = sender.SendTransactional(t.Context(), email.SendOptions{Template: "sign-in", To: "signe@example.com", ToName: "Signe", Subject: "Sign in"})
			server.Close()

			if b.Len() == 0 {
				t.Fatal("expected the sender to log something for", answer.body)
			}
			if strings.Contains(b.String(), "signe@example.com") {
				t.Fatal("the logs hold the address:", b.String())
			}
		}
	})
}
