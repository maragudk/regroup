package model_test

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"strings"
	"testing"

	"hegel.dev/go/hegel"
	"maragu.dev/is"

	"app/model"
)

func TestNewFormToken(t *testing.T) {
	t.Run("makes a token of 22 URL-safe characters from 16 random bytes, which parses back as itself", func(t *testing.T) {
		token, err := model.NewFormToken(bytes.NewReader(bytes.Repeat([]byte{0xab}, 16)))
		is.NotError(t, err)
		is.Equal(t, model.FormToken("q6urq6urq6urq6urq6urqw"), token)

		parsed, err := model.ParseFormToken(token.String())
		is.NotError(t, err)
		is.Equal(t, token, parsed)
	})

	t.Run("fails when the randomness runs out", func(t *testing.T) {
		_, err := model.NewFormToken(bytes.NewReader(make([]byte, 15)))
		is.True(t, err != nil, "no error")
	})

	t.Run("makes tokens that parse back as themselves, from any randomness", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			random := hegel.Draw(ht, hegel.Binary(16, 16))

			token, err := model.NewFormToken(bytes.NewReader(random))
			if err != nil {
				ht.Fatalf("making a token from %x failed: %v", random, err)
			}
			parsed, err := model.ParseFormToken(token.String())
			if err != nil || parsed != token {
				ht.Fatalf("token %q parsed as %q, %v", token, parsed, err)
			}
		})
	})

	t.Run("makes a different token each time from good randomness", func(t *testing.T) {
		random := rand.NewChaCha8([32]byte{})
		seen := map[model.FormToken]bool{}
		for range 1000 {
			token, err := model.NewFormToken(random)
			is.NotError(t, err)
			is.True(t, !seen[token], token.String())
			seen[token] = true
		}
	})
}

func TestParseFormToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "an empty token", input: ""},
		{name: "a token that is too short", input: "q6urq6urq6urq6urq6urq"},
		{name: "a token that is too long", input: "q6urq6urq6urq6urq6urqwA"},
		{name: "a token with padding", input: "q6urq6urq6urq6urq6urqw=="},
		{name: "a token with a line break in it", input: "q6urq6urq6u\nrq6urq6urqw"},
		{name: "a token that is not the only encoding of its bytes", input: "q6urq6urq6urq6urq6urqx"},
		{name: "a token in the standard alphabet instead of the URL-safe one", input: "+6urq6urq6urq6urq6urqw"},
		{name: "a cursor", input: "AQAAAZvNMb8AAAAAAAAAACo"},
	}

	for _, test := range tests {
		t.Run("refuses "+test.name, func(t *testing.T) {
			token, err := model.ParseFormToken(test.input)
			is.Error(t, model.ErrorFormTokenInvalid, err)
			is.Equal(t, model.FormToken(""), token)
		})
	}

	t.Run("accepts a token in the URL-safe alphabet", func(t *testing.T) {
		token, err := model.ParseFormToken("-_urq6urq6urq6urq6urqw")
		is.NotError(t, err)
		is.Equal(t, model.FormToken("-_urq6urq6urq6urq6urqw"), token)
	})
}

func FuzzParseFormToken(f *testing.F) {
	f.Add("")
	f.Add("q6urq6urq6urq6urq6urqw")
	f.Add("q6urq6urq6urq6urq6urqx")
	f.Add("q6urq6urq6u\nrq6urq6urqw")
	f.Add("q6urq6urq6urq6urq6urqw==")
	f.Add("not a token")

	f.Fuzz(func(t *testing.T, s string) {
		token, err := model.ParseFormToken(s)
		if err != nil {
			if !errors.Is(err, model.ErrorFormTokenInvalid) {
				t.Fatalf("unexpected error %v", err)
			}
			if token != "" {
				t.Fatalf("got token %q along with an error", token)
			}
			return
		}

		// Whatever is accepted is a token that could have been made, and the only encoding of it.
		if token.String() != s || len(s) != 22 || strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
			t.Fatalf("accepted %q", s)
		}
	})
}
