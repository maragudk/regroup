package jobs

import (
	"context"
	"encoding/json/v2"

	"maragu.dev/glue/jobs"

	"app/model"
)

// signInLinkSender sends a sign-in link, if the address is one that gets one.
type signInLinkSender interface {
	SendSignInLink(ctx context.Context, address model.EmailAddress) error
}

// SendSignInLink is the job of [model.JobNameSendSignInLink]. Who gets a link is up to the sender.
func SendSignInLink(s signInLinkSender) jobs.Func {
	return jobs.WithTracing("jobs.SendSignInLink", func(ctx context.Context, m []byte) error {
		var jd model.SendSignInLinkJobData
		if err := json.Unmarshal(m, &jd); err != nil {
			panic(err)
		}
		return s.SendSignInLink(ctx, jd.Email)
	})
}

// invitationSender sends an invitation that is waiting.
type invitationSender interface {
	SendInvitation(ctx context.Context, id model.InvitationID) error
}

// SendInvitation is the job of [model.JobNameSendInvitation].
func SendInvitation(s invitationSender) jobs.Func {
	return jobs.WithTracing("jobs.SendInvitation", func(ctx context.Context, m []byte) error {
		var jd model.SendInvitationJobData
		if err := json.Unmarshal(m, &jd); err != nil {
			panic(err)
		}
		return s.SendInvitation(ctx, jd.InvitationID)
	})
}
