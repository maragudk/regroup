package http

import (
	"log/slog"
	"net/http"
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

	// Extra, if not nil, returns something for the group page to show beside the name of the person
	// who is signed in, and for the page that says the group is private to show under that.
	Extra func(signedIn bool) Node
}

// InjectHTTPRouter returns what adds the app's routes to a router. No response of theirs may be stored.
func InjectHTTPRouter(log *slog.Logger, svc *service.Fat, opts InjectHTTPRouterOptions) func(*Router) {
	return func(r *Router) {
		r.Use(noStore, AddUserToContext(log, svc))

		r.Group(func(r *Router) {
			timeline(r, log, svc, opts)
		})
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
