package http

import (
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	. "maragu.dev/gomponents"

	"app/model"
	"app/service"
)

// InjectHTTPRouterOptions is what the app's routes are set up with.
type InjectHTTPRouterOptions struct {
	// Group that this instance holds.
	Group model.Group

	// Now is the clock.
	Now func() time.Time

	// Random is the randomness that forms get their tokens from, such as [crypto/rand.Reader]. It must be
	// given, or [InjectHTTPRouter] panics, and a page that cannot read from it panics too.
	Random io.Reader

	// TimelinePageSize is how many posts a page of the timeline has. If it is 0, the business logic
	// decides.
	TimelinePageSize int

	// Extra, if not nil, returns something for the pages for members to show beside the name of the
	// person who is signed in on the sign, and for the page that says the group is private to show under
	// its field.
	Extra func(signedIn bool) Node
}

// extra to show for someone signed in or not, from [InjectHTTPRouterOptions.Extra]: nothing if it is nil.
func (o InjectHTTPRouterOptions) extra(signedIn bool) Node {
	if o.Extra == nil {
		return nil
	}
	return o.Extra(signedIn)
}

// InjectHTTPRouter returns what adds the app's routes to a router. No response of theirs may be stored.
func InjectHTTPRouter(log *slog.Logger, svc *service.Fat, opts InjectHTTPRouterOptions) func(*Router) {
	if opts.Random == nil {
		panic("http: no randomness for form tokens in InjectHTTPRouterOptions.Random")
	}
	token := formTokens(opts.Random)

	return func(r *Router) {
		r.Use(noStore, noReferrer, AddUserToContext(log, svc))

		r.Group(func(r *Router) {
			timeline(r, log, svc, opts, token)
		})

		r.Group(func(r *Router) {
			people(r, log, svc, opts, token)
		})
	}
}

// formTokens returns what gives a form that makes something its token, from the given randomness, which
// it reads one at a time. Every form shown gets a new one. It panics if the randomness cannot be read.
func formTokens(random io.Reader) func() string {
	var lock sync.Mutex
	return func() string {
		lock.Lock()
		defer lock.Unlock()

		t, err := model.NewFormToken(random)
		if err != nil {
			panic("error reading randomness for a form token: " + err.Error())
		}
		return t.String()
	}
}

// noStore tells browsers and caches to keep no copy of a response. The group is private, and a page
// carries a content security policy nonce that must not be paired with a stored copy of another response.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// noReferrer tells browsers to send no Referer header from a page, since the address of a page can hold
// the secret of a link in an email, and the group is private.
func noReferrer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
