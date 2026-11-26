package http

import (
	"maragu.dev/httph"
)

// CSP for every page: everything comes from the app's own origin, and nothing else is allowed.
//
// Scripts also get a nonce that is new for each response, available from [httph.NonceFromContext].
// Datastar needs it to run the expressions in a page, and with it the policy never needs 'unsafe-eval'.
// Because a browser ignores 'unsafe-inline' for scripts once there is a nonce, allowUnsafeInline matters
// for styles only.
func CSP(allowUnsafeInline bool) func(*httph.ContentSecurityPolicyOptions) {
	return func(opts *httph.ContentSecurityPolicyOptions) {
		opts.ScriptSrc = "'self'"
		opts.ScriptNonce = true

		styleSrc := "'self'"
		if allowUnsafeInline {
			styleSrc += " 'unsafe-inline'"
		}
		opts.StyleSrc = styleSrc
	}
}
