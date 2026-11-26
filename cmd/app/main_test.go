package main

import (
	"testing"

	"app/model"
)

func TestParseFirstAdmin(t *testing.T) {
	t.Run("parses the address as every address is parsed, and leaves it empty when the configuration names none", func(t *testing.T) {
		for input, expected := range map[string]model.EmailAddress{"": "", " Markus@Example.com ": "markus@example.com"} {
			got, err := parseFirstAdmin(input)
			if err != nil || got != expected {
				t.Fatalf("parsed %q as %q with %v", input, got, err)
			}
		}
	})

	t.Run("refuses what is not an address, without repeating it", func(t *testing.T) {
		_, err := parseFirstAdmin("markus at example.com")
		if err == nil || err.Error() != "FIRST_ADMIN_EMAIL is not an email address" {
			t.Fatalf("got %v", err)
		}
	})
}
