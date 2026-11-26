package model_test

import (
	"errors"
	"slices"
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
		{name: "refuses a body of only invisible formatting characters", input: "\u200b\ufeff\u2060\u00ad", err: model.ErrorBodyEmpty},
		{name: "keeps invisible formatting characters next to visible ones", input: "👩\u200d💻 shipped", expected: "👩\u200d💻 shipped"},
		{name: "refuses a body of only Hangul fillers", input: "\u3164\uffa0\u115f\u1160", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only the Braille blank", input: "\u2800 \u2800", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only other characters that are drawn blank", input: "\U0001d159\U00013441\U00013442\U00016fe4", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only variation selectors and the combining grapheme joiner", input: "\ufe0f\U000e0100\u034f\u180b", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only tag characters and the Mongolian vowel separator", input: "\U000e0001\U000e0067\U000e007f\u180e", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only default-ignorable code points that are not assigned yet", input: "\u2065\ufff0\U000e0fff", err: model.ErrorBodyEmpty},
		{name: "refuses a body of only Khmer inherent vowels", input: "\u17b4\u17b5", err: model.ErrorBodyEmpty},
		{name: "keeps a Hangul syllable next to a filler", input: "한\u3164", expected: "한\u3164"},
		{name: "keeps a Braille pattern with a dot in it", input: "\u2801", expected: "\u2801"},
		{name: "keeps a combining accent on its own, which is drawn", input: "\u0301", expected: "\u0301"},
		{name: "keeps an Arabic number sign, which is a format character that is drawn", input: "\u0600", expected: "\u0600"},
		{name: "keeps a character for private use, which is drawn", input: "\ue000", expected: "\ue000"},
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

	t.Run("refuses as empty any text of only white space, default-ignorable code points, and characters drawn blank", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := string(hegel.Draw(ht, hegel.Lists(blankRune()).MaxSize(30)))

			if _, err := model.CleanPostBody(input); err != model.ErrorBodyEmpty {
				ht.Fatalf("cleaning %+q gave %v, not that it is empty", input, err)
			}
		})
	})

	t.Run("accepts such text with one visible character anywhere in it, and keeps that character", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			blanks := hegel.Draw(ht, hegel.Lists(blankRune()).MaxSize(30))
			visible := hegel.Draw(ht, visibleRune())
			at := hegel.Draw(ht, hegel.Integers(0, len(blanks)))
			input := string(blanks[:at]) + string(visible) + string(blanks[at:])

			body, err := model.CleanPostBody(input)
			if err != nil {
				ht.Fatalf("cleaning %+q gave %v", input, err)
			}
			if !strings.ContainsRune(body, visible) {
				ht.Fatalf("cleaning %+q gave %+q, without %+q", input, body, visible)
			}
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
		space := hegel.SampledFrom([]string{"", " ", "\n", "\r\n", "\r", "\t", "\u00a0", " \n \t", "\u200b", "\ufeff\u2060", "\u00ad", "\u3164", "\u2800", "\ufe0f"})
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
			middle = hegel.Draw(tc, hegel.Text().Alphabet("ab \n\r\t\x00\x7f\u200b\u200d\ufeff\u2028\u3164\u2800\u0600").MaxSize(20))
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
			if strings.ContainsFunc(normalised, func(r rune) bool { return !blank(r) }) {
				ht.Fatalf("refused %q as empty, but it has something visible", input)
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
	if !strings.ContainsFunc(body, func(r rune) bool { return !blank(r) }) {
		ht.Fatalf("accepted %q, which has nothing visible", input)
	}
	if strings.Contains(body, "\r") {
		ht.Fatalf("body %q holds a carriage return", body)
	}
	if utf8.RuneCountInString(body) > maxLength {
		ht.Fatalf("body is %v characters, more than %v", utf8.RuneCountInString(body), maxLength)
	}
}

// defaultIgnorable code points, as Unicode's DerivedCoreProperties.txt lists them, written out here so that
// the tests do not take the rule from the code they test. Fonts draw none of them.
var defaultIgnorable = []struct{ lo, hi rune }{
	{0x00AD, 0x00AD}, {0x034F, 0x034F}, {0x061C, 0x061C}, {0x115F, 0x1160}, {0x17B4, 0x17B5},
	{0x180B, 0x180F}, {0x200B, 0x200F}, {0x202A, 0x202E}, {0x2060, 0x206F}, {0x3164, 0x3164},
	{0xFE00, 0xFE0F}, {0xFEFF, 0xFEFF}, {0xFFA0, 0xFFA0}, {0xFFF0, 0xFFF8}, {0x1BCA0, 0x1BCA3},
	{0x1D173, 0x1D17A}, {0xE0000, 0xE0FFF},
}

// drawnBlank characters are drawn as nothing, though Unicode does not class them as ignorable: the Braille
// pattern blank, the musical null notehead, the Egyptian hieroglyph full and half blanks, and the Khitan
// small script filler.
var drawnBlank = []rune{0x2800, 0x1D159, 0x13441, 0x13442, 0x16FE4}

// drawnFormat characters are format characters that are drawn, such as the Arabic number sign: the
// prepended concatenation marks, as Unicode's PropList.txt lists them.
var drawnFormat = []struct{ lo, hi rune }{
	{0x0600, 0x0605}, {0x06DD, 0x06DD}, {0x070F, 0x070F}, {0x0890, 0x0891}, {0x08E2, 0x08E2},
	{0x110BD, 0x110BD}, {0x110CD, 0x110CD},
}

// blank reports whether a body made only of the given character would show nothing: it is white space,
// default-ignorable, drawn blank, or a format character that is not drawn.
func blank(r rune) bool {
	in := func(ranges []struct{ lo, hi rune }) bool {
		return slices.ContainsFunc(ranges, func(d struct{ lo, hi rune }) bool { return d.lo <= r && r <= d.hi })
	}
	return in(defaultIgnorable) || unicode.Is(unicode.White_Space, r) || slices.Contains(drawnBlank, r) ||
		(unicode.Is(unicode.Cf, r) && !in(drawnFormat))
}

// blankRune that a body may hold, but that shows nothing: white space other than carriage returns, a
// default-ignorable code point, or a character drawn blank.
func blankRune() hegel.Generator[rune] {
	// White space, as Unicode's PropList.txt lists it, without the controls among it, which a body may not
	// hold other than line feeds and tabs.
	whiteSpace := []rune{
		'\t', '\n', ' ', 0x00A0, 0x1680, 0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007, 0x2008,
		0x2009, 0x200A, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000,
	}

	return hegel.Composite(func(tc hegel.TestCase) rune {
		switch hegel.Draw(tc, hegel.Integers(0, 2)) {
		case 0:
			return hegel.Draw(tc, hegel.SampledFrom(whiteSpace))
		case 1:
			d := hegel.Draw(tc, hegel.SampledFrom(defaultIgnorable))
			return rune(hegel.Draw(tc, hegel.Integers(int(d.lo), int(d.hi))))
		default:
			return hegel.Draw(tc, hegel.SampledFrom(drawnBlank))
		}
	})
}

// visibleRune is a letter, a mark, a number, a punctuation mark, or a symbol that is not blank.
func visibleRune() hegel.Generator[rune] {
	return hegel.Composite(func(tc hegel.TestCase) rune {
		table := hegel.Draw(tc, hegel.SampledFrom([]*unicode.RangeTable{unicode.L, unicode.M, unicode.N, unicode.P, unicode.S}))

		var ranges []unicode.Range32
		for _, r := range table.R16 {
			ranges = append(ranges, unicode.Range32{Lo: uint32(r.Lo), Hi: uint32(r.Hi), Stride: uint32(r.Stride)})
		}
		ranges = append(ranges, table.R32...)
		r := hegel.Draw(tc, hegel.SampledFrom(ranges))
		v := rune(r.Lo + r.Stride*uint32(hegel.Draw(tc, hegel.Integers(0, int((r.Hi-r.Lo)/r.Stride)))))

		tc.Assume(!blank(v))
		return v
	})
}

func FuzzCleanPostBody(f *testing.F) {
	f.Add("Cake in the kitchen.")
	f.Add("")
	f.Add("  \r\n\t ")
	f.Add("a\r\nb\rc")
	f.Add("\xff")
	f.Add("Cake\x07")
	f.Add(string(rune(0x200b)) + string(rune(0x3164)))
	f.Add(strings.Repeat("æ", model.CommentBodyMaxLength+1))

	f.Fuzz(func(t *testing.T, s string) {
		post, postErr := model.CleanPostBody(s)
		comment, commentErr := model.CleanCommentBody(s)

		for _, err := range []error{postErr, commentErr} {
			if err != nil && !errors.Is(err, model.ErrorBodyEmpty) && !errors.Is(err, model.ErrorBodyInvalid) && !errors.Is(err, model.ErrorBodyTooLong) {
				t.Fatalf("unexpected error %v", err)
			}
		}

		// A comment is a post with a lower maximum: what is fine as one is fine as the other, the same.
		if commentErr == nil && (postErr != nil || post != comment) {
			t.Fatalf("cleaned %q as the comment %q, but as the post %q with %v", s, comment, post, postErr)
		}
		if postErr != nil {
			return
		}

		// What is accepted is text with line feeds for line breaks, nothing to trim, at most the maximum
		// length, no control character but line feeds and tabs, and the same when cleaned again.
		if !utf8.ValidString(post) || strings.ContainsRune(post, '\r') || strings.TrimSpace(post) != post || utf8.RuneCountInString(post) > model.PostBodyMaxLength {
			t.Fatalf("cleaned %q as %q", s, post)
		}
		if strings.ContainsFunc(post, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) {
			t.Fatalf("cleaned %q as %q, with a control character in it", s, post)
		}
		if again, err := model.CleanPostBody(post); err != nil || again != post {
			t.Fatalf("cleaned %q as %q, and that as %q with %v", s, post, again, err)
		}
	})
}
