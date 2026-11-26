package model

type JobName string

func (j JobName) String() string {
	return string(j)
}

const (
	// JobNameSendSignInLink sends a sign-in link to an address, if it is one that gets one.
	JobNameSendSignInLink JobName = "send-sign-in-link"

	// JobNameSendInvitation sends an invitation that is waiting.
	JobNameSendInvitation JobName = "send-invitation"
)

// SendSignInLinkJobData is the address someone asked for a sign-in link with, as [ParseEmailAddress]
// returned it. Whether the address gets one is decided when the job runs, so that asking looks the same
// whatever the answer.
type SendSignInLinkJobData struct {
	Email EmailAddress
}

// SendInvitationJobData is the invitation to send. Its link is made when the job runs, so that the link's
// token is never stored.
type SendInvitationJobData struct {
	InvitationID InvitationID
}
