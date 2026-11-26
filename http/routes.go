package http

import (
	"log/slog"
	"net/http"
	"time"

	. "maragu.dev/gomponents"

	"app/model"
	"app/service"
)

type InjectHTTPRouterOptions struct {
	// Group that this instance holds.
	Group model.Group

	// Now is the clock.
	Now func() time.Time

	// PreviewLink, if not nil, returns a link that pages show to someone who is signed in, and to
	// someone who is not.
	PreviewLink func(signedIn bool) Node
}

func InjectHTTPRouter(log *slog.Logger, svc *service.Fat, opts InjectHTTPRouterOptions) func(*Router) {
	return func(r *Router) {
		r.Use(noStore, AddUserToContext(log, svc))

		r.Group(func(r *Router) {
			Timeline(r, log, svc, opts)
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
