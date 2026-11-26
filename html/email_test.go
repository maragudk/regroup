package html_test

import (
	"encoding/json/v2"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maragu.dev/glue/email"
	"maragu.dev/glue/email/postmark"
	"maragu.dev/is"

	"app/html"
	"app/model"
)

// sent is what glue's sender sends to Postmark.
type sent struct {
	To       string
	Subject  string
	HtmlBody string
}

// send the given email through glue's sender, with the app's templates, to a stand-in for Postmark, and
// return what reached it.
func send(t *testing.T, opts email.SendOptions) sent {
	t.Helper()

	var got sent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		is.NotError(t, err)
		is.NotError(t, json.Unmarshal(b, &got))
		_, _ = io.WriteString(w, "{}")
	}))
	t.Cleanup(server.Close)

	sender := postmark.NewSender(postmark.NewSenderOptions{
		AppName:                   "regroup",
		BaseURL:                   "https://loft.example.com",
		EndpointURL:               server.URL,
		Emails:                    html.EmailTemplates(),
		ReplyToEmailAddress:       "reply@example.com",
		TransactionalEmailAddress: "from@example.com",
	})
	is.NotError(t, sender.SendTransactional(t.Context(), opts))
	return got
}

func TestEmailTemplates(t *testing.T) {
	t.Run("sends a sign-in email naming the group, with a button and an address that lead to the link, in words a person can follow", func(t *testing.T) {
		got := send(t, email.SendOptions{
			Keywords: model.Keywords{"token": "abc-_123", "group": "The Loft", "lifetime": "15 minutes"},
			Subject:  "Sign in to The Loft",
			Template: "sign-in",
			To:       "signe@example.com",
			ToName:   "Signe",
		})

		is.Equal(t, "Sign in to The Loft", got.Subject)
		is.True(t, strings.Contains(got.HtmlBody, "<h1>Sign in to The Loft</h1>"), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, "Here is your link to sign in to The Loft. It works once, for the next 15 minutes."), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, `<a href="https://loft.example.com/sign-in/link?token=abc-_123" class="f-fallback button" target="_blank">Sign in</a>`), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, "If you did not ask for it, you can ignore this email. Nobody can sign in as you without the link."), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, "copy this address into your browser: https://loft.example.com/sign-in/link?token=abc-_123"), got.HtmlBody)
		is.True(t, !strings.Contains(got.HtmlBody, "{{"), got.HtmlBody)
	})

	t.Run("sends an invitation naming who invited and the group, with a button and an address that lead to the invitation", func(t *testing.T) {
		got := send(t, email.SendOptions{
			Keywords: model.Keywords{"token": "abc", "group": "The Loft", "inviter": "Signe", "lifetime": "7 days"},
			Subject:  "Signe invited you to The Loft",
			Template: "invitation",
			To:       "amira@example.com",
		})

		is.True(t, strings.Contains(got.HtmlBody, "<h1>You are invited to The Loft</h1>"), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, "Signe has invited you to join The Loft, a private group."), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, `<a href="https://loft.example.com/invitation?token=abc" class="f-fallback button" target="_blank">Accept the invitation</a>`), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, "The link works once, for the next 7 days. When you accept, you choose the name the group sees."), got.HtmlBody)
		is.True(t, strings.Contains(got.HtmlBody, "If you were not expecting this, you can ignore it. Nothing happens unless you accept."), got.HtmlBody)
		is.True(t, !strings.Contains(got.HtmlBody, "{{"), got.HtmlBody)
	})

	t.Run("escapes the names of the group and the inviter, and placeholders in them stay text", func(t *testing.T) {
		got := send(t, email.SendOptions{
			Keywords: model.Keywords{"token": "abc", "group": "<i>Loft</i> & Co", "inviter": "Dee {{token}} \"<b>\""},
			Subject:  "x",
			Template: "invitation",
			To:       "amira@example.com",
		})

		is.True(t, strings.Contains(got.HtmlBody, "Dee {{token}} &#34;&lt;b&gt;&#34; has invited you to join &lt;i&gt;Loft&lt;/i&gt; &amp; Co"), got.HtmlBody)
		is.True(t, !strings.Contains(got.HtmlBody, "<i>Loft"), got.HtmlBody)
	})

	t.Run("has no image the app does not serve, and no colour but the app's orange for links", func(t *testing.T) {
		layout, err := fs.ReadFile(html.EmailTemplates(), "layout.html")
		is.NotError(t, err)
		is.True(t, !strings.Contains(string(layout), "<img"))
		is.True(t, !strings.Contains(string(layout), "#4f46e5"))
	})
}

func TestEmailBody(t *testing.T) {
	t.Run("gives the body of a template without the layout, with its placeholders replaced and escaped", func(t *testing.T) {
		body, err := html.EmailBody("sign-in", map[string]string{"baseURL": "", "token": "abc", "group": "<Loft>"})
		is.NotError(t, err)
		is.True(t, strings.HasPrefix(body, "<h1>Sign in to &lt;Loft&gt;</h1>"), body)
		is.True(t, strings.Contains(body, `href="/sign-in/link?token=abc"`), body)
		is.True(t, !strings.Contains(body, "<html"), body)
	})

	t.Run("fails for a template there is not", func(t *testing.T) {
		_, err := html.EmailBody("nope", nil)
		is.True(t, err != nil)
	})
}
