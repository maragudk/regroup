package model

import (
	"encoding/base64"
	"fmt"
	"io"
)

// FormToken of a form on a page. Each form that makes something carries a new one, and sends it along, so
// that sending the same form again, such as after an answer that was lost, does not make the same thing
// twice.
type FormToken string

// String satisfies [fmt.Stringer].
func (t FormToken) String() string {
	return string(t)
}

var _ fmt.Stringer = FormToken("")

// NewFormToken from 16 bytes of the given randomness, encoded as 22 URL-safe characters.
func NewFormToken(random io.Reader) (FormToken, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	return FormToken(base64.RawURLEncoding.EncodeToString(b)), nil
}

// ParseFormToken from what [NewFormToken] made. Anything else is [ErrorFormTokenInvalid].
func ParseFormToken(s string) (FormToken, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != 16 {
		return "", ErrorFormTokenInvalid
	}

	// The decoder skips line breaks, so only the canonical encoding is accepted.
	if base64.RawURLEncoding.EncodeToString(b) != s {
		return "", ErrorFormTokenInvalid
	}

	return FormToken(s), nil
}
