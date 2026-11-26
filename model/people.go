package model

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// EmailAddressMaxLength in bytes, which is the longest address that can be sent to (RFC 5321).
const EmailAddressMaxLength = 254

// ParseEmailAddress that a person typed, or return [ErrorEmailInvalid]. One rule decides what is accepted
// and when two spellings are the same address:
//   - White space at the start and the end is trimmed.
//   - The address is lowercased, so two spellings that differ only in case are the same address. The
//     lowercased address is what is stored and compared.
//   - What is left must be a valid email address by the definition HTML forms use (WHATWG): a local part
//     of ASCII letters, digits, and the characters .!#$%&'*+/=?^_`{|}~- , an @, and a domain of at least
//     two labels of ASCII letters, digits, and hyphens. Addresses with other characters, such as
//     internationalised ones, are refused.
//   - It must be at most [EmailAddressMaxLength] bytes.
//
// Nothing checks that a mailbox exists.
func ParseEmailAddress(s string) (EmailAddress, error) {
	e := EmailAddress(strings.ToLower(strings.TrimSpace(s)))
	if len(e) > EmailAddressMaxLength || !e.IsValid() {
		return "", ErrorEmailInvalid
	}
	return e, nil
}

// NameMaxLength in characters (runes), after cleaning with [CleanName].
const NameMaxLength = 50

// CleanName that a person chose for the group to see, or return why it can't be used. A name is plain
// text on one line, cleaned as a post's body is (see [CleanPostBody]):
//   - It must be valid UTF-8, or [ErrorNameInvalid].
//   - White space at the start and the end is trimmed.
//   - What is left must hold at least one visible character, or [ErrorNameEmpty], by the rule post bodies
//     follow.
//   - It must contain no control characters, line breaks and tabs included, no line or paragraph
//     separators, and no characters that change the direction of the text around them (the bidirectional
//     embeddings, overrides, and isolates), or [ErrorNameInvalid]. A name goes into sentences and email
//     subjects, where these would break the line or turn the text after the name around.
//   - It must be at most [NameMaxLength] characters (runes), or [ErrorNameTooLong].
//
// Names need not be unique.
func CleanName(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", ErrorNameInvalid
	}

	name = strings.TrimSpace(name)

	if !strings.ContainsFunc(name, visible) {
		return "", ErrorNameEmpty
	}

	for _, r := range name {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Bidi_Control) {
			return "", ErrorNameInvalid
		}
	}

	if utf8.RuneCountInString(name) > NameMaxLength {
		return "", ErrorNameTooLong
	}

	return name, nil
}

// LinkToken is the secret in a link in an email: a sign-in link or an invitation. Only its [LinkToken.Hash]
// is stored, so a copy of the database signs nobody in.
type LinkToken string

// String satisfies [fmt.Stringer].
func (t LinkToken) String() string {
	return string(t)
}

var _ fmt.Stringer = LinkToken("")

// Hash of the token, which is what is stored and looked up: SHA-256, in lowercase hexadecimal. The token
// holds 256 random bits, so the hash needs no salt.
func (t LinkToken) Hash() string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// NewLinkToken from 32 bytes of the given randomness, encoded as 43 URL-safe characters.
func NewLinkToken(random io.Reader) (LinkToken, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	return LinkToken(base64.RawURLEncoding.EncodeToString(b)), nil
}

// ParseLinkToken from what [NewLinkToken] made. Anything else is [ErrorLinkInvalid], the one refusal
// for a link that does not work, whatever the reason.
func ParseLinkToken(s string) (LinkToken, error) {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil || len(b) != 32 {
		return "", ErrorLinkInvalid
	}

	// The decoder skips line breaks, so only the canonical encoding is accepted.
	if base64.RawURLEncoding.EncodeToString(b) != s {
		return "", ErrorLinkInvalid
	}

	return LinkToken(s), nil
}

const (
	// SignInLinkLifetime is how long a sign-in link works after it is sent, once.
	SignInLinkLifetime = 15 * time.Minute

	// SignInLinksPerHour is how many sign-in links are sent to one address in an hour at most.
	SignInLinksPerHour = 5

	// InvitationLifetime is how long an invitation works after it is sent, once.
	InvitationLifetime = 7 * 24 * time.Hour
)

// SignInLink that was sent to an email address. Its token is not kept, only the token's hash.
type SignInLink struct {
	Hash    string
	Created Time
	Expires Time
	Email   EmailAddress

	// Used is when the link was used up, by signing in with it or with another link to the same address,
	// or nil if it was not.
	Used *Time
}

// SignInLinkCheck is what looking at a sign-in link tells, without using it up.
type SignInLinkCheck struct {
	// Joining is whether signing in with the link makes the person join the group, as the first admin, so
	// that they have to choose their name first.
	Joining bool
}

// InvitationID of an [Invitation].
type InvitationID string

// String satisfies [fmt.Stringer].
func (i InvitationID) String() string {
	return string(i)
}

// Invitation to the group that is waiting to be accepted.
type Invitation struct {
	ID      InvitationID
	Created Time
	Expires Time

	// Email address it was sent to.
	Email EmailAddress

	// InvitedBy is who sent it, and InvitedByName their name now.
	InvitedBy     UserID `db:"invited_by"`
	InvitedByName string `db:"invited_by_name"`
}

// InvitationCheck is what looking at an invitation's link tells, without using it up.
type InvitationCheck struct {
	// InvitedByName is the name of the person who sent the invitation, as it is now.
	InvitedByName string

	// Expires is when the invitation stops working.
	Expires Time
}

// Member of the group, as the list of members shows them.
type Member struct {
	ID   UserID
	Name string
	Role Role

	// Joined is when the person joined the group.
	Joined Time

	// Email address of the member, which is empty unless the person reading the list may see it, by
	// [ActionSeeEmailAddresses].
	Email EmailAddress
}
