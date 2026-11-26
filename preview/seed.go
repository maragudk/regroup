// Package preview provides a made-up office group to fill an empty database with, a way to sign in as any
// of its people without an email, and an outbox that keeps the emails the app would have sent, for a page
// to show with links that work. Anyone who can reach the routes can sign in as anyone, and read every
// email.
package preview

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"maragu.dev/errors"

	"app/model"
	"app/sqlite"
)

// People of the made-up group, in the order they joined. Each has the address of their name, lowercased,
// at example.com.
var people = []string{
	"Markus", "Signe", "Thomas", "Amira", "Jonas", "Karen", "Henrik",
	"Mette", "Lars", "Priya", "Oskar", "Freja", "Ibrahim", "Sofie",
}

// roles of the people of the made-up group who are not members: Markus is the admin, and two others are
// moderators.
var roles = map[string]model.Role{
	"Markus": model.RoleAdmin,
	"Signe":  model.RoleModerator,
	"Thomas": model.RoleModerator,
}

// seedPost of the made-up group, posted daysAgo days before today at the given time of day, in the
// group's time zone.
type seedPost struct {
	daysAgo      int
	hour, minute int
	author       string
	body         string
	comments     []seedComment
}

// seedComment on a [seedPost], written the given duration after the post.
type seedComment struct {
	after  time.Duration
	author string
	body   string
}

var posts = []seedPost{
	{daysAgo: 4, hour: 10, minute: 5, author: "Thomas",
		body: "Release 2.4 is out. Thanks to everyone who tested it over the weekend.\n\nAnd sorry about the fire alarm on Friday. Unrelated, I promise.",
		comments: []seedComment{
			{after: 40 * time.Minute, author: "Amira", body: "Congratulations! The new export is so much faster."},
			{after: 2 * time.Hour, author: "Jonas", body: "Was it really unrelated?"},
			{after: 3 * time.Hour, author: "Thomas", body: "Mostly."},
		}},
	{daysAgo: 3, hour: 13, minute: 20, author: "Karen",
		body: "Does anyone know a good accountant? Mine is retiring to Portugal, and the annual report is due in March.",
		comments: []seedComment{
			{after: 25 * time.Minute, author: "Signe", body: "I use Lene Holm on Nørrebrogade. She is patient with receipts in shoeboxes."},
			{after: time.Hour, author: "Henrik", body: "Seconded, Lene is good."},
			{after: 5 * time.Hour, author: "Karen", body: "Thank you both. I have written to her."},
		}},
	{daysAgo: 2, hour: 9, minute: 45, author: "Jonas",
		body: "Greetings from Lisbon. It is 19 degrees, and nobody here has heard the word sleet.\nBack on Monday.",
		comments: []seedComment{
			{after: 30 * time.Minute, author: "Freja", body: "Bring pastéis de nata."},
			{after: 2 * time.Hour, author: "Markus", body: "Enjoy. Your plant is being watered."},
		}},
	{daysAgo: 1, hour: 9, minute: 12, author: "Amira",
		body: "The coffee machine is sulking again. It blinks orange, makes a noise like a tired cow, and produces nothing.\n\nI have put a note on it.",
		comments: []seedComment{
			{after: 10 * time.Minute, author: "Oskar", body: "Have you tried descaling it?"},
			{after: 15 * time.Minute, author: "Amira", body: "Oskar, it was descaled on Tuesday."},
			{after: 50 * time.Minute, author: "Priya", body: "Tea still works, for the record."},
			// Written the next morning, after everyone's last look.
			{after: 23 * time.Hour, author: "Lars", body: "I have called the service company. They come Thursday between 8 and 12."},
		}},
	{daysAgo: 1, hour: 14, minute: 30, author: "Markus",
		body: "Reminder: the meeting room is booked all Thursday afternoon for the cooperative's board. The phone booth is free."},
	{daysAgo: 1, hour: 16, minute: 5, author: "Sofie",
		body: "Found: a blue umbrella in the stairwell. It is on the shelf by the post.",
		comments: []seedComment{
			{after: 20 * time.Minute, author: "Mette", body: "Mine! Thank you."},
		}},
	{daysAgo: 0, hour: 8, minute: 40, author: "Signe",
		body: "Cake in the kitchen. It is carrot, it is my birthday, and there is more than enough.",
		comments: []seedComment{
			{after: 5 * time.Minute, author: "Thomas", body: "Happy birthday!"},
			{after: 30 * time.Minute, author: "Ibrahim", body: "Happy birthday, Signe. The cake is excellent."},
			{after: 45 * time.Minute, author: "Mette", body: "Are we saving a piece for Jonas?"},
			{after: 50 * time.Minute, author: "Signe", body: "No."},
		}},
	{daysAgo: 0, hour: 10, minute: 15, author: "Ibrahim",
		body: "The corner window by the printer will not close properly. I have taped it for now, so please do not open it.",
		comments: []seedComment{
			{after: 20 * time.Minute, author: "Henrik", body: "I will bring a screwdriver tomorrow."},
		}},
}

// Seed an empty database with a made-up office group: its people, and a few days of posts
// and comments, timed relative to now in the given time zone, so that the group always looks current.
// Everyone last looked at the group the evening before, so what came after that is new to them.
//
// A database that already has people is left as it is, and Seed returns false.
func Seed(ctx context.Context, db *sqlite.Database, now time.Time, loc *time.Location) (bool, error) {
	var count int
	if err := db.H.Get(ctx, &count, `select count(*) from users`); err != nil {
		return false, errors.Wrap(err, "error counting users")
	}
	if count > 0 {
		return false, nil
	}

	ids := map[string]model.UserID{}
	for _, name := range people {
		var accountID model.AccountID
		if err := db.H.Get(ctx, &accountID, `insert into accounts (name) values (?) returning id`, name); err != nil {
			return false, errors.Wrap(err, "error creating account")
		}

		role := cmp.Or(roles[name], model.RoleMember)
		var invitedBy *model.UserID
		if name != people[0] {
			invitedBy = new(ids[people[0]])
		}

		var id model.UserID
		query := `insert into users (account_id, name, email, confirmed, active, role, invited_by) values (?, ?, ?, 1, 1, ?, ?) returning id`
		if err := db.H.Get(ctx, &id, query, accountID, name, strings.ToLower(name)+"@example.com", role, invitedBy); err != nil {
			return false, errors.Wrap(err, "error creating user")
		}
		ids[name] = id
	}

	type item struct {
		at      time.Time
		post    int
		comment int // -1 for the post itself
	}

	now = now.In(loc)
	today := model.DateOf(now)
	var items []item
	for i, p := range posts {
		date := today.AddDays(-p.daysAgo)
		at := time.Date(date.Year, date.Month, date.Day, p.hour, p.minute, 0, 0, loc)
		items = append(items, item{at: at, post: i, comment: -1})
		for j, c := range p.comments {
			items = append(items, item{at: at.Add(c.after), post: i, comment: j})
		}
	}

	slices.SortStableFunc(items, func(a, b item) int { return a.at.Compare(b.at) })

	// Nothing is in the future: what would be is moved to just before now, keeping its order.
	for i := range items {
		if latest := now.Add(-time.Duration(len(items)-i) * time.Minute); items[i].at.After(latest) {
			items[i].at = latest
		}
	}

	yesterdayEvening := today.AddDays(-1)
	lookedAt := time.Date(yesterdayEvening.Year, yesterdayEvening.Month, yesterdayEvening.Day, 18, 0, 0, 0, loc)
	looked := false
	created := map[int]model.Post{}

	for _, it := range items {
		if !looked && it.at.After(lookedAt) {
			if err := lookAtEverything(ctx, db, ids, created); err != nil {
				return false, err
			}
			looked = true
		}

		p := posts[it.post]
		if it.comment < 0 {
			post, err := db.CreatePost(ctx, model.Post{Created: model.Time{T: it.at}, AuthorID: ids[p.author], Body: p.body}, "")
			if err != nil {
				return false, errors.Wrap(err, "error creating post")
			}
			created[it.post] = post
			continue
		}

		c := p.comments[it.comment]
		if _, err := db.CreateComment(ctx, model.Comment{Created: model.Time{T: it.at}, PostID: created[it.post].ID, AuthorID: ids[c.author], Body: c.body}, ""); err != nil {
			return false, errors.Wrap(err, "error creating comment")
		}
	}

	return true, nil
}

// lookAtEverything as each person, so that their read state is as if they had looked at the timeline and
// opened every post's comments.
func lookAtEverything(ctx context.Context, db *sqlite.Database, ids map[string]model.UserID, created map[int]model.Post) error {
	for _, name := range people {
		if _, _, err := db.GetTimeline(ctx, ids[name], nil, 1); err != nil {
			return errors.Wrap(err, "error looking at timeline")
		}
		for _, p := range created {
			if _, err := db.GetComments(ctx, ids[name], p.ID); err != nil {
				return errors.Wrap(err, "error opening comments")
			}
		}
	}
	return nil
}
