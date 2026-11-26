// Package servicetest provides testing helpers for the service package.
package servicetest

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"maragu.dev/glue/email"

	"app/service"
)

// NewFat for testing, with a logger of its own.
//
// No operation is wired: a test wires the ones it exercises, with the capabilities it is already
// asserting against.
func NewFat(t *testing.T) *service.Fat {
	t.Helper()

	return service.NewFat(service.NewFatOptions{
		Log: slog.New(slog.NewTextHandler(&testWriter{t: t}, nil)),
	})
}

// testWriter logs through the test it belongs to, so an operation says what it did under the test
// that ran it and stays quiet unless that test fails.
type testWriter struct {
	t *testing.T
}

func (t *testWriter) Write(p []byte) (n int, err error) {
	t.t.Log(string(p))
	return len(p), nil
}

// Sender of emails for testing, which keeps what it is asked to send. It is safe for concurrent use.
type Sender struct {
	mu   sync.Mutex
	sent []email.SendOptions
	err  error
}

// SendTransactional satisfies [email.Sender], and returns the error [Sender.Fail] set, if any.
func (s *Sender) SendTransactional(ctx context.Context, opts email.SendOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return s.err
	}
	s.sent = append(s.sent, opts)
	return nil
}

// Fail every email sent after this with the given error, or none if it is nil.
func (s *Sender) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.err = err
}

// Take the emails sent since the last time, in the order they were sent.
func (s *Sender) Take() []email.SendOptions {
	s.mu.Lock()
	defer s.mu.Unlock()

	sent := s.sent
	s.sent = nil
	return sent
}

var _ email.Sender = (*Sender)(nil)
