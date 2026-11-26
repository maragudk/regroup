package jobs

import (
	"maragu.dev/glue/jobs"

	"app/model"
)

// emailSender sends the emails of the app.
type emailSender interface {
	signInLinkSender
	invitationSender
}

type RegisterOpts struct {
	// Emails is what sends the app's emails.
	Emails emailSender
}

// Funcs of every job, by name, with the given dependencies.
func Funcs(opts RegisterOpts) map[model.JobName]jobs.Func {
	return map[model.JobName]jobs.Func{
		model.JobNameSendSignInLink: SendSignInLink(opts.Emails),
		model.JobNameSendInvitation: SendInvitation(opts.Emails),
	}
}

// Register all available jobs with the given dependencies.
func Register(r *jobs.Runner, opts RegisterOpts) {
	for name, f := range Funcs(opts) {
		r.Register(name.String(), f)
	}
}
