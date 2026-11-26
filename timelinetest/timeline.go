// Package timelinetest checks generated histories of a group against a model of the timeline's rules.
//
// A history is a sequence of actions by several people: joining, looking at the timeline, loading
// earlier pages, posting, commenting, opening comments, sending a form again, being made inactive, and
// the clock moving on. [Run] draws one with hegel and runs it against a [Driver], which is one level of
// the app, such as the business logic or the router as a browser sees it. After every action it checks
// what the driver shows against the model.
//
// The model is plain slices and maps, written from the rules in docs/todo/group-page.md and the diary of
// the first group page, not from the code:
//
//   - The timeline holds every post, newest first by the time it was posted, and by the order posts were
//     made when two have the same time. A page holds a fixed number of posts, and a cursor gets the posts
//     after the last post of the page it came from. Posts are grouped into days by their date in the
//     group's time zone, and a day is labelled today or yesterday by the clock.
//   - A post by someone else is new to a reader if it was posted since the reader's previous look at the
//     timeline. Getting the first page is a look. A page after the first is marked against the same look
//     as the first page of the reader's latest look.
//   - A comment by someone else is new to a reader until the reader opens that post's comments.
//   - Nothing a person wrote is ever new to them, and nothing from before they joined is new to them.
//   - A form sent again with a token its author used before makes nothing. With the same text once
//     cleaned, it is answered as the first time. With other text, it is answered with what the first
//     sending made and a word that this text was not posted. A refused body uses no token.
//   - Someone who is inactive or unknown gets nothing and changes nothing.
package timelinetest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"hegel.dev/go/hegel"
	"hegel.dev/go/hegel/temporal"

	"app/model"
)

// Outcome of an action, as the person who did it is told.
type Outcome int

const (
	// Done is an action that did what was asked, including a form sent again with the same text.
	Done Outcome = iota
	// Refused is a body that cannot be posted, such as an empty one.
	Refused
	// Changed is a form sent again with other text than the first time, which made nothing.
	Changed
	// NotFound is a post that is not there.
	NotFound
	// Private is the answer to someone who is not signed in, inactive, or unknown.
	Private
	// BadRequest is a request the page could not have sent, such as one with a token that is not one.
	BadRequest
)

// String satisfies [fmt.Stringer].
func (o Outcome) String() string {
	switch o {
	case Done:
		return "done"
	case Refused:
		return "refused"
	case Changed:
		return "changed"
	case NotFound:
		return "not found"
	case Private:
		return "private"
	case BadRequest:
		return "bad request"
	default:
		return fmt.Sprint("outcome ", int(o))
	}
}

// OutcomeOf the given error from the business logic. It is false if the error is not one of the outcomes.
func OutcomeOf(err error) (Outcome, bool) {
	switch {
	case err == nil:
		return Done, true
	case errors.Is(err, model.ErrorBodyEmpty), errors.Is(err, model.ErrorBodyTooLong), errors.Is(err, model.ErrorBodyInvalid):
		return Refused, true
	case errors.Is(err, model.ErrorFormResentChanged):
		return Changed, true
	case errors.Is(err, model.ErrorPostNotFound):
		return NotFound, true
	case errors.Is(err, model.ErrorUserInactive), errors.Is(err, model.ErrorUserNotFound):
		return Private, true
	case errors.Is(err, model.ErrorFormTokenInvalid), errors.Is(err, model.ErrorCursorInvalid):
		return BadRequest, true
	default:
		return 0, false
	}
}

// Post as a reader sees it.
type Post struct {
	ID     model.PostID
	Author string
	Body   string

	// Time it was posted, in the group's time zone, in [time.RFC3339].
	Time string

	New         bool
	Comments    int
	NewComments int
}

// Comment as a reader sees it.
type Comment struct {
	ID     model.CommentID
	Author string
	Body   string

	// Time it was written, in the group's time zone, in [time.RFC3339].
	Time string

	New bool
}

// Day of a [Page].
type Day struct {
	Date model.Date

	// Heading is whether the answer drew the day's heading, and so whether Today and Yesterday say
	// anything. A page that already has a day's heading gets more posts under it without one.
	Heading bool

	Today, Yesterday bool
	Posts            []Post
}

// Page of the timeline, as one answer shows it.
type Page struct {
	Days []Day

	// Next is the cursor of the page after this one, or nil if there is none.
	Next *Cursor

	// Members is how many people the page says are in the group, or -1 if it does not say.
	Members int
}

// Cursor of a page after another. After is what the business logic made. Days and Shown are what a page
// in a browser knows about the days it has headings for, which only a driver that reads pages uses.
type Cursor struct {
	After string
	Days  string
	Shown []model.Date
}

// PageAnswer to a look at the timeline or a request for a page after a cursor.
type PageAnswer struct {
	Outcome Outcome
	Page    Page

	// Shown is all the text the answer shows, for checking that someone who gets nothing learns nothing.
	Shown string
}

// PostAnswer to a post.
type PostAnswer struct {
	Outcome Outcome

	// Post that the answer shows: the one made, or the one made the first time with the same form.
	Post Post

	// Field is the text left in the form's field, or nil if the driver has no form to show.
	Field *string

	Shown string
}

// CommentAnswer to a comment.
type CommentAnswer struct {
	Outcome Outcome

	// Comment that the answer shows, if the driver can tell which: the one made, or the one made the
	// first time with the same form. Its ID is empty otherwise.
	Comment Comment

	// Opened is whether the answer opened the comments of a post, as the group page does after a comment.
	// OpenedPost is that post, and Comments its comments.
	Opened     bool
	OpenedPost model.PostID
	Comments   []Comment

	// Field is the text left in the form's field, or nil if the driver has no form to show.
	Field *string

	Shown string
}

// CommentsAnswer to opening the comments of a post.
type CommentsAnswer struct {
	Outcome  Outcome
	Comments []Comment
	Shown    string
}

// Driver is one level of the app that a history runs against. A driver fails the test it was reset with
// on anything that is not an answer, such as an error from the database, or an answer a browser could not
// read.
type Driver interface {
	// Reset to a group with nobody in it and nothing posted, in the given time zone, at the given time,
	// with the given number of posts on a page of the timeline.
	Reset(t testing.TB, zone *time.Location, now time.Time, pageSize int)

	// Join a new, active person with the given name to the group, and return their ID.
	Join(name string) model.UserID

	// Deactivate the given person.
	Deactivate(id model.UserID)

	// SetClock to the given time, which is never before the time it was.
	SetClock(now time.Time)

	// Look at the timeline as the given person: get its first page.
	Look(actor model.UserID) PageAnswer

	// Earlier gets the page after the given cursor as the given person.
	Earlier(actor model.UserID, c Cursor) PageAnswer

	// Walk the whole timeline as the given person, from before its newest post to its end, page by page,
	// without looking at it. Each answer is one page.
	Walk(actor model.UserID) ([]PageAnswer, Outcome)

	// Post as the given person, from the form with the given token.
	Post(actor model.UserID, token, body string) PostAnswer

	// Comment as the given person on the given post, from the form with the given token.
	Comment(actor model.UserID, post model.PostID, token, body string) CommentAnswer

	// Open the comments of the given post as the given person.
	Open(actor model.UserID, post model.PostID) CommentsAnswer
}

// zones that histories run in: UTC, one with daylight saving, one at UTC+14, one at a half hour from UTC
// with daylight saving, and one at a quarter hour from UTC with daylight saving, far from UTC.
var zones = []string{"UTC", "Europe/Copenhagen", "Pacific/Kiritimati", "America/St_Johns", "Pacific/Chatham"}

// names of the people who can join, in the order they join. Some have characters that mean something in
// HTML.
var names = []string{"Ada", "Bjørn", "Chloé", "<i>Dee</i>", "Eve & \"Co\""}

// stranger is the ID of nobody in the group.
const stranger = model.UserID("u_nobody")

// Test runs generated histories against the given driver: the given number of them, of 30 steps each,
// unless the environment variables HISTORY_CASES and HISTORY_STEPS give other numbers, for a longer run.
func Test(t *testing.T, d Driver, cases int) {
	t.Helper()

	cases = fromEnv(t, "HISTORY_CASES", cases)
	steps := fromEnv(t, "HISTORY_STEPS", 30)
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

// Run a generated history against the given driver, with the given options for the state machine.
func Run(ht *hegel.T, d Driver, opts ...hegel.StateMachineOption) {
	zone, err := time.LoadLocation(hegel.Draw(ht, hegel.SampledFrom(zones)))
	if err != nil {
		ht.Fatal(err)
	}
	start := hegel.Draw(ht, temporal.ZonedDatetimes().Zones(hegel.Just(zone)).
		Min(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)).Max(time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)).
		Precision(temporal.PrecisionMilliseconds))
	pageSize := hegel.Draw(ht, hegel.Integers(1, 4))
	ht.Note(fmt.Sprintf("in %v from %v, %v posts a page", zone, start.Format(time.RFC3339Nano), pageSize))

	m := &machine{d: d, zone: zone, now: start, pageSize: pageSize}
	func() {
		defer catch(ht)
		d.Reset(failer{TB: ht}, zone, start, pageSize)
		m.join(ht)
	}()

	hegel.RunStateful(ht, m, append([]hegel.StateMachineOption{hegel.WithAlwaysCheckInvariants("InvariantTimelines")}, opts...)...)
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
	index  int
	id     model.UserID
	name   string
	active bool

	// joined, looked, and basis are what had been made when the person joined, at their latest look, and
	// at the look before that, as counted by [machine.made].
	joined, looked, basis int

	// opened is what had been made when the person last opened a post's comments, by the post's index.
	opened map[int]int

	// sent is what the person made with each token, by whether it was a comment and the token.
	sent map[sentKey]int
}

type sentKey struct {
	comment bool
	token   string
}

// modelPost in the model.
type modelPost struct {
	id       model.PostID
	author   int
	body     string
	created  time.Time
	made     int
	comments []int
}

// modelComment in the model.
type modelComment struct {
	id      model.CommentID
	post    int
	author  int
	body    string
	created time.Time
	made    int
}

// kept is a cursor kept from an answer, with the index of the post it is after.
type kept struct {
	cursor Cursor
	after  int
}

// machine runs a history against a driver and the model side by side.
type machine struct {
	d        Driver
	zone     *time.Location
	now      time.Time
	pageSize int

	// made counts the posts and comments made, in the order they were made.
	made int

	people   []*person
	posts    []*modelPost
	comments []*modelComment
	tokens   []string
	cursors  []kept

	// notes counts the bodies written, so that each is different.
	notes int
}

// join the next person.
func (m *machine) join(tc hegel.TestCase) {
	name := names[len(m.people)]
	id := m.d.Join(name)
	m.people = append(m.people, &person{
		index: len(m.people), id: id, name: name, active: true,
		joined: m.made, looked: m.made, basis: m.made,
		opened: map[int]int{}, sent: map[sentKey]int{},
	})
	tc.Note(fmt.Sprintf("%v joins", name))
}

// RuleJoin has another person join.
func (m *machine) RuleJoin(tc hegel.TestCase) {
	defer catch(tc)

	tc.Assume(len(m.people) < len(names))
	m.join(tc)
}

// RuleDeactivate makes someone inactive, while someone else stays active.
func (m *machine) RuleDeactivate(tc hegel.TestCase) {
	defer catch(tc)

	var active []*person
	for _, p := range m.people {
		if p.active {
			active = append(active, p)
		}
	}
	tc.Assume(len(active) > 1)
	p := hegel.Draw(tc, hegel.SampledFrom(active))
	tc.Note(fmt.Sprintf("%v is made inactive", p.name))
	m.d.Deactivate(p.id)
	p.active = false
}

// RuleAdvanceClock moves the clock on, by nothing at all up to a week.
func (m *machine) RuleAdvanceClock(tc hegel.TestCase) {
	defer catch(tc)

	d := hegel.Draw(tc, hegel.SampledFrom([]time.Duration{0, time.Millisecond, time.Second, time.Minute, time.Hour, 5 * time.Hour, 23 * time.Hour, 24 * time.Hour, 25 * time.Hour, 7 * 24 * time.Hour}))
	m.setClock(tc, m.now.Add(d))
}

// RuleToMidnight moves the clock to just before, at, or just after the next midnight in the group's time
// zone.
func (m *machine) RuleToMidnight(tc hegel.TestCase) {
	defer catch(tc)

	y, mo, d := m.now.In(m.zone).Date()
	midnight := time.Date(y, mo, d+1, 0, 0, 0, 0, m.zone)
	offset := hegel.Draw(tc, hegel.SampledFrom([]time.Duration{-time.Millisecond, 0, time.Millisecond}))
	m.setClock(tc, midnight.Add(offset))
}

func (m *machine) setClock(tc hegel.TestCase, t time.Time) {
	tc.Note(fmt.Sprintf("the clock is %v", t.In(m.zone).Format(time.RFC3339Nano)))
	m.now = t
	m.d.SetClock(t)
}

// actor draws someone in the group, or now and then a stranger, who is nil.
func (m *machine) actor(tc hegel.TestCase) (*person, model.UserID, string) {
	if hegel.Draw(tc, hegel.WeightedBooleans(0.1)) {
		return nil, stranger, "a stranger"
	}
	return m.who(m.people[hegel.Draw(tc, hegel.Integers(0, len(m.people)-1))])
}

// who the given person is, with their ID and a name for notes.
func (m *machine) who(p *person) (*person, model.UserID, string) {
	if !p.active {
		return p, p.id, p.name + " (inactive)"
	}
	return p, p.id, p.name
}

// RuleLook has someone look at the timeline.
func (m *machine) RuleLook(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Note(fmt.Sprintf("%v looks at the timeline", who))
	answer := m.d.Look(id)
	tc.Event("look: " + answer.Outcome.String())
	if m.private(tc, p, answer.Outcome, answer.Shown) {
		return
	}
	m.outcome(tc, "the look", Done, answer.Outcome)

	p.basis, p.looked = p.looked, m.made

	expected := m.page(p, m.sorted())
	expected.Members = 0
	for _, q := range m.people {
		if q.active {
			expected.Members++
		}
	}
	m.comparePage(tc, "the first page", answer.Page, expected)
	m.keep(answer.Page)
}

// RuleEarlier has someone get the page after a cursor kept from an answer before, perhaps one from before
// later posts, and perhaps one someone else got.
func (m *machine) RuleEarlier(tc hegel.TestCase) {
	defer catch(tc)

	tc.Assume(len(m.cursors) > 0)
	k := m.cursors[hegel.Draw(tc, hegel.Integers(0, len(m.cursors)-1))]
	p, id, who := m.actor(tc)
	tc.Note(fmt.Sprintf("%v gets the page after %v", who, m.posts[k.after].id))
	answer := m.d.Earlier(id, k.cursor)
	tc.Event("earlier: " + answer.Outcome.String())
	if m.private(tc, p, answer.Outcome, answer.Shown) {
		return
	}
	m.outcome(tc, "the page", Done, answer.Outcome)

	expected := m.page(p, m.after(k.after))
	expected.Members = -1
	answer.Page.Members = -1
	m.comparePage(tc, "the page after "+m.posts[k.after].id.String(), answer.Page, expected)
	m.keep(answer.Page)
}

// RuleMalformed has someone send what the page could not have sent: a token or a cursor that is not one.
func (m *machine) RuleMalformed(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	tc.Assume(p != nil && p.active)
	var o Outcome
	switch hegel.Draw(tc, hegel.Integers(0, 2)) {
	case 0:
		tc.Note(fmt.Sprintf("%v posts with a token that is not one", who))
		o = m.d.Post(id, "not a token", "Cake.").Outcome
	case 1:
		tc.Note(fmt.Sprintf("%v comments with a token that is not one", who))
		o = m.d.Comment(id, m.postID(tc), "q6urq6urq6urq6urq6urqx", "Cake.").Outcome
	default:
		tc.Note(fmt.Sprintf("%v asks for a page after a cursor that is not one", who))
		o = m.d.Earlier(id, Cursor{After: "AQAAAAAAAAAAAAAAAAAAAAAA"}).Outcome
	}
	m.outcome(tc, "what could not have been sent", BadRequest, o)
}

// body that can be sent, with what it is stored as, or whether it is refused.
type body struct {
	sent    string
	stored  string
	refused bool
}

// drawBody of the given maximum length: mostly one that can be posted, some of them as something else
// once cleaned, and now and then one that is refused. If something was stored with the token before,
// half the time it is that text, written the same or the same once cleaned.
func (m *machine) drawBody(tc hegel.TestCase, maxLength int, before string, sentBefore bool) body {
	if sentBefore && hegel.Draw(tc, hegel.Booleans()) {
		return hegel.Draw(tc, hegel.SampledFrom([]body{{sent: before, stored: before}, {sent: "\t" + before + "\r\n", stored: before}}))
	}

	m.notes++
	note := fmt.Sprint("Note ", m.notes)
	if !hegel.Draw(tc, hegel.WeightedBooleans(0.2)) {
		return hegel.Draw(tc, hegel.SampledFrom([]body{
			{sent: note, stored: note},
			{sent: "  " + note + "\r\nand more \n", stored: note + "\nand more"},
			{sent: "<b>" + note + "</b> & co", stored: "<b>" + note + "</b> & co"},
		}))
	}
	return hegel.Draw(tc, hegel.SampledFrom([]body{
		{sent: "", refused: true},
		{sent: " \n\t ", refused: true},
		{sent: string(rune(0x200b)) + string(rune(0x3164)), refused: true},
		{sent: strings.Repeat("x", maxLength+1), refused: true},
		{sent: note + " \xff", refused: true},
		{sent: note + " \x07", refused: true},
	}))
}

// token for a form: mostly a new one, and now and then one sent before by anyone.
func (m *machine) token(tc hegel.TestCase) string {
	if len(m.tokens) > 0 && hegel.Draw(tc, hegel.WeightedBooleans(0.15)) {
		return hegel.Draw(tc, hegel.SampledFrom(m.tokens))
	}
	t, err := model.NewFormToken(bytes.NewReader(hegel.Draw(tc, hegel.Binary(16, 16))))
	if err != nil {
		tc.Errorf("error making a token: %v", err)
	}
	if !slices.Contains(m.tokens, t.String()) {
		m.tokens = append(m.tokens, t.String())
	}
	return t.String()
}

// RulePost has someone post.
func (m *machine) RulePost(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	m.post(tc, p, id, who, m.token(tc))
}

// RuleSendAgain has someone send a form they sent before again, as after an answer that was lost, with
// the same text or with other text.
func (m *machine) RuleSendAgain(tc hegel.TestCase) {
	defer catch(tc)

	var senders []*person
	for _, p := range m.people {
		if len(p.sent) > 0 {
			senders = append(senders, p)
		}
	}
	tc.Assume(len(senders) > 0)
	p, id, who := m.who(hegel.Draw(tc, hegel.SampledFrom(senders)))

	var keys []sentKey
	for k := range p.sent {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b sentKey) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
	k := hegel.Draw(tc, hegel.SampledFrom(keys))
	if !k.comment {
		m.post(tc, p, id, who, k.token)
		return
	}

	// The form of a comment belongs to its post, so it is sent to that post again, unless crafted.
	target := m.comments[p.sent[k]].post
	postID := m.posts[target].id
	if hegel.Draw(tc, hegel.WeightedBooleans(0.2)) {
		target, postID = m.drawPost(tc)
	}
	m.comment(tc, p, id, who, target, postID, k.token)
}

// post as the given person, with the given token.
func (m *machine) post(tc hegel.TestCase, p *person, id model.UserID, who, token string) {
	before, sentBefore := -1, false
	if p != nil {
		before, sentBefore = m.sentBy(p, false, token)
	}
	var stored string
	if sentBefore {
		stored = m.posts[before].body
	}
	b := m.drawBody(tc, model.PostBodyMaxLength, stored, sentBefore)
	tc.Note(fmt.Sprintf("%v posts %q with token %v", who, abbreviate(b.sent), token))

	answer := m.d.Post(id, token, b.sent)
	tc.Event("post: " + answer.Outcome.String())
	if m.private(tc, p, answer.Outcome, answer.Shown) {
		return
	}

	switch {
	case sentBefore:
		expected := Changed
		if !b.refused && b.stored == stored {
			expected = Done
		}
		m.outcome(tc, "the post sent again", expected, answer.Outcome)
		m.comparePost(tc, answer.Post, m.posts[before])
		m.compareField(tc, answer.Field, expected, b.sent)

	case b.refused:
		m.outcome(tc, "the post", Refused, answer.Outcome)
		m.compareField(tc, answer.Field, Refused, b.sent)

	default:
		m.outcome(tc, "the post", Done, answer.Outcome)
		if answer.Post.ID == "" || slices.ContainsFunc(m.posts, func(q *modelPost) bool { return q.id == answer.Post.ID }) {
			tc.Errorf("the post made has the ID %q, which is empty or another post's", answer.Post.ID)
		}
		m.made++
		m.posts = append(m.posts, &modelPost{id: answer.Post.ID, author: p.index, body: b.stored, created: m.now, made: m.made})
		p.sent[sentKey{comment: false, token: token}] = len(m.posts) - 1
		m.comparePost(tc, answer.Post, m.posts[len(m.posts)-1])
		m.compareField(tc, answer.Field, Done, b.sent)
	}
}

// postID draws a post's ID, or the ID of a post that is not there, with its index, which is -1 then.
func (m *machine) postID(tc hegel.TestCase) model.PostID {
	_, id := m.drawPost(tc)
	return id
}

func (m *machine) drawPost(tc hegel.TestCase) (int, model.PostID) {
	if len(m.posts) == 0 || hegel.Draw(tc, hegel.WeightedBooleans(0.1)) {
		return -1, "p_00000000000000000000000000000000"
	}
	i := hegel.Draw(tc, hegel.Integers(0, len(m.posts)-1))
	return i, m.posts[i].id
}

// RuleComment has someone comment on a post, or on one that is not there.
func (m *machine) RuleComment(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	target, postID := m.drawPost(tc)
	m.comment(tc, p, id, who, target, postID, m.token(tc))
}

// comment as the given person on the post of the given index, which is -1 for one that is not there,
// with the given token.
func (m *machine) comment(tc hegel.TestCase, p *person, id model.UserID, who string, target int, postID model.PostID, token string) {
	before, sentBefore := -1, false
	if p != nil {
		before, sentBefore = m.sentBy(p, true, token)
	}
	var stored string
	if sentBefore {
		stored = m.comments[before].body
	}
	b := m.drawBody(tc, model.CommentBodyMaxLength, stored, sentBefore)
	tc.Note(fmt.Sprintf("%v comments %q on %v with token %v", who, abbreviate(b.sent), postID, token))

	answer := m.d.Comment(id, postID, token, b.sent)
	tc.Event("comment: " + answer.Outcome.String())
	if m.private(tc, p, answer.Outcome, answer.Shown) {
		return
	}

	switch {
	case sentBefore:
		expected := Changed
		if !b.refused && b.stored == stored {
			expected = Done
		}
		m.outcome(tc, "the comment sent again", expected, answer.Outcome)
		m.compareComment(tc, answer.Comment, m.comments[before])
		m.compareField(tc, answer.Field, expected, b.sent)
		if answer.Opened {
			m.open(tc, p, m.comments[before].post, answer.OpenedPost, answer.Comments)
		}

	case target < 0:
		m.outcome(tc, "the comment on a post that is not there", NotFound, answer.Outcome)

	case b.refused:
		m.outcome(tc, "the comment", Refused, answer.Outcome)
		m.compareField(tc, answer.Field, Refused, b.sent)

	default:
		m.outcome(tc, "the comment", Done, answer.Outcome)
		m.made++
		m.comments = append(m.comments, &modelComment{id: answer.Comment.ID, post: target, author: p.index, body: b.stored, created: m.now, made: m.made})
		c := m.comments[len(m.comments)-1]
		m.posts[target].comments = append(m.posts[target].comments, len(m.comments)-1)
		p.sent[sentKey{comment: true, token: token}] = len(m.comments) - 1
		m.compareField(tc, answer.Field, Done, b.sent)
		if answer.Opened {
			// A driver that cannot tell which comment it made learns it from the comments it opened.
			if c.id == "" {
				for _, shown := range answer.Comments {
					if !slices.ContainsFunc(m.comments, func(o *modelComment) bool { return o.id == shown.ID }) {
						c.id = shown.ID
					}
				}
			}
			m.open(tc, p, target, answer.OpenedPost, answer.Comments)
		}
		if c.id == "" || slices.ContainsFunc(m.comments[:len(m.comments)-1], func(o *modelComment) bool { return o.id == c.id }) {
			tc.Errorf("the comment made has the ID %q, which is empty or another comment's", c.id)
		}
		m.compareComment(tc, answer.Comment, c)
	}
}

// RuleOpen has someone open the comments of a post, or of one that is not there.
func (m *machine) RuleOpen(tc hegel.TestCase) {
	defer catch(tc)

	p, id, who := m.actor(tc)
	target, postID := m.drawPost(tc)
	tc.Note(fmt.Sprintf("%v opens the comments of %v", who, postID))
	answer := m.d.Open(id, postID)
	tc.Event("open: " + answer.Outcome.String())
	if m.private(tc, p, answer.Outcome, answer.Shown) {
		return
	}
	if target < 0 {
		m.outcome(tc, "opening the comments of a post that is not there", NotFound, answer.Outcome)
		return
	}
	m.outcome(tc, "opening the comments", Done, answer.Outcome)
	m.open(tc, p, target, postID, answer.Comments)
}

// InvariantTimelines checks the whole timeline of everyone active against the model, walked page by page
// without looking. Since the model changes a person's marks only through what they do themselves and
// what others post, this also checks that nobody's action changes what someone else is shown otherwise.
func (m *machine) InvariantTimelines(tc hegel.TestCase) {
	defer catch(tc)

	for _, p := range m.people {
		if !p.active {
			continue
		}

		pages, o := m.d.Walk(p.id)
		m.outcome(tc, "the walk of "+p.name, Done, o)

		for i, answer := range pages {
			m.outcome(tc, "a page of the walk of "+p.name, Done, answer.Outcome)
			m.keep(answer.Page)
			var n int
			for _, d := range answer.Page.Days {
				n += len(d.Posts)
			}
			last := i == len(pages)-1
			if n > m.pageSize || (!last && n != m.pageSize) || last != (answer.Page.Next == nil) {
				tc.Errorf("page %v of %v in the walk of %v has %v posts with a page size of %v, and a next page: %v", i+1, len(pages), p.name, n, m.pageSize, answer.Page.Next != nil)
			}
		}

		var walked []Day
		for _, answer := range pages {
			for _, d := range answer.Page.Days {
				if len(walked) > 0 && walked[len(walked)-1].Date == d.Date {
					walked[len(walked)-1].Posts = append(walked[len(walked)-1].Posts, d.Posts...)
					continue
				}
				walked = append(walked, d)
			}
		}

		m.compareDays(tc, "the timeline of "+p.name+" walked page by page", walked, m.days(m.sorted(), p))
	}
}

// sentBy the given person with the given token: the index of what it made, and whether it made anything.
func (m *machine) sentBy(p *person, comment bool, token string) (int, bool) {
	i, ok := p.sent[sentKey{comment: comment, token: token}]
	return i, ok
}

// private checks that someone who is not an active member gets nothing and learns nothing, and reports
// whether they were not.
func (m *machine) private(tc hegel.TestCase, p *person, o Outcome, shown string) bool {
	if p != nil && p.active {
		return false
	}
	m.outcome(tc, "the answer to someone not in the group", Private, o)
	for _, q := range m.people {
		if strings.Contains(shown, q.name) {
			tc.Errorf("the answer to someone not in the group shows the name %q: %q", q.name, shown)
		}
	}
	for _, q := range m.posts {
		if strings.Contains(shown, q.body) {
			tc.Errorf("the answer to someone not in the group shows the post %q: %q", q.body, shown)
		}
	}
	for _, c := range m.comments {
		if strings.Contains(shown, c.body) {
			tc.Errorf("the answer to someone not in the group shows the comment %q: %q", c.body, shown)
		}
	}
	return true
}

func (m *machine) outcome(tc hegel.TestCase, what string, expected, got Outcome) {
	if expected != got {
		tc.Errorf("%v was %v, not %v", what, got, expected)
	}
}

// open checks the comments the given person was shown when they opened the given post's comments, and
// records the opening.
func (m *machine) open(tc hegel.TestCase, p *person, target int, opened model.PostID, got []Comment) {
	if opened != m.posts[target].id {
		tc.Errorf("the comments of %v were opened, not of %v", opened, m.posts[target].id)
	}

	since := max(p.joined, p.opened[target])
	var expected []Comment
	for _, i := range m.posts[target].comments {
		c := m.comments[i]
		expected = append(expected, Comment{ID: c.id, Author: m.people[c.author].name, Body: c.body, Time: m.clock(c.created), New: c.author != p.index && c.made > since})
	}
	p.opened[target] = m.made

	if e, g := describeComments(expected), describeComments(got); e != g {
		tc.Errorf("the comments of %v opened by %v are\n%v\nnot\n%v", m.posts[target].id, p.name, g, e)
	}
}

func (m *machine) comparePost(tc hegel.TestCase, got Post, expected *modelPost) {
	e := Post{ID: expected.id, Author: m.people[expected.author].name, Body: expected.body, Time: m.clock(expected.created)}
	got.New, got.Comments, got.NewComments = false, 0, 0
	if got != e {
		tc.Errorf("the post shown is %+v, not %+v", got, e)
	}
}

func (m *machine) compareComment(tc hegel.TestCase, got Comment, expected *modelComment) {
	if got.ID == "" {
		return
	}
	e := Comment{ID: expected.id, Author: m.people[expected.author].name, Body: expected.body, Time: m.clock(expected.created)}
	got.New = false
	if got != e {
		tc.Errorf("the comment shown is %+v, not %+v", got, e)
	}
}

// compareField checks the text left in a form's field: nothing after something was made, and the text
// that was sent otherwise, as a browser reads it, if it can be text.
func (m *machine) compareField(tc hegel.TestCase, got *string, o Outcome, sent string) {
	if got == nil {
		return
	}
	expected := ""
	if o != Done {
		expected = strings.ReplaceAll(strings.ReplaceAll(sent, "\r\n", "\n"), "\r", "\n")
	}
	if !strings.Contains(expected, "\xff") && *got != expected {
		tc.Errorf("the field holds %q, not %q", abbreviate(*got), abbreviate(expected))
	}
}

// sorted indexes of the posts, as the timeline has them: newest first, and the one made later first when
// two have the same time.
func (m *machine) sorted() []int {
	var indexes []int
	for i := range m.posts {
		indexes = append(indexes, i)
	}
	slices.SortFunc(indexes, func(a, b int) int {
		if c := m.posts[b].created.Compare(m.posts[a].created); c != 0 {
			return c
		}
		return m.posts[b].made - m.posts[a].made
	})
	return indexes
}

// after the post of the given index, in the order of the timeline.
func (m *machine) after(i int) []int {
	sorted := m.sorted()
	return sorted[slices.Index(sorted, i)+1:]
}

// page of the timeline that starts with the first of the given posts, for the given reader.
func (m *machine) page(p *person, posts []int) Page {
	more := len(posts) > m.pageSize
	posts = posts[:min(len(posts), m.pageSize)]
	page := Page{Days: m.days(posts, p), Members: -1}
	if more {
		page.Next = &Cursor{}
	}
	return page
}

// keep the cursor of the given page, if it has one, with the post it is after.
func (m *machine) keep(page Page) {
	if page.Next == nil || len(page.Days) == 0 {
		return
	}
	if slices.ContainsFunc(m.cursors, func(k kept) bool { return k.cursor.After == page.Next.After && k.cursor.Days == page.Next.Days }) {
		return
	}
	posts := page.Days[len(page.Days)-1].Posts
	last := posts[len(posts)-1].ID
	for i, p := range m.posts {
		if p.id == last {
			m.cursors = append(m.cursors, kept{cursor: *page.Next, after: i})
		}
	}
}

// days that the given posts are grouped into, as the given reader sees them.
func (m *machine) days(posts []int, p *person) []Day {
	today := model.DateOf(m.now.In(m.zone))
	var days []Day
	for _, i := range posts {
		q := m.posts[i]
		date := model.DateOf(q.created.In(m.zone))
		if len(days) == 0 || days[len(days)-1].Date != date {
			days = append(days, Day{Date: date, Heading: true, Today: date == today, Yesterday: date == today.AddDays(-1)})
		}
		v := Post{ID: q.id, Author: m.people[q.author].name, Body: q.body, Time: m.clock(q.created), Comments: len(q.comments)}
		v.New = q.author != p.index && q.made > p.basis
		since := max(p.joined, p.opened[i])
		for _, c := range q.comments {
			if m.comments[c].author != p.index && m.comments[c].made > since {
				v.NewComments++
			}
		}
		days[len(days)-1].Posts = append(days[len(days)-1].Posts, v)
	}
	return days
}

func (m *machine) clock(t time.Time) string {
	return t.In(m.zone).Format(time.RFC3339)
}

func (m *machine) comparePage(tc hegel.TestCase, what string, got, expected Page) {
	if got.Members != expected.Members {
		tc.Errorf("%v says there are %v members, not %v", what, got.Members, expected.Members)
	}
	if (got.Next == nil) != (expected.Next == nil) {
		tc.Errorf("%v has a page after it: %v, but should have: %v", what, got.Next != nil, expected.Next != nil)
	}
	m.compareDays(tc, what, got.Days, expected.Days)
}

// compareDays, leaving out whether a day is today or yesterday where the answer drew no heading.
func (m *machine) compareDays(tc hegel.TestCase, what string, got, expected []Day) {
	expected = slices.Clone(expected)
	for i := range min(len(got), len(expected)) {
		if !got[i].Heading {
			expected[i].Heading = false
		}
	}
	if e, g := describeDays(expected), describeDays(got); e != g {
		tc.Errorf("%v is\n%v\nnot\n%v", what, g, e)
	}
}

func describeDays(days []Day) string {
	var b strings.Builder
	for _, d := range days {
		label := ""
		switch {
		case !d.Heading:
			label = " (heading not drawn)"
		case d.Today:
			label = " today"
		case d.Yesterday:
			label = " yesterday"
		}
		fmt.Fprintf(&b, "  %v%v\n", d.Date, label)
		for _, p := range d.Posts {
			fmt.Fprintf(&b, "    %v by %q at %v: %q", p.ID, p.Author, p.Time, abbreviate(p.Body))
			if p.New {
				b.WriteString(", new")
			}
			fmt.Fprintf(&b, ", %v comments, %v new\n", p.Comments, p.NewComments)
		}
	}
	if b.Len() == 0 {
		return "  (nothing)"
	}
	return b.String()
}

func describeComments(comments []Comment) string {
	var b strings.Builder
	for _, c := range comments {
		fmt.Fprintf(&b, "  %v by %q at %v: %q", c.ID, c.Author, c.Time, abbreviate(c.Body))
		if c.New {
			b.WriteString(", new")
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "  (none)"
	}
	return b.String()
}

// abbreviate a long text for a message.
func abbreviate(s string) string {
	if len(s) > 40 {
		return fmt.Sprintf("%v… (%v bytes)", s[:30], len(s))
	}
	return s
}
