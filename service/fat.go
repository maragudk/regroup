// Package service provides business logic.
// See https://www.alexedwards.net/blog/the-fat-service-pattern
package service

import (
	"context"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"maragu.dev/glue/email"
	"maragu.dev/glue/s3"

	"app/model"
	"app/sqlite"
)

// Fat holds the business logic, one exported method per operation.
//
// It carries only what every operation needs: the logger and the tracer. Capabilities belong to each
// operation's wiring function (such as [GetUser]), which sets that operation's func field from what it is
// given — so an operation cannot reach a capability it did not declare, since Fat holds none itself,
// and a method panics if never wired.
//
// The func fields are written once by the wiring functions and only read after that, so a wired Fat
// is safe for concurrent use. Wiring an operation that is already wired panics.
type Fat struct {
	log    *slog.Logger
	tracer trace.Tracer

	acceptInvitation   func(ctx context.Context, token, name string) (model.User, error)
	changeRole         func(ctx context.Context, actor, target model.UserID, role string) error
	checkInvitation    func(ctx context.Context, token string) (model.InvitationCheck, error)
	checkSignInLink    func(ctx context.Context, token string) (model.SignInLinkCheck, error)
	createComment      func(ctx context.Context, actor model.UserID, postID model.PostID, token, body string) (model.Comment, error)
	createPost         func(ctx context.Context, actor model.UserID, token, body string) (model.Post, error)
	getComments        func(ctx context.Context, actor model.UserID, postID model.PostID) ([]model.TimelineComment, error)
	getInvitations     func(ctx context.Context, actor model.UserID) ([]model.Invitation, error)
	getMembers         func(ctx context.Context, actor model.UserID) (int, error)
	getTimeline        func(ctx context.Context, actor model.UserID, cursor string, limit int) (model.Timeline, error)
	getUser            func(ctx context.Context, id model.UserID) (model.User, error)
	invite             func(ctx context.Context, actor model.UserID, address string) (model.Invitation, error)
	listMembers        func(ctx context.Context, actor model.UserID) ([]model.Member, error)
	requestSignInLink  func(ctx context.Context, address string) error
	sendInvitation     func(ctx context.Context, id model.InvitationID) error
	sendSignInLink     func(ctx context.Context, address model.EmailAddress) error
	signIn             func(ctx context.Context, token, name string) (model.User, error)
	withdrawInvitation func(ctx context.Context, actor model.UserID, id model.InvitationID) error
}

// NewFatOptions is the configuration a [Fat] carries whatever it ends up wired to. The capabilities
// belong to the wiring functions, and the tracer is the package's own.
type NewFatOptions struct {
	Log *slog.Logger
}

// NewFat with no operation wired: the wiring functions wire one operation each, [Setup] all of them
// at once. A nil Log discards.
func NewFat(opts NewFatOptions) *Fat {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}

	return &Fat{
		log:    opts.Log,
		tracer: otel.Tracer("app/service"),
	}
}

// Setup every operation of the given [Fat] with the real capabilities, named concretely: the narrow
// interfaces exist for the operations rather than for this.
//
// The wiring functions it calls are the list of what each operation actually depends on. A capability
// that no operation wires yet is a parameter all the same, so the first operation to need one finds it
// already plumbed: bucket is waiting like that.
//
// The clock and the randomness are passed in, so that no operation reads the time or makes a secret
// directly. The randomness must be safe for concurrent use, as [crypto/rand.Reader] is. The first admin
// is the address of the person who may join as the first admin while the group has nobody in it, or
// empty if nobody may.
func Setup(f *Fat, bucket *s3.Bucket, db *sqlite.Database, sender email.Sender, now func() time.Time, random io.Reader, group model.Group, firstAdmin model.EmailAddress) {
	AcceptInvitation(f, db, now)
	ChangeRole(f, db)
	CheckInvitation(f, db, now)
	CheckSignInLink(f, db, now, firstAdmin)
	CreateComment(f, db, now)
	CreatePost(f, db, now)
	GetComments(f, db)
	GetInvitations(f, db, now)
	GetMembers(f, db)
	GetTimeline(f, db, now, group)
	GetUser(f, db)
	Invite(f, db, now)
	ListMembers(f, db)
	RequestSignInLink(f, db)
	SendInvitation(f, db, sender, now, random, group)
	SendSignInLink(f, db, sender, now, random, group, firstAdmin)
	SignIn(f, db, now, firstAdmin)
	WithdrawInvitation(f, db, now)
}

// userGetter is the store a user is read from.
type userGetter interface {
	GetUser(ctx context.Context, id model.UserID) (model.User, error)
}

// GetUser wires [Fat.GetUser] to the given store.
//
// Panics if the operation is already wired, since that is a composition mistake rather than a way to
// swap a store out from under a running operation.
func GetUser(f *Fat, db userGetter) {
	if f.getUser != nil {
		panic("service: GetUser already wired")
	}

	f.getUser = db.GetUser
}

// GetUser with the given ID.
//
// Panics unless the operation was wired, by [Setup] or by the function of the same name.
func (f *Fat) GetUser(ctx context.Context, id model.UserID) (model.User, error) {
	if f.getUser == nil {
		panic("service: GetUser not wired; call service.GetUser or service.Setup")
	}

	return f.getUser(ctx, id)
}
