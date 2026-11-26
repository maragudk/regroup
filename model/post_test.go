package model_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"hegel.dev/go/hegel"
	"maragu.dev/is"

	"app/model"
)

func TestCleanPostBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		err      error
	}{
		{name: "keeps plain text as it is", input: "Cake in the kitchen", expected: "Cake in the kitchen"},
		{name: "trims whitespace at the start and the end", input: " \n\t Cake \n\n", expected: "Cake"},
		{name: "keeps line breaks inside", input: "Cake\n\nin the kitchen", expected: "Cake\n\nin the kitchen"},
		{name: "turns carriage return and line feed into a line feed", input: "Cake\r\nin the kitchen", expected: "Cake\nin the kitchen"},
		{name: "turns a lone carriage return into a line feed", input: "Cake\rin the kitchen", expected: "Cake\nin the kitchen"},
		{name: "keeps tabs inside", input: "Cake\tplease", expected: "Cake\tplease"},
		{name: "keeps HTML as text", input: "<script>alert(1)</script>", expected: "<script>alert(1)</script>"},
		{name: "accepts exactly the maximum length", input: strings.Repeat("æ", model.PostBodyMaxLength), expected: strings.Repeat("æ", model.PostBodyMaxLength)},
		{name: "refuses an empty body", input: "", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only whitespace", input: " \r\n\t\u00a0 ", err: model.ErrorBodyEmpty},
		{name: "refuses a body one character too long", input: strings.Repeat("æ", model.PostBodyMaxLength+1), err: model.ErrorBodyTooLong},
		{name: "counts the length after trimming", input: " " + strings.Repeat("a", model.PostBodyMaxLength) + " ", expected: strings.Repeat("a", model.PostBodyMaxLength)},
		{name: "refuses invalid UTF-8", input: "Cake \xff", err: model.ErrorBodyInvalid},
		{name: "refuses a control character inside", input: "Cake\x00please", err: model.ErrorBodyInvalid},
	}

	for _, test := range tests {
		t.Run("should "+test.name, func(t *testing.T) {
			body, err := model.CleanPostBody(test.input)
			if test.err != nil {
				is.Error(t, test.err, err)
				is.Equal(t, "", body)
				return
			}
			is.NotError(t, err)
			is.Equal(t, test.expected, body)
		})
	}

	t.Run("cleans any text so it is trimmed, within the maximum, and free of carriage returns, or says why not", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := hegel.Draw(ht, bodyText(model.PostBodyMaxLength))

			body, err := model.CleanPostBody(input)
			checkCleanedBody(ht, input, body, err, model.PostBodyMaxLength)
		})
	})

	t.Run("cleaning a cleaned body changes nothing", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := hegel.Draw(ht, bodyText(model.PostBodyMaxLength))

			body, err := model.CleanPostBody(input)
			ht.Assume(err == nil)

			again, err := model.CleanPostBody(body)
			if err != nil {
				ht.Fatalf("cleaning %q again failed: %v", body, err)
			}
			if again != body {
				ht.Fatalf("cleaning %q again gave %q", body, again)
			}
		})
	})
}

func TestCleanCommentBody(t *testing.T) {
	t.Run("accepts exactly the maximum length", func(t *testing.T) {
		body, err := model.CleanCommentBody(strings.Repeat("æ", model.CommentBodyMaxLength))
		is.NotError(t, err)
		is.Equal(t, model.CommentBodyMaxLength, utf8.RuneCountInString(body))
	})

	t.Run("refuses one character more than the maximum, which is smaller than a post's", func(t *testing.T) {
		is.True(t, model.CommentBodyMaxLength < model.PostBodyMaxLength)

		_, err := model.CleanCommentBody(strings.Repeat("æ", model.CommentBodyMaxLength+1))
		is.Error(t, model.ErrorBodyTooLong, err)
	})

	t.Run("refuses a body of only whitespace", func(t *testing.T) {
		_, err := model.CleanCommentBody("\n \n")
		is.Error(t, model.ErrorBodyEmpty, err)
	})

	t.Run("cleans any text so it is trimmed, within the maximum, and free of carriage returns, or says why not", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := hegel.Draw(ht, bodyText(model.CommentBodyMaxLength))

			body, err := model.CleanCommentBody(input)
			checkCleanedBody(ht, input, body, err, model.CommentBodyMaxLength)
		})
	})
}

// bodyText that is often near the given maximum length, often padded with whitespace, and sometimes
// holds control characters, carriage returns, or invalid UTF-8.
func bodyText(maxLength int) hegel.Generator[string] {
	return hegel.Composite(func(tc hegel.TestCase) string {
		space := hegel.SampledFrom([]string{"", " ", "\n", "\r\n", "\r", "\t", "\u00a0", " \n \t"})
		before := hegel.Draw(tc, space)
		after := hegel.Draw(tc, space)

		var middle string
		switch hegel.Draw(tc, hegel.Integers(0, 3)) {
		case 0:
			middle = hegel.Draw(tc, hegel.Text().MaxSize(50))
		case 1:
			// Around the maximum, so that both sides of the limit come up.
			n := hegel.Draw(tc, hegel.Integers(maxLength-2, maxLength+2))
			middle = strings.Repeat(hegel.Draw(tc, hegel.SampledFrom([]string{"a", "æ", "😀"})), n)
		case 2:
			middle = string(hegel.Draw(tc, hegel.Binary(0, 20)))
		case 3:
			middle = hegel.Draw(tc, hegel.Text().Alphabet("ab \n\r\t\x00\x7f").MaxSize(20))
		}

		return before + middle + after
	})
}

// checkCleanedBody against what cleaning promises: either a body that is the input with its line breaks
// normalised and its ends trimmed, within the maximum length, or an error that the input explains.
func checkCleanedBody(ht *hegel.T, input, body string, err error, maxLength int) {
	ht.Helper()

	normalised := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n"))

	if err != nil {
		if body != "" {
			ht.Fatalf("got body %q along with error %v", body, err)
		}
		switch err {
		case model.ErrorBodyInvalid:
			if utf8.ValidString(input) && !strings.ContainsFunc(normalised, func(r rune) bool {
				return unicode.IsControl(r) && r != '\n' && r != '\t'
			}) {
				ht.Fatalf("refused %q as invalid, but it is valid UTF-8 with no control characters", input)
			}
		case model.ErrorBodyEmpty:
			if normalised != "" {
				ht.Fatalf("refused %q as empty", input)
			}
		case model.ErrorBodyTooLong:
			if utf8.RuneCountInString(normalised) <= maxLength {
				ht.Fatalf("refused %q as too long", input)
			}
		default:
			ht.Fatalf("unexpected error %v", err)
		}
		return
	}

	if !utf8.ValidString(input) {
		ht.Fatalf("accepted invalid UTF-8 %q", input)
	}
	if body != normalised {
		ht.Fatalf("body %q is not the input %q with its line breaks normalised and its ends trimmed", body, input)
	}
	if body == "" {
		ht.Fatalf("accepted %q as empty", input)
	}
	if strings.Contains(body, "\r") {
		ht.Fatalf("body %q holds a carriage return", body)
	}
	if utf8.RuneCountInString(body) > maxLength {
		ht.Fatalf("body is %v characters, more than %v", utf8.RuneCountInString(body), maxLength)
	}
}
