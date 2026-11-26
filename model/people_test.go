package model_test

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"hegel.dev/go/hegel"
	"maragu.dev/is"

	"app/model"
)

func TestLifetimeWords(t *testing.T) {
	t.Run("says a lifetime in whole days, hours, or minutes, in the singular for one", func(t *testing.T) {
		for d, expected := range map[time.Duration]string{
			15 * time.Minute:   "15 minutes",
			time.Minute:        "1 minute",
			time.Hour:          "1 hour",
			90 * time.Minute:   "90 minutes",
			48 * time.Hour:     "2 days",
			24 * time.Hour:     "1 day",
			7 * 24 * time.Hour: "7 days",
			36 * time.Hour:     "36 hours",
		} {
			is.Equal(t, expected, model.LifetimeWords(d))
		}
	})
}

func TestParseEmailAddress(t *testing.T) {
	long := strings.Repeat("a", 64) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 58) + ".dk"

	tests := []struct {
		name     string
		input    string
		expected model.EmailAddress
		err      error
	}{
		{name: "accepts an address as it is", input: "signe@example.com", expected: "signe@example.com"},
		{name: "lowercases it", input: "Signe.Holm@Example.COM", expected: "signe.holm@example.com"},
		{name: "trims white space at both ends", input: " \t signe@example.com \n", expected: "signe@example.com"},
		{name: "accepts the special characters of a local part", input: "a.b+c!#$%&'*/=?^_\x60{|}~-@example.co.uk", expected: "a.b+c!#$%&'*/=?^_\x60{|}~-@example.co.uk"},
		{name: "accepts an address of exactly the maximum length", input: long, expected: model.EmailAddress(long)},
		{name: "refuses an address one byte too long", input: "a" + long, err: model.ErrorEmailInvalid},
		{name: "refuses an empty address", input: "", err: model.ErrorEmailInvalid},
		{name: "refuses white space alone", input: "  ", err: model.ErrorEmailInvalid},
		{name: "refuses an address without an @", input: "signe.example.com", err: model.ErrorEmailInvalid},
		{name: "refuses an address with two @", input: "signe@holm@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses a domain of one label", input: "signe@localhost", err: model.ErrorEmailInvalid},
		{name: "refuses white space inside", input: "signe holm@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses a name and an address", input: "Signe <signe@example.com>", err: model.ErrorEmailInvalid},
		{name: "refuses two addresses", input: "signe@example.com,thomas@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses a line break inside", input: "signe@example.com\nBcc: x@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses characters outside ASCII", input: "bjørn@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses the Kelvin sign, which lowercases to a k", input: "marKus@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses a capital I with a dot, which lowercases to an i", input: "İda@example.com", err: model.ErrorEmailInvalid},
		{name: "refuses a domain that starts with a hyphen", input: "signe@-example.com", err: model.ErrorEmailInvalid},
	}

	for _, test := range tests {
		t.Run("should "+test.name, func(t *testing.T) {
			e, err := model.ParseEmailAddress(test.input)
			if test.err != nil {
				is.Error(t, test.err, err)
				is.Equal(t, model.EmailAddress(""), e)
				return
			}
			is.NotError(t, err)
			is.Equal(t, test.expected, e)
		})
	}

	t.Run("parses two spellings of an address that differ in case and white space as the same address", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			address := hegel.Draw(ht, hegel.Emails())
			e, err := model.ParseEmailAddress(address)
			ht.Assume(err == nil)

			spelling := respell(ht, address)
			again, err := model.ParseEmailAddress(spelling)
			if err != nil {
				ht.Fatalf("parsed %q, but not %q: %v", address, spelling, err)
			}
			if again != e {
				ht.Fatalf("parsed %q as %q, but %q as %q", address, e, spelling, again)
			}
		})
	})

	t.Run("accepts only what is trimmed, lowercase, within the maximum length, and an address by the rule, and parses it to itself", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := hegel.Draw(ht, emailText())

			e, err := model.ParseEmailAddress(input)
			if err != nil {
				if !errors.Is(err, model.ErrorEmailInvalid) {
					ht.Fatalf("unexpected error %v", err)
				}
				if trimmed := strings.TrimSpace(input); len(trimmed) <= model.EmailAddressMaxLength && emailRule.MatchString(trimmed) {
					ht.Fatalf("refused %q, which is an address by the rule", input)
				}
				return
			}
			checkAddress(ht, input, e)
		})
	})
}

// emailRule is the definition of a valid email address that HTML forms use, from the WHATWG HTML
// standard, written out here so that the tests do not take the rule from the code they test.
var emailRule = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_\x60{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$")

// checkAddress that the given address, parsed from the given input, is what parsing promises.
func checkAddress(t interface{ Fatalf(string, ...any) }, input string, e model.EmailAddress) {
	s := e.String()
	switch {
	case s != strings.ToLower(strings.TrimSpace(input)):
		t.Fatalf("parsed %q as %q, which is not it trimmed and lowercased", input, e)
	case len(s) > model.EmailAddressMaxLength:
		t.Fatalf("parsed %q as %q, which is longer than the maximum", input, e)
	case !emailRule.MatchString(s):
		t.Fatalf("parsed %q as %q, which is not an address by the rule", input, e)
	}
	again, err := model.ParseEmailAddress(s)
	if err != nil || again != e {
		t.Fatalf("parsed %q as %q, and that as %q with %v", input, e, again, err)
	}
}

// respell the given address with other cases and white space around it.
func respell(tc hegel.TestCase, address string) string {
	var b strings.Builder
	b.WriteString(hegel.Draw(tc, hegel.SampledFrom([]string{"", " ", "\t", "\n", " \u00a0"})))
	for _, r := range address {
		if hegel.Draw(tc, hegel.Booleans()) {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	b.WriteString(hegel.Draw(tc, hegel.SampledFrom([]string{"", " ", "\r\n", "\u3000"})))
	return b.String()
}

// emailText that is often an address, often around the maximum length, and sometimes hostile.
func emailText() hegel.Generator[string] {
	return hegel.Composite(func(tc hegel.TestCase) string {
		switch hegel.Draw(tc, hegel.Integers(0, 3)) {
		case 0:
			return respell(tc, hegel.Draw(tc, hegel.Emails()))
		case 1:
			// Around the maximum length, so that both sides of the limit come up.
			n := hegel.Draw(tc, hegel.Integers(model.EmailAddressMaxLength-4, model.EmailAddressMaxLength+4))
			return strings.Repeat("a", n-195) + "@" + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63) + ".dk"
		case 2:
			return hegel.Draw(tc, hegel.Text().Alphabet("aZ.@-+ <>,\n\x00ø\"").MaxSize(30))
		default:
			return hegel.Draw(tc, hegel.Text().MaxSize(40))
		}
	})
}

func FuzzParseEmailAddress(f *testing.F) {
	f.Add("signe@example.com")
	f.Add(" Signe@Example.COM ")
	f.Add("")
	f.Add("signe@localhost")
	f.Add("Signe <signe@example.com>")
	f.Add("signe@example.com\nBcc: x@example.com")
	f.Add("bjørn@example.com")
	f.Add(strings.Repeat("a", 300) + "@example.com")

	f.Fuzz(func(t *testing.T, s string) {
		e, err := model.ParseEmailAddress(s)
		if err != nil {
			if !errors.Is(err, model.ErrorEmailInvalid) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		checkAddress(t, s, e)
	})
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		err      error
	}{
		{name: "keeps a name as it is", input: "Signe Holm", expected: "Signe Holm"},
		{name: "keeps letters from anywhere", input: "Bjørn Ægidius 王 Ахмед", expected: "Bjørn Ægidius 王 Ахмед"},
		{name: "keeps HTML as text", input: "<b>Dee</b> & Co", expected: "<b>Dee</b> & Co"},
		{name: "keeps an emoji with a joiner in it", input: "Ada 👩\u200d💻", expected: "Ada 👩\u200d💻"},
		{name: "trims white space at both ends", input: " \t Signe \n", expected: "Signe"},
		{name: "accepts exactly the maximum length", input: strings.Repeat("æ", model.NameMaxLength), expected: strings.Repeat("æ", model.NameMaxLength)},
		{name: "counts the length after trimming", input: "  " + strings.Repeat("a", model.NameMaxLength) + "  ", expected: strings.Repeat("a", model.NameMaxLength)},
		{name: "refuses a name one character too long", input: strings.Repeat("æ", model.NameMaxLength+1), err: model.ErrorNameTooLong},
		{name: "refuses an empty name", input: "", err: model.ErrorNameEmpty},
		{name: "refuses a name of only white space", input: " \t\u00a0 ", err: model.ErrorNameEmpty},
		{name: "refuses a name of only invisible characters", input: "\u200b\u3164\u2800", err: model.ErrorNameEmpty},
		{name: "refuses a line break inside", input: "Signe\nHolm", err: model.ErrorNameInvalid},
		{name: "refuses a carriage return inside", input: "Signe\rHolm", err: model.ErrorNameInvalid},
		{name: "refuses a tab inside", input: "Signe\tHolm", err: model.ErrorNameInvalid},
		{name: "refuses a line separator inside", input: "Signe\u2028Holm", err: model.ErrorNameInvalid},
		{name: "refuses a paragraph separator inside", input: "Signe\u2029Holm", err: model.ErrorNameInvalid},
		{name: "refuses a control character", input: "Signe\x07", err: model.ErrorNameInvalid},
		{name: "refuses a right-to-left override, which would turn the text after the name around", input: "Signe\u202e", err: model.ErrorNameInvalid},
		{name: "refuses a directional isolate", input: "\u2067Signe", err: model.ErrorNameInvalid},
		{name: "refuses invalid UTF-8", input: "Signe \xff", err: model.ErrorNameInvalid},
	}

	for _, test := range tests {
		t.Run("should "+test.name, func(t *testing.T) {
			name, err := model.CleanName(test.input)
			if test.err != nil {
				is.Error(t, test.err, err)
				is.Equal(t, "", name)
				return
			}
			is.NotError(t, err)
			is.Equal(t, test.expected, name)
		})
	}

	t.Run("cleans any text so it is one trimmed line within the maximum with something visible, or says why not", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := hegel.Draw(ht, nameText())

			name, err := model.CleanName(input)
			checkName(ht, input, name, err)
		})
	})

	t.Run("cleaning a cleaned name changes nothing", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			input := hegel.Draw(ht, nameText())
			name, err := model.CleanName(input)
			ht.Assume(err == nil)

			again, err := model.CleanName(name)
			if err != nil || again != name {
				ht.Fatalf("cleaned %q as %q, and that as %q with %v", input, name, again, err)
			}
		})
	})
}

// nameText that is often a name, often around the maximum length, often padded, and sometimes holds
// line breaks, controls, and characters that turn text around.
func nameText() hegel.Generator[string] {
	return hegel.Composite(func(tc hegel.TestCase) string {
		space := hegel.SampledFrom([]string{"", " ", "\t", "\n", "\r\n", "\u00a0", "\u200b", "\u3164"})
		before := hegel.Draw(tc, space)
		after := hegel.Draw(tc, space)

		var middle string
		switch hegel.Draw(tc, hegel.Integers(0, 3)) {
		case 0:
			middle = hegel.Draw(tc, hegel.Text().MaxSize(60))
		case 1:
			n := hegel.Draw(tc, hegel.Integers(model.NameMaxLength-2, model.NameMaxLength+2))
			middle = strings.Repeat(hegel.Draw(tc, hegel.SampledFrom([]string{"a", "æ", "😀"})), n)
		case 2:
			middle = string(hegel.Draw(tc, hegel.Binary(0, 20)))
		default:
			middle = hegel.Draw(tc, hegel.Text().Alphabet("ab \n\r\t\x00\x7f\u200b\u202e\u2066\u2028\u2029\u3164").MaxSize(20))
		}
		return before + middle + after
	})
}

// turnsAround reports whether a character changes the direction of the text around it: the
// bidirectional controls, as Unicode's PropList.txt lists them, written out here so that the tests do
// not take the rule from the code they test.
func turnsAround(r rune) bool {
	return r == 0x061C || r == 0x200E || r == 0x200F || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// checkName against what cleaning a name promises: the input trimmed, on one line, within the maximum
// length, with something visible, and no character that turns text around, or an error the input
// explains.
func checkName(t interface{ Fatalf(string, ...any) }, input, name string, err error) {
	trimmed := strings.TrimSpace(input)
	bad := func(r rune) bool {
		return unicode.IsControl(r) || r == 0x2028 || r == 0x2029 || turnsAround(r)
	}

	if err != nil {
		if name != "" {
			t.Fatalf("got name %q along with error %v", name, err)
		}
		switch err {
		case model.ErrorNameInvalid:
			if utf8.ValidString(input) && !strings.ContainsFunc(trimmed, bad) {
				t.Fatalf("refused %q as invalid", input)
			}
		case model.ErrorNameEmpty:
			if strings.ContainsFunc(trimmed, func(r rune) bool { return !blank(r) }) {
				t.Fatalf("refused %q as empty, but it has something visible", input)
			}
		case model.ErrorNameTooLong:
			if utf8.RuneCountInString(trimmed) <= model.NameMaxLength {
				t.Fatalf("refused %q as too long", input)
			}
		default:
			t.Fatalf("unexpected error %v", err)
		}
		return
	}

	switch {
	case !utf8.ValidString(input):
		t.Fatalf("accepted invalid UTF-8 %q", input)
	case name != trimmed:
		t.Fatalf("name %q is not the input %q trimmed", name, input)
	case strings.ContainsFunc(name, bad):
		t.Fatalf("name %q holds a line break, a control, or a character that turns text around", name)
	case !strings.ContainsFunc(name, func(r rune) bool { return !blank(r) }):
		t.Fatalf("accepted %q, which has nothing visible", input)
	case utf8.RuneCountInString(name) > model.NameMaxLength:
		t.Fatalf("name %q is longer than the maximum", name)
	}
}

func FuzzCleanName(f *testing.F) {
	f.Add("Signe Holm")
	f.Add("")
	f.Add(" \t ")
	f.Add("Signe\nHolm")
	f.Add("Signe\u202e")
	f.Add("\xff")
	f.Add(strings.Repeat("æ", model.NameMaxLength+1))

	f.Fuzz(func(t *testing.T, s string) {
		name, err := model.CleanName(s)
		checkName(t, s, name, err)
	})
}

func TestLinkToken(t *testing.T) {
	t.Run("makes a token of 43 URL-safe characters from 32 bytes, which parses as itself", func(t *testing.T) {
		token, err := model.NewLinkToken(bytes.NewReader(make([]byte, 32)))
		is.NotError(t, err)
		is.Equal(t, model.LinkToken("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"), token)

		parsed, err := model.ParseLinkToken(token.String())
		is.NotError(t, err)
		is.Equal(t, token, parsed)
	})

	t.Run("fails if the randomness runs out", func(t *testing.T) {
		_, err := model.NewLinkToken(bytes.NewReader(make([]byte, 31)))
		is.True(t, err != nil)
	})

	t.Run("hashes a token as the SHA-256 of its text, in lowercase hexadecimal", func(t *testing.T) {
		is.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", model.LinkToken("abc").Hash())
	})

	t.Run("makes tokens of 43 characters that parse as themselves", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			token, err := model.NewLinkToken(bytes.NewReader(hegel.Draw(ht, hegel.Binary(32, 32))))
			if err != nil {
				ht.Fatal(err)
			}
			if parsed, err := model.ParseLinkToken(token.String()); err != nil || parsed != token || len(token) != 43 {
				ht.Fatalf("token %q parsed as %q with %v", token, parsed, err)
			}
		})
	})

	t.Run("hashes tokens that differ to hashes that differ, of 64 lowercase hexadecimal characters", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			a, err := model.NewLinkToken(bytes.NewReader(hegel.Draw(ht, hegel.Binary(32, 32))))
			if err != nil {
				ht.Fatal(err)
			}
			b, err := model.NewLinkToken(bytes.NewReader(hegel.Draw(ht, hegel.Binary(32, 32))))
			if err != nil {
				ht.Fatal(err)
			}
			for _, h := range []string{a.Hash(), b.Hash()} {
				if len(h) != 64 || strings.Trim(h, "0123456789abcdef") != "" {
					ht.Fatalf("hash %q is not 64 lowercase hexadecimal characters", h)
				}
			}
			if (a == b) != (a.Hash() == b.Hash()) {
				ht.Fatalf("tokens %q and %q have hashes %q and %q", a, b, a.Hash(), b.Hash())
			}
		})
	})

	t.Run("refuses a token with one character changed, added, or taken away, unless it is still one that could be made", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			token, err := model.NewLinkToken(bytes.NewReader(hegel.Draw(ht, hegel.Binary(32, 32))))
			if err != nil {
				ht.Fatal(err)
			}
			s := token.String()
			i := hegel.Draw(ht, hegel.Integers(0, len(s)-1))
			var changed string
			switch hegel.Draw(ht, hegel.Integers(0, 2)) {
			case 0:
				c := hegel.Draw(ht, hegel.SampledFrom([]string{"A", "a", "0", "-", "_", "+", "/", "=", ".", " ", "\n", "%"}))
				changed = s[:i] + c + s[i+1:]
			case 1:
				changed = s[:i] + s[i+1:]
			default:
				changed = s[:i] + "A" + s[i:]
			}

			parsed, err := model.ParseLinkToken(changed)
			if canonical(changed) {
				if err != nil || parsed.String() != changed {
					ht.Fatalf("refused %q, which could have been made", changed)
				}
				return
			}
			if !errors.Is(err, model.ErrorLinkInvalid) {
				ht.Fatalf("parsed %q, changed from %q, with %v", changed, token, err)
			}
		})
	})

	t.Run("refuses with the one refusal for links anything that a token could not be", func(t *testing.T) {
		for _, s := range []string{"", "t_0123456789abcdef0123456789abcdef", strings.Repeat("A", 42), strings.Repeat("A", 44), strings.Repeat("A", 42) + "B", strings.Repeat("A", 43) + "=", strings.Repeat("A", 20) + "\n" + strings.Repeat("A", 23), strings.Repeat("+", 43)} {
			_, err := model.ParseLinkToken(s)
			is.Error(t, model.ErrorLinkInvalid, err, s)
		}
	})

	t.Run("draws different tokens from different randomness", func(t *testing.T) {
		r := rand.NewChaCha8([32]byte{1})
		seen := map[model.LinkToken]bool{}
		for range 1000 {
			token, err := model.NewLinkToken(r)
			is.NotError(t, err)
			is.True(t, !seen[token])
			seen[token] = true
		}
	})
}

// canonical reports whether the given text is the unpadded URL-safe base64 of 32 bytes, written out
// here so that the tests do not take the rule from the code they test: 43 characters of the alphabet,
// the last of which carries only four bits, so it is every fourth character of the alphabet.
func canonical(s string) bool {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	if len(s) != 43 {
		return false
	}
	for _, c := range []byte(s) {
		if strings.IndexByte(alphabet, c) < 0 {
			return false
		}
	}
	return strings.IndexByte(alphabet, s[42])%4 == 0
}

func FuzzParseLinkToken(f *testing.F) {
	f.Add("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	f.Add("")
	f.Add("t_0123456789abcdef0123456789abcdef")
	f.Add(strings.Repeat("A", 42) + "B")
	f.Add(strings.Repeat("A", 20) + "\n" + strings.Repeat("A", 23))

	f.Fuzz(func(t *testing.T, s string) {
		token, err := model.ParseLinkToken(s)
		if err != nil {
			if !errors.Is(err, model.ErrorLinkInvalid) {
				t.Fatalf("unexpected error %v", err)
			}
			if canonical(s) {
				t.Fatalf("refused %q, which could have been made", s)
			}
			return
		}
		if token.String() != s || !canonical(s) {
			t.Fatalf("parsed %q as %q", s, token)
		}
	})
}
