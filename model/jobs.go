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
// returned it, whether or not the address gets one.
type SendSignInLinkJobData struct {
	Email EmailAddress
}

// SendInvitationJobData is the invitation to send, without its link, whose token is a secret.
type SendInvitationJobData struct {
	InvitationID InvitationID
}
