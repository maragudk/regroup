// Package peopletest checks generated histories of a group against a model of the rules of signing in,
// roles, and invitations.
//
// A history is a sequence of actions: asking for sign-in links with addresses that are and are not in the
// group, looking at links and using them, inviting, inviting again, withdrawing, accepting, changing
// roles, people being made inactive, posting and looking at the timeline, and the clock moving past the
// links' expiry. [Run] draws one with hegel and runs it against a [Driver], which is one level of the app,
// such as the business logic. After every action it checks what the driver did against the model.
//
// The model is plain slices, written from the rules in docs/todo/people.md, not from the code:
//
//   - Every person has exactly one role: admin, moderator, or member. The first admin is the address the
//     configuration names. While the group has nobody in it, that address can get a sign-in link, and
//     signing in with it has them choose a name and join as an admin.
//   - Asking for a sign-in link with an address that is not one is refused. Any other address gets the
//     same answer. A link is sent only to an active person's address, or to the first admin's while the
//     group is empty, and to one address at most five times in an hour. Nothing else is sent.
//   - A sign-in link works for 15 minutes, until it is used. Looking at it does not use it up. Signing in
//     with it uses up every link to the same address. A person who is not active cannot sign in.
//   - A moderator or an admin invites an address that is not a person's. The invitation works for 7 days,
//     once, and inviting the address again replaces it. Moderators and admins see the invitations that
//     are waiting and withdraw them until they expire. Accepting one with a name that can be used makes a new active member,
//     invited by who sent it, who joins then.
//   - Only an admin changes a role, and nobody their own. So there is always an admin.
//   - Every active person sees the members' names and roles. Only admins see their addresses.
//   - A post by someone else is new to a reader if it was posted since the reader's previous look at the
//     timeline, and after they joined.
//   - Someone who is inactive or unknown can do nothing but what anyone can.
package peopletest

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"hegel.dev/go/hegel"

	"app/model"
)

// Email that the app sent.
type Email struct {
	To       model.EmailAddress
	Template string
	Subject  string
	Token    string
}

// Person in the group, as the app stores them.
type Person struct {
	ID        model.UserID
	Name      string
	Email     model.EmailAddress
	Role      model.Role
	Active    bool
	InvitedBy *model.UserID
}

// Post on the timeline as a reader sees it.
type Post struct {
	ID  model.PostID
	New bool
}

// Driver is one level of the app that a history runs against. A driver fails the test it was reset with
// on anything that is not an answer, such as an error from the database. Errors from the business domain,
// such as [model.ErrorLinkInvalid], are answers, and are returned.
type Driver interface {
	// Reset to a group with nobody in it, at the given time, with the given address of the first admin,
	// and randomness from the given seed.
	Reset(t testing.TB, now time.Time, firstAdmin model.EmailAddress, seed [32]byte)

	// SetClock to the given time, which is never before the time it was.
	SetClock(now time.Time)

	// Deactivate the given person, as moderation will.
	Deactivate(id model.UserID)

	// People in the group, active or not, in the order they joined.
	People() []Person

	// Emails sent since the last call, after every job that was waiting has run.
	Emails() []Email

	RequestSignInLink(address string) error
	CheckSignInLink(token string) (joining bool, err error)
	SignIn(token, name string) (model.UserID, error)
	Invite(actor model.UserID, address string) (model.InvitationID, error)
	GetInvitations(actor model.UserID) ([]model.Invitation, error)
	WithdrawInvitation(actor model.UserID, id model.InvitationID) error
	CheckInvitation(token string) (inviter string, err error)
	AcceptInvitation(token, name string) (model.UserID, error)
	ChangeRole(actor, target model.UserID, role string) error
	ListMembers(actor model.UserID) ([]model.Member, error)

	// Post something as the given person.
	Post(actor model.UserID) (model.PostID, error)

	// Look at the first page of the timeline as the given person, which holds every post.
	Look(actor model.UserID) ([]Post, error)
}

// addresses that histories use. The first admin is one of them, and the others may be invited.
var addresses = []model.EmailAddress{"ada@example.com", "bjorn@example.com", "chloe@example.com", "dee@example.com", "eve@example.com", "fred@example.com"}

// stranger is the ID of nobody in the group.
const stranger = model.UserID("u_nobody")

// name that can be chosen, with what it is stored as, or the error it is refused with.
type name struct {
	chosen string
	stored string
	err    error
}

var names = []name{
	{chosen: "Ada", stored: "Ada"},
	{chosen: "Bjørn", stored: "Bjørn"},
	{chosen: "  Chloé ", stored: "Chloé"},
	{chosen: "<i>Dee</i>", stored: "<i>Dee</i>"},
	{chosen: "Eve & \"Co\"", stored: "Eve & \"Co\""},
	{chosen: "Ada", stored: "Ada"},
}

var refusedNames = []name{
	{chosen: "", err: model.ErrorNameEmpty},
	{chosen: " \u200b ", err: model.ErrorNameEmpty},
	{chosen: "Ada\nLovelace", err: model.ErrorNameInvalid},
	{chosen: "Ada\u202e", err: model.ErrorNameInvalid},
	{chosen: strings.Repeat("a", 51), err: model.ErrorNameTooLong},
}

// Test runs generated histories against the given driver: the given number of them, of 50 steps each,
// unless the environment variables HISTORY_CASES and HISTORY_STEPS give other numbers, for a longer run.
func Test(t *testing.T, d Driver, cases int) {
	t.Helper()

	cases = fromEnv(t, "HISTORY_CASES", cases)
	steps := fromEnv(t, "HISTORY_STEPS", 50)
	hegel.Test(t, func(ht *hegel.T) {
		Run(ht, d, hegel.WithStatefulStepCount(steps))
	}, hegel.WithTestCases(cases))
}

// fromEnv is the number in the given environment variable, or the given number if it is not set.
func fromEnv(t *testing.T, name string, n int) int {
	t.Helper()

	s := os.Getenv(name)
	if s == "" {
		return n
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		t.Fatalf("%v is %q, which is not a positive number", name, s)
	}
	return n
}

// Run a generated history against the given driver, with the given options for the state machine. Most
// histories start with the first admin signed in, and the rest with nobody in the group.
func Run(ht *hegel.T, d Driver, opts ...hegel.StateMachineOption) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	start := from.Add(time.Duration(hegel.Draw(ht, hegel.Integers[int64](0, 15*365*24*3600*1000))) * time.Millisecond)
	firstAdmin := hegel.Draw(ht, hegel.SampledFrom(addresses))
	var seed [32]byte
	copy(seed[:], hegel.Draw(ht, hegel.Binary(32, 32)))
	ht.Note(fmt.Sprintf("from %v, with %v as the first admin", start.Format(time.RFC3339Nano), firstAdmin))

	m := &machine{d: d, now: start, firstAdmin: firstAdmin}
	func() {
		defer catch(ht)
		d.Reset(failer{TB: ht}, start, firstAdmin, seed)

		if hegel.Draw(ht, hegel.WeightedBooleans(0.8)) {
			m.ask(ht, firstAdmin.String())
			m.signIn(ht, m.links[len(m.links)-1].token, names[0])
		}
	}()

	hegel.RunStateful(ht, m, append([]hegel.StateMachineOption{hegel.WithAlwaysCheckInvariants("InvariantPeople", "InvariantAdmin", "InvariantNoEmail")}, opts...)...)
}

// failer is the test a driver fails when something is not an answer. Failing it fails the rule that is
// running, through [catch], so that hegel can shrink the history that led to it.
type failer struct {
	testing.TB
}

// failure of a driver, as a message.
type failure string

func (f failer) Error(args ...any) {
	panic(failure(fmt.Sprint(args...)))
}

func (f failer) Errorf(format string, args ...any) {
	panic(failure(fmt.Sprintf(format, args...)))
}

func (f failer) Fail() {
	panic(failure("the driver failed"))
}

func (f failer) FailNow() {
	panic(failure("the driver failed"))
}

func (f failer) Fatal(args ...any) {
	panic(failure(fmt.Sprint(args...)))
}

func (f failer) Fatalf(format string, args ...any) {
	panic(failure(fmt.Sprintf(format, args...)))
}

// catch a failure of a driver, and fail the given test case with it. Anything else panics on.
func catch(tc hegel.TestCase) {
	r := recover()
	if r == nil {
		return
	}
	if f, ok := r.(failure); ok {
		tc.Errorf("%v", string(f))
		return
	}
	panic(r)
}

// person in the model.
type person struct {
	id        model.UserID
	name      string
	email     model.EmailAddress
	role      model.Role
	active    bool
	invitedBy *model.UserID

	// looked is how many posts had been made at the person's latest look at the timeline, or when they
	// joined, if they have not looked since.
	looked int
}

// link in the model, from a sign-in email.
type link struct {
	token   string
	email   model.EmailAddress
	created time.Time
	expires time.Time
	used    bool
}

// invitation in the model. Its token is what its email held.
type invitation struct {
	id      model.InvitationID
	email   model.EmailAddress
	inviter model.UserID
	created time.Time
	expires time.Time
	token   string

	// gone is whether it was accepted, withdrawn, or replaced.
	gone bool
}

// post in the model.
type post struct {
	id     model.PostID
	author model.UserID
}

// machine runs a history against a driver and the model side by side.
type machine struct {
	d          Driver
	now        time.Time
	firstAdmin model.EmailAddress

	people      []*person
	links       []*link
	invitations []*invitation
	posts       []*post
}

func (m *machine) personByEmail(e model.EmailAddress) *person {
	for _, p := range m.people {
		if p.email == e {
			return p
		}
	}
	return nil
}

func (m *machine) personByID(id model.UserID) *person {
	for _, p := range m.people {
		if p.id == id {
			return p
		}
	}
	return nil
}

// signInTarget is who a working link to the given address signs in, and whether they join by it, or false
// if a link to the address does not work.
func (m *machine) signInTarget(e model.EmailAddress) (*person, bool, bool) {
	if p := m.personByEmail(e); p != nil {
		return p, false, p.active
	}
	if e == m.firstAdmin && len(m.people) == 0 {
		return nil, true, true
	}
	return nil, false, false
}

// expectError that the got error is the expected one, nil meaning none.
func expectError(tc hegel.TestCase, what string, expected, got error) {
	if expected == nil && got != nil || expected != nil && !errors.Is(got, expected) {
		tc.Errorf("%v gave %v, not %v", what, got, expected)
	}
}

// expectEmails that the driver sent exactly the given emails, by recipient and template, in order.
func (m *machine) expectEmails(tc hegel.TestCase, what string, expected []Email) []Email {
	got := m.d.Emails()
	match := len(got) == len(expected)
	for i := range got {
		if match && (got[i].To != expected[i].To || got[i].Template != expected[i].Template || got[i].Token == "") {
			match = false
		}
	}
	if !match {
		tc.Errorf("%v sent %+v, not %+v", what, got, expected)
	}
	return got
}

// spelling of the given address that a person might type: in other cases, and with white space around.
func spelling(tc hegel.TestCase, e model.EmailAddress) string {
	var b strings.Builder
	b.WriteString(hegel.Draw(tc, hegel.SampledFrom([]string{"", "", " ", "\t"})))
	for _, r := range e.String() {
		if hegel.Draw(tc, hegel.WeightedBooleans(0.2)) {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	b.WriteString(hegel.Draw(tc, hegel.SampledFrom([]string{"", "", " ", "\n"})))
	return b.String()
}

// address that a person might type: mostly a spelling of one of the addresses, and now and then one
// that is not an address.
func (m *machine) address(tc hegel.TestCase) (string, model.EmailAddress) {
	if hegel.Draw(tc, hegel.WeightedBooleans(0.05)) {
		return "not an address", ""
	}
	e := hegel.Draw(tc, hegel.SampledFrom(addresses))
	return spelling(tc, e), e
}

// actor draws someone in the group, often an active admin or moderator, or now and then a stranger, who
// is nil.
func (m *machine) actor(tc hegel.TestCase) (*person, model.UserID, string) {
	if len(m.people) == 0 || hegel.Draw(tc, hegel.WeightedBooleans(0.05)) {
		return nil, stranger, "a stranger"
	}
	var staff []*person
	for _, p := range m.people {
		if p.active && p.role != model.RoleMember {
			staff = append(staff, p)
		}
	}
	p := hegel.Draw(tc, hegel.SampledFrom(m.people))
	if len(staff) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.4)) {
		p = hegel.Draw(tc, hegel.SampledFrom(staff))
	}
	return p, p.id, m.who(p)
}

func (m *machine) who(p *person) string {
	s := fmt.Sprintf("%v (%v)", p.name, p.role)
	if !p.active {
		s += " (inactive)"
	}
	return s
}

// authorized is the error that the given actor gets for an action only the given roles may take, or nil.
func authorized(p *person, roles ...model.Role) error {
	switch {
	case p == nil:
		return model.ErrorUserNotFound
	case !p.active:
		return model.ErrorUserInactive
	case !slices.Contains(roles, p.role):
		return model.ErrorNotAllowed
	}
	return nil
}

// drawName that someone chooses: mostly one that can be used.
func drawName(tc hegel.TestCase) name {
	if hegel.Draw(tc, hegel.WeightedBooleans(0.15)) {
		return hegel.Draw(tc, hegel.SampledFrom(refusedNames))
	}
	return hegel.Draw(tc, hegel.SampledFrom(names))
}

// garbage tokens, which no link has: empty, not a token at all, and one that could be a token.
var garbage = []string{"", "garbage", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}

// RuleAskLink has someone ask for a sign-in link with an address.
func (m *machine) RuleAskLink(tc hegel.TestCase) {
	defer catch(tc)

	typed, _ := m.address(tc)
	m.ask(tc, typed)
}

// ask for a sign-in link with the given address, as typed.
func (m *machine) ask(tc hegel.TestCase, typed string) {
	tc.Note(fmt.Sprintf("someone asks for a sign-in link with %q", typed))
	err := m.d.RequestSignInLink(typed)

	e, parseErr := model.ParseEmailAddress(typed)
	if parseErr != nil {
		expectError(tc, "asking with what is not an address", model.ErrorEmailInvalid, err)
		m.expectEmails(tc, "asking with what is not an address", nil)
		return
	}
	expectError(tc, "asking", nil, err)

	var expected []Email
	if _, _, ok := m.signInTarget(e); ok {
		var recent int
		for _, l := range m.links {
			if l.email == e && l.created.After(m.now.Add(-time.Hour)) {
				recent++
			}
		}
		if recent < 5 {
			expected = []Email{{To: e, Template: "sign-in"}}
		}
	}
	tc.Event(fmt.Sprint("ask: ", len(expected), " sent"))

	for _, sent := range m.expectEmails(tc, "asking with "+e.String(), expected) {
		m.links = append(m.links, &link{token: sent.Token, email: e, created: m.now, expires: m.now.Add(15 * time.Minute)})
	}
}

// RuleAskRepeatedly has someone ask for sign-in links with one address several times in a row, around
// the limit of links an address gets in an hour.
func (m *machine) RuleAskRepeatedly(tc hegel.TestCase) {
	defer catch(tc)

	_, e := m.address(tc)
	tc.Assume(e != "")
	for range hegel.Draw(tc, hegel.Integers(4, 7)) {
		m.ask(tc, spelling(tc, e))
	}
}

// drawLinkToken of a sign-in link sent, or of an invitation, or garbage, with the link if it is one.
func (m *machine) drawLinkToken(tc hegel.TestCase) (string, *link) {
	switch {
	case len(m.links) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.6)):
		l := m.links[len(m.links)-1]
		return l.token, l
	case len(m.links) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.6)):
		l := hegel.Draw(tc, hegel.SampledFrom(m.links))
		return l.token, l
	case len(m.invitations) > 0 && hegel.Draw(tc, hegel.Booleans()):
		inv := hegel.Draw(tc, hegel.SampledFrom(m.invitations))
		return inv.token, nil
	default:
		return hegel.Draw(tc, hegel.SampledFrom(garbage)), nil
	}
}

// works is whether the given link works now, and who it signs in or whether they join by it.
func (m *machine) works(l *link) (*person, bool, bool) {
	if l == nil || l.used || !m.now.Before(l.expires) {
		return nil, false, false
	}
	return m.signInTarget(l.email)
}

// RuleCheckLink has someone open the page a sign-in link leads to.
func (m *machine) RuleCheckLink(tc hegel.TestCase) {
	defer catch(tc)

	token, l := m.drawLinkToken(tc)
	tc.Note(fmt.Sprintf("someone opens the sign-in link %q", token))
	joining, err := m.d.CheckSignInLink(token)

	_, expectedJoining, ok := m.works(l)
	tc.Event(fmt.Sprint("check link: works ", ok))
	if !ok {
		expectError(tc, "opening the link", model.ErrorLinkInvalid, err)
		return
	}
	expectError(tc, "opening the link", nil, err)
	if joining != expectedJoining {
		tc.Errorf("opening the link said joining is %v, not %v", joining, expectedJoining)
	}
}

// RuleSignIn has someone sign in with a sign-in link.
func (m *machine) RuleSignIn(tc hegel.TestCase) {
	defer catch(tc)

	token, _ := m.drawLinkToken(tc)
	m.signIn(tc, token, drawName(tc))
}

// signIn with the given token, choosing the given name.
func (m *machine) signIn(tc hegel.TestCase, token string, n name) {
	var l *link
	for _, candidate := range m.links {
		if candidate.token == token {
			l = candidate
		}
	}
	tc.Note(fmt.Sprintf("someone signs in with the link %q, choosing the name %q", token, n.chosen))
	id, err := m.d.SignIn(token, n.chosen)

	p, joining, ok := m.works(l)
	tc.Event(fmt.Sprint("sign in: works ", ok, ", joining ", joining))
	switch {
	case !ok:
		expectError(tc, "signing in", model.ErrorLinkInvalid, err)
		return
	case joining && n.err != nil:
		expectError(tc, "joining with a name that cannot be used", n.err, err)
		return
	}
	expectError(tc, "signing in", nil, err)

	if joining {
		if id == "" || m.personByID(id) != nil {
			tc.Errorf("the first admin joined as %q, which is empty or someone else's", id)
		}
		m.people = append(m.people, &person{id: id, name: n.stored, email: l.email, role: model.RoleAdmin, active: true, looked: len(m.posts)})
	} else if id != p.id {
		tc.Errorf("the link signed in %v, not %v", id, p.id)
	}

	for _, other := range m.links {
		if other.email == l.email {
			other.used = true
		}
	}
}

// RuleInvite has someone invite an address.
func (m *machine) RuleInvite(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	typed, e := m.address(tc)
	var outside []model.EmailAddress
	for _, a := range addresses {
		if m.personByEmail(a) == nil {
			outside = append(outside, a)
		}
	}
	if len(outside) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.6)) {
		e = hegel.Draw(tc, hegel.SampledFrom(outside))
		typed = spelling(tc, e)
	}
	m.invite(tc, p, id, who, typed, e)
}

// RuleJoin has an active admin or moderator invite an address that is not anyone's, and the invitation be
// accepted with a name that can be used, which is the way most people join.
func (m *machine) RuleJoin(tc hegel.TestCase) {
	defer catch(tc)

	var staff []*person
	for _, p := range m.people {
		if p.active && p.role != model.RoleMember {
			staff = append(staff, p)
		}
	}
	var outside []model.EmailAddress
	for _, a := range addresses {
		if m.personByEmail(a) == nil {
			outside = append(outside, a)
		}
	}
	tc.Assume(len(staff) > 0 && len(outside) > 0)
	p := hegel.Draw(tc, hegel.SampledFrom(staff))
	e := hegel.Draw(tc, hegel.SampledFrom(outside))
	m.invite(tc, p, p.id, m.who(p), spelling(tc, e), e)
	inv := m.invitations[len(m.invitations)-1]
	m.accept(tc, inv.token, inv, hegel.Draw(tc, hegel.SampledFrom(names)))
}

// invite the given address, as typed, as the given actor.
func (m *machine) invite(tc hegel.TestCase, p *person, id model.UserID, who, typed string, e model.EmailAddress) {
	tc.Note(fmt.Sprintf("%v invites %q", who, typed))
	invID, err := m.d.Invite(id, typed)

	expected := authorized(p, model.RoleAdmin, model.RoleModerator)
	if expected == nil {
		switch existing := m.personByEmail(e); {
		case e == "":
			expected = model.ErrorEmailInvalid
		case existing != nil && existing.active:
			expected = model.ErrorAlreadyMember
		case existing != nil:
			expected = model.ErrorFormerMember
		}
	}
	tc.Event(fmt.Sprint("invite: ", expected))
	expectError(tc, "inviting", expected, err)
	if expected != nil {
		m.expectEmails(tc, "a refused invitation", nil)
		return
	}

	for _, inv := range m.invitations {
		if inv.email == e && !inv.gone {
			inv.gone = true
		}
	}
	inv := &invitation{id: invID, email: e, inviter: id, created: m.now, expires: m.now.Add(7 * 24 * time.Hour)}
	m.invitations = append(m.invitations, inv)

	sent := m.expectEmails(tc, "inviting "+e.String(), []Email{{To: e, Template: "invitation"}})
	if len(sent) == 1 {
		inv.token = sent[0].Token
		if expected := p.name + " invited you to The Loft"; sent[0].Subject != expected {
			tc.Errorf("the invitation's subject is %q, not %q", sent[0].Subject, expected)
		}
	}
}

// waiting invitations, the newest first.
func (m *machine) waiting() []*invitation {
	var invs []*invitation
	for _, inv := range slices.Backward(m.invitations) {
		if !inv.gone && m.now.Before(inv.expires) {
			invs = append(invs, inv)
		}
	}
	return invs
}

// RuleListInvitations has someone list the invitations that are waiting.
func (m *machine) RuleListInvitations(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Note(fmt.Sprintf("%v lists the invitations", who))
	got, err := m.d.GetInvitations(id)

	expected := authorized(p, model.RoleAdmin, model.RoleModerator)
	expectError(tc, "listing invitations", expected, err)
	if expected != nil {
		if len(got) > 0 {
			tc.Errorf("someone who may not see invitations got %+v", got)
		}
		return
	}

	waiting := m.waiting()
	match := len(got) == len(waiting)
	for i := range got {
		if match && (got[i].ID != waiting[i].id || got[i].Email != waiting[i].email || got[i].InvitedBy != waiting[i].inviter || !got[i].Expires.T.Equal(waiting[i].expires)) {
			match = false
		}
	}
	if !match {
		tc.Errorf("the invitations are %+v, not %v of them", got, len(waiting))
	}
}

// RuleWithdraw has someone withdraw an invitation.
func (m *machine) RuleWithdraw(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	var inv *invitation
	invID := model.InvitationID("i_nope")
	if len(m.invitations) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.9)) {
		inv = hegel.Draw(tc, hegel.SampledFrom(m.invitations))
		invID = inv.id
	}
	tc.Note(fmt.Sprintf("%v withdraws the invitation %v", who, invID))
	err := m.d.WithdrawInvitation(id, invID)

	expected := authorized(p, model.RoleAdmin, model.RoleModerator)
	if expected == nil && (inv == nil || inv.gone || !m.now.Before(inv.expires)) {
		expected = model.ErrorInvitationNotFound
	}
	tc.Event(fmt.Sprint("withdraw: ", expected))
	expectError(tc, "withdrawing", expected, err)
	if expected == nil {
		inv.gone = true
	}
}

// drawInvitationToken of an invitation sent, or of a sign-in link, or garbage, with the invitation if it
// is one.
func (m *machine) drawInvitationToken(tc hegel.TestCase) (string, *invitation) {
	switch {
	case len(m.invitations) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.6)):
		inv := m.invitations[len(m.invitations)-1]
		return inv.token, inv
	case len(m.invitations) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.6)):
		inv := hegel.Draw(tc, hegel.SampledFrom(m.invitations))
		return inv.token, inv
	case len(m.links) > 0 && hegel.Draw(tc, hegel.Booleans()):
		return hegel.Draw(tc, hegel.SampledFrom(m.links)).token, nil
	default:
		return hegel.Draw(tc, hegel.SampledFrom(garbage)), nil
	}
}

func (m *machine) invitationWorks(inv *invitation) bool {
	return inv != nil && !inv.gone && m.now.Before(inv.expires)
}

// RuleCheckInvitation has someone open the page an invitation leads to.
func (m *machine) RuleCheckInvitation(tc hegel.TestCase) {
	defer catch(tc)

	token, inv := m.drawInvitationToken(tc)
	tc.Note(fmt.Sprintf("someone opens the invitation %q", token))
	inviter, err := m.d.CheckInvitation(token)

	ok := m.invitationWorks(inv)
	tc.Event(fmt.Sprint("check invitation: works ", ok))
	if !ok {
		expectError(tc, "opening the invitation", model.ErrorLinkInvalid, err)
		return
	}
	expectError(tc, "opening the invitation", nil, err)
	if expected := m.personByID(inv.inviter).name; inviter != expected {
		tc.Errorf("the invitation says it is from %q, not %q", inviter, expected)
	}
}

// RuleAccept has someone accept an invitation, choosing a name.
func (m *machine) RuleAccept(tc hegel.TestCase) {
	defer catch(tc)

	token, inv := m.drawInvitationToken(tc)
	m.accept(tc, token, inv, drawName(tc))
}

// accept the invitation with the given token, which is the given invitation or none, choosing the given
// name.
func (m *machine) accept(tc hegel.TestCase, token string, inv *invitation, n name) {
	tc.Note(fmt.Sprintf("someone accepts the invitation %q, choosing the name %q", token, n.chosen))
	id, err := m.d.AcceptInvitation(token, n.chosen)

	ok := m.invitationWorks(inv)
	tc.Event(fmt.Sprint("accept: works ", ok))
	switch {
	case !ok:
		expectError(tc, "accepting", model.ErrorLinkInvalid, err)
		return
	case n.err != nil:
		expectError(tc, "accepting with a name that cannot be used", n.err, err)
		return
	}
	expectError(tc, "accepting", nil, err)

	if id == "" || m.personByID(id) != nil {
		tc.Errorf("the invitation made %q, which is empty or someone else", id)
	}
	inviter := inv.inviter
	m.people = append(m.people, &person{id: id, name: n.stored, email: inv.email, role: model.RoleMember, active: true, invitedBy: &inviter, looked: len(m.posts)})
	inv.gone = true
}

// RuleChangeRole has someone change someone's role, perhaps their own.
func (m *machine) RuleChangeRole(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Assume(len(m.people) > 0)
	target := hegel.Draw(tc, hegel.SampledFrom(m.people))
	var others []*person
	for _, q := range m.people {
		if q != p && q.active {
			others = append(others, q)
		}
	}
	switch {
	case p != nil && hegel.Draw(tc, hegel.WeightedBooleans(0.2)):
		target = p
	case len(others) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.6)):
		target = hegel.Draw(tc, hegel.SampledFrom(others))
	}
	role := hegel.Draw(tc, hegel.SampledFrom([]string{"admin", "moderator", "member", "admin", "moderator", "member", "owner"}))
	tc.Note(fmt.Sprintf("%v makes %v a %v", who, m.who(target), role))
	err := m.d.ChangeRole(id, target.id, role)

	var expected error
	switch {
	case p == nil:
		expected = model.ErrorUserNotFound
	case !p.active:
		expected = model.ErrorUserInactive
	case p.role != model.RoleAdmin || p == target:
		expected = model.ErrorNotAllowed
	case !target.active:
		expected = model.ErrorMemberNotFound
	case role == "owner":
		expected = model.ErrorRoleInvalid
	}
	tc.Event(fmt.Sprint("change role: ", expected))
	expectError(tc, "changing the role", expected, err)
	if expected == nil {
		target.role = model.Role(role)
	}
}

// RuleListMembers has someone list the members.
func (m *machine) RuleListMembers(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Note(fmt.Sprintf("%v lists the members", who))
	got, err := m.d.ListMembers(id)

	expected := authorized(p, model.RoleAdmin, model.RoleModerator, model.RoleMember)
	expectError(tc, "listing members", expected, err)
	if expected != nil {
		if len(got) > 0 {
			tc.Errorf("someone who may not see the members got %+v", got)
		}
		return
	}

	var active []*person
	for _, q := range m.people {
		if q.active {
			active = append(active, q)
		}
	}
	match := len(got) == len(active)
	for i := range got {
		if !match {
			break
		}
		email := model.EmailAddress("")
		if p.role == model.RoleAdmin {
			email = active[i].email
		}
		match = got[i].ID == active[i].id && got[i].Name == active[i].name && got[i].Role == active[i].role && got[i].Email == email
	}
	if !match {
		tc.Errorf("%v got the members %+v", who, got)
	}
}

// RuleDeactivate makes someone inactive, as moderation will, unless they are the last active admin, since
// the rules of moderation are not decided yet.
func (m *machine) RuleDeactivate(tc hegel.TestCase) {
	defer catch(tc)

	var candidates []*person
	var admins int
	for _, p := range m.people {
		if p.active && p.role == model.RoleAdmin {
			admins++
		}
	}
	for _, p := range m.people {
		if p.active && (p.role != model.RoleAdmin || admins > 1) {
			candidates = append(candidates, p)
		}
	}
	tc.Assume(len(candidates) > 0)
	p := hegel.Draw(tc, hegel.SampledFrom(candidates))
	tc.Note(fmt.Sprintf("%v is made inactive", m.who(p)))
	m.d.Deactivate(p.id)
	p.active = false
}

// RuleAdvanceClock moves the clock on: often to just before or at a link's expiry, or by any number of
// milliseconds up to twenty minutes or up to eight days.
func (m *machine) RuleAdvanceClock(tc hegel.TestCase) {
	defer catch(tc)

	millis := func(maxDuration time.Duration) hegel.Generator[time.Duration] {
		return hegel.Map(hegel.Integers[int64](0, int64(maxDuration/time.Millisecond)), func(n int64) time.Duration { return time.Duration(n) * time.Millisecond })
	}
	d := hegel.Draw(tc, hegel.OneOf(
		hegel.SampledFrom([]time.Duration{time.Millisecond, time.Minute, 15*time.Minute - time.Millisecond, 15 * time.Minute, time.Hour, 24 * time.Hour, 7*24*time.Hour - time.Millisecond, 7 * 24 * time.Hour}),
		millis(20*time.Minute),
		millis(8*24*time.Hour),
	))
	m.now = m.now.Add(d)
	tc.Note(fmt.Sprintf("the clock moves on by %v", d))
	m.d.SetClock(m.now)
}

// RulePost has someone post.
func (m *machine) RulePost(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Assume(len(m.posts) < 50)
	tc.Note(fmt.Sprintf("%v posts", who))
	postID, err := m.d.Post(id)

	expected := authorized(p, model.RoleAdmin, model.RoleModerator, model.RoleMember)
	expectError(tc, "posting", expected, err)
	if expected == nil {
		m.posts = append(m.posts, &post{id: postID, author: id})
	}
}

// RuleLook has someone look at the timeline: what was posted by others since their previous look, or
// since they joined, is new to them.
func (m *machine) RuleLook(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Note(fmt.Sprintf("%v looks at the timeline", who))
	got, err := m.d.Look(id)

	expected := authorized(p, model.RoleAdmin, model.RoleModerator, model.RoleMember)
	expectError(tc, "looking", expected, err)
	if expected != nil {
		if len(got) > 0 {
			tc.Errorf("someone who may not look got %+v", got)
		}
		return
	}

	match := len(got) == len(m.posts)
	for i := range got {
		if !match {
			break
		}
		// The timeline is the newest first, and every post here has the same time or a later one.
		index := len(m.posts) - 1 - i
		q := m.posts[index]
		match = got[i].ID == q.id && got[i].New == (q.author != id && index >= p.looked)
	}
	if !match {
		tc.Errorf("%v saw %+v, with %v posts, of which they had seen %v", who, got, len(m.posts), p.looked)
	}
	p.looked = len(m.posts)
}

// InvariantPeople is that the group has exactly the people the model has, with the names, addresses,
// roles, and inviters it has: so every invitation made one member at most, and no role changed but by an
// admin's action on someone else.
func (m *machine) InvariantPeople(tc hegel.TestCase) {
	defer catch(tc)

	got := m.d.People()
	match := len(got) == len(m.people)
	for i := range got {
		if !match {
			break
		}
		p := m.people[i]
		match = got[i].ID == p.id && got[i].Name == p.name && got[i].Email == p.email && got[i].Role == p.role && got[i].Active == p.active &&
			(got[i].InvitedBy == nil) == (p.invitedBy == nil) && (p.invitedBy == nil || *got[i].InvitedBy == *p.invitedBy)
	}
	if !match {
		var expected []string
		for _, p := range m.people {
			expected = append(expected, fmt.Sprintf("%v %v %v %v", p.id, p.name, p.role, p.active))
		}
		tc.Errorf("the group is %+v, not %v", got, expected)
	}
}

// InvariantAdmin is that a group with anyone in it has an active admin.
func (m *machine) InvariantAdmin(tc hegel.TestCase) {
	defer catch(tc)

	people := m.d.People()
	if len(people) == 0 {
		return
	}
	if !slices.ContainsFunc(people, func(p Person) bool { return p.Active && p.Role == model.RoleAdmin }) {
		tc.Errorf("the group has no active admin: %+v", people)
	}
}

// InvariantNoEmail is that no email was sent that a rule did not expect, such as one to an address that
// is not in the group.
func (m *machine) InvariantNoEmail(tc hegel.TestCase) {
	defer catch(tc)

	if sent := m.d.Emails(); len(sent) > 0 {
		tc.Errorf("emails were sent that nobody asked for: %+v", sent)
	}
}
