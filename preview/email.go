package preview

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"maragu.dev/glue/email"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"

	"app/html"
	"app/model"
)

// Outbox keeps the newest hundred emails the app would have sent, instead of sending them, so that the
// preview can show them with links that work. It keeps them in memory, so they are gone when the app
// stops. It is safe for concurrent use.
type Outbox struct {
	log    *slog.Logger
	now    func() time.Time
	mu     sync.Mutex
	emails []keptEmail
}

// keptEmail in an [Outbox].
type keptEmail struct {
	at   time.Time
	opts email.SendOptions
}

// NewOutbox with nothing in it, which logs what it keeps to the given logger and times it by the given
// clock.
func NewOutbox(log *slog.Logger, now func() time.Time) *Outbox {
	return &Outbox{log: log, now: now}
}

// SendTransactional keeps the email instead of sending it, and satisfies [email.Sender].
func (o *Outbox) SendTransactional(ctx context.Context, opts email.SendOptions) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.emails = append(o.emails, keptEmail{at: o.now(), opts: opts})
	if len(o.emails) > 100 {
		o.emails = o.emails[len(o.emails)-100:]
	}

	o.log.InfoContext(ctx, "Kept an email for the preview instead of sending it", "template", opts.Template)
	return nil
}

// kept emails, the newest first.
func (o *Outbox) kept() []keptEmail {
	o.mu.Lock()
	defer o.mu.Unlock()

	emails := slices.Clone(o.emails)
	slices.Reverse(emails)
	return emails
}

var _ email.Sender = (*Outbox)(nil)

// emailsPage lists the emails in the outbox, the newest first, each with its subject, recipient, and
// body, whose links lead into the app the page is on.
func emailsPage(props html.PageProps, o *Outbox) (Node, error) {
	var items []Node
	for _, e := range o.kept() {
		keywords := model.Keywords{"baseURL": "", "name": e.opts.ToName}
		for k, v := range e.opts.Keywords {
			keywords[k] = v
		}
		body, err := html.EmailBody(e.opts.Template, keywords)
		if err != nil {
			return html.ErrorPage(props), err
		}

		items = append(items, Li(Class("grid gap-2 border-t border-stone-200 pt-4"),
			H2(Class("font-semibold"), Text(e.opts.Subject)),
			P(Class("text-sm text-stone-600"), Text("To "+e.opts.To.String()+", at "+e.at.UTC().Format(time.RFC3339))),
			Div(Class("grid gap-2"), Raw(body)),
		))
	}

	props.Title = "Preview: emails"
	return html.Page(props,
		H1(Text("Emails the preview kept")),
		P(Text("The preview sends no email. This is what it would have sent, the newest first, with links that work.")),
		If(len(items) == 0, P(Text("Nothing yet."))),
		Ul(Class("grid gap-6"), Group(items)),
	), nil
}
