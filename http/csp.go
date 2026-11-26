package http

import (
	"maragu.dev/httph"
)

// CSP for every page: everything comes from the app's own origin, and nothing else is allowed.
//
// Scripts are also allowed by a nonce that is new for each response, available from
// [httph.NonceFromContext]. Evaluating strings as scripts is never allowed. A browser ignores
// 'unsafe-inline' for scripts once there is a nonce, so allowUnsafeInline allows inline styles only.
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
