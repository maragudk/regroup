package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// PostID of a [Post].
type PostID string

// String satisfies [fmt.Stringer].
func (i PostID) String() string {
	return string(i)
}

// CommentID of a [Comment].
type CommentID string

// String satisfies [fmt.Stringer].
func (i CommentID) String() string {
	return string(i)
}

// Post on the group's timeline. It has no title, only a body of plain text.
type Post struct {
	ID       PostID
	Created  Time
	AuthorID UserID `db:"author_id"`
	Body     string
}

// Comment on a [Post]. Comments are a flat list.
type Comment struct {
	ID       CommentID
	Created  Time
	PostID   PostID `db:"post_id"`
	AuthorID UserID `db:"author_id"`
	Body     string
}

const (
	// PostBodyMaxLength in characters (runes), after cleaning with [CleanPostBody].
	PostBodyMaxLength = 10_000

	// CommentBodyMaxLength in characters (runes), after cleaning with [CleanCommentBody].
	CommentBodyMaxLength = 2_000
)

// CleanPostBody for storing, or return why it can't be stored. A body is plain text:
//   - It must be valid UTF-8, or [ErrorBodyInvalid].
//   - Line breaks are kept, and "\r\n" and a lone "\r" become "\n".
//   - Whitespace at the start and the end is trimmed.
//   - What is left must hold at least one visible character, or [ErrorBodyEmpty]. See [visible].
//   - It must contain no control characters other than line breaks and tabs, or [ErrorBodyInvalid].
//   - It must be at most [PostBodyMaxLength] characters (runes), or [ErrorBodyTooLong].
func CleanPostBody(body string) (string, error) {
	return cleanBody(body, PostBodyMaxLength)
}

// CleanCommentBody for storing, or return why it can't be stored. The rules are those of [CleanPostBody],
// with a maximum of [CommentBodyMaxLength] characters (runes).
func CleanCommentBody(body string) (string, error) {
	return cleanBody(body, CommentBodyMaxLength)
}

// cleanBody by the rules of [CleanPostBody], with the given maximum length.
func cleanBody(body string, maxLength int) (string, error) {
	if !utf8.ValidString(body) {
		return "", ErrorBodyInvalid
	}

	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.TrimSpace(body)

	if !strings.ContainsFunc(body, visible) {
		return "", ErrorBodyEmpty
	}

	for _, r := range body {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", ErrorBodyInvalid
		}
	}

	if utf8.RuneCountInString(body) > maxLength {
		return "", ErrorBodyTooLong
	}

	return body, nil
}

// visible reports whether a character is drawn as something a reader can see. These are not:
//   - White space.
//   - Default-ignorable code points, which fonts draw as nothing: format characters such as a zero-width
//     space or a tag character, variation selectors, the Hangul fillers, the combining grapheme joiner,
//     and the code points Unicode keeps for more of these.
//   - Other format characters (Unicode category Cf), except the prepended concatenation marks, such as the
//     Arabic number sign, which are drawn.
//   - The Braille pattern blank, which is drawn as nothing though Unicode does not class it as ignorable.
//
// Everything else is visible, including a combining mark on its own, a character for private use, and a
// control character, which [cleanBody] refuses for another reason.
func visible(r rune) bool {
	switch {
	case unicode.In(r, unicode.White_Space, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point):
		return false
	case unicode.Is(unicode.Cf, r):
		return unicode.Is(unicode.Prepended_Concatenation_Mark, r)
	default:
		return r != 0x2800 // BRAILLE PATTERN BLANK
	}
}
