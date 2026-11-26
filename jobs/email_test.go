package jobs_test

import (
	"context"
	"encoding/json/v2"
	"testing"

	"maragu.dev/errors"
	"maragu.dev/is"

	"app/jobs"
	"app/model"
)

type emailsStub struct {
	err        error
	address    model.EmailAddress
	invitation model.InvitationID
}

func (s *emailsStub) SendSignInLink(ctx context.Context, address model.EmailAddress) error {
	s.address = address
	return s.err
}

func (s *emailsStub) SendInvitation(ctx context.Context, id model.InvitationID) error {
	s.invitation = id
	return s.err
}

func TestSendSignInLink(t *testing.T) {
	t.Run("should hand the address to the sender", func(t *testing.T) {
		s := &emailsStub{}

		is.NotError(t, jobs.SendSignInLink(s)(t.Context(), marshal(t, model.SendSignInLinkJobData{Email: "signe@example.com"})))
		is.Equal(t, model.EmailAddress("signe@example.com"), s.address)
	})

	t.Run("should return the error if the sender fails, so that the job is tried again", func(t *testing.T) {
		s := &emailsStub{err: errors.New("no")}

		err := jobs.SendSignInLink(s)(t.Context(), marshal(t, model.SendSignInLinkJobData{Email: "signe@example.com"}))
		is.Error(t, s.err, err)
	})
}

func TestSendInvitation(t *testing.T) {
	t.Run("should hand the invitation to the sender", func(t *testing.T) {
		s := &emailsStub{}

		is.NotError(t, jobs.SendInvitation(s)(t.Context(), marshal(t, model.SendInvitationJobData{InvitationID: "i_1"})))
		is.Equal(t, model.InvitationID("i_1"), s.invitation)
	})

	t.Run("should return the error if the sender fails, so that the job is tried again", func(t *testing.T) {
		s := &emailsStub{err: errors.New("no")}

		err := jobs.SendInvitation(s)(t.Context(), marshal(t, model.SendInvitationJobData{InvitationID: "i_1"}))
		is.Error(t, s.err, err)
	})
}

func TestFuncs(t *testing.T) {
	t.Run("should have a function for each job the app creates", func(t *testing.T) {
		funcs := jobs.Funcs(jobs.RegisterOpts{Emails: &emailsStub{}})

		is.Equal(t, 2, len(funcs))
		is.True(t, funcs[model.JobNameSendSignInLink] != nil)
		is.True(t, funcs[model.JobNameSendInvitation] != nil)
	})
}

func marshal(t *testing.T, data any) []byte {
	t.Helper()

	m, err := json.Marshal(data)
	is.NotError(t, err)
	return m
}
