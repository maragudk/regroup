package html_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	nethtml "golang.org/x/net/html"
	"hegel.dev/go/hegel"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
	"maragu.dev/is"

	"app/html"
	"app/model"
)

// parsed page of the given node.
func parsed(t *testing.T, n Node) *nethtml.Node {
	t.Helper()

	doc, err := nethtml.Parse(strings.NewReader(render(t, n)))
	is.NotError(t, err)
	return doc
}

// elements under the given node that match.
func elements(n *nethtml.Node, match func(*nethtml.Node) bool) []*nethtml.Node {
	var found []*nethtml.Node
	for d := range n.Descendants() {
		if d.Type == nethtml.ElementNode && match(d) {
			found = append(found, d)
		}
	}
	return found
}

// textOf everything under the given node.
func textOf(n *nethtml.Node) string {
	var b strings.Builder
	for d := range n.Descendants() {
		if d.Type == nethtml.TextNode {
			b.WriteString(d.Data)
		}
	}
	return b.String()
}

func tagged(name string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool { return n.Data == name }
}

func withAttr(key, value string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool {
		for _, a := range n.Attr {
			if a.Key == key && a.Val == value {
				return true
			}
		}
		return false
	}
}

func hasAttribute(key string) func(*nethtml.Node) bool {
	return func(n *nethtml.Node) bool {
		for _, a := range n.Attr {
			if a.Key == key {
				return true
			}
		}
		return false
	}
}

// labelFor the given field: the text of the label that names it.
func labelFor(doc, field *nethtml.Node) string {
	labels := elements(doc, func(n *nethtml.Node) bool { return n.Data == "label" && attribute(n, "for") == attribute(field, "id") })
	if len(labels) != 1 {
		return ""
	}
	return textOf(labels[0])
}

func TestPrivatePage(t *testing.T) {
	t.Run("says that the group is private, and has one field and one button to ask for a sign-in link, and nothing else", func(t *testing.T) {
		page := render(t, html.PrivatePage(html.PrivatePageProps{}))
		doc := parsed(t, html.PrivatePage(html.PrivatePageProps{}))

		is.True(t, strings.Contains(page, "<title>regroup</title>"), page)
		is.True(t, regexp.MustCompile(`<div[^>]*>regroup</div><h1[^>]*>This group is private\.</h1>`).MatchString(page), page)
		is.True(t, !strings.Contains(page, "<a "), page)

		forms := elements(doc, tagged("form"))
		is.Equal(t, 1, len(forms))
		is.Equal(t, "post", attribute(forms[0], "method"))
		is.Equal(t, "/sign-in", attribute(forms[0], "action"))
		is.Equal(t, "sign-in-request", attribute(forms[0], "data-form"))
		is.True(t, hasAttribute("novalidate")(forms[0]))

		fields := elements(doc, tagged("input"))
		is.Equal(t, 1, len(fields))
		is.Equal(t, "email", attribute(fields[0], "type"))
		is.Equal(t, "email", attribute(fields[0], "name"))
		is.Equal(t, "email", attribute(fields[0], "autocomplete"))
		is.Equal(t, "Your email address", attribute(fields[0], "placeholder"))
		is.Equal(t, "Email address", labelFor(doc, fields[0]))
		is.Equal(t, "", attribute(fields[0], "aria-invalid"))

		buttons := elements(doc, tagged("button"))
		is.Equal(t, 1, len(buttons))
		is.Equal(t, "Send me a link", textOf(buttons[0]))
		is.Equal(t, "submit", attribute(buttons[0], "type"))
		is.True(t, strings.Contains(attribute(buttons[0], "class"), "cursor-pointer"))
		is.True(t, strings.Contains(attribute(buttons[0], "class"), "bg-primary-500"))
		is.Equal(t, 0, len(elements(doc, hasAttribute("data-message"))))
	})

	t.Run("keeps what was typed after a refusal, says why under the field, and ties the message to the field", func(t *testing.T) {
		doc := parsed(t, html.PrivatePage(html.PrivatePageProps{Email: "signe", Refusal: html.Message{Key: "email-invalid", Text: "That is not an email address."}}))

		field := elements(doc, tagged("input"))[0]
		is.Equal(t, "signe", attribute(field, "value"))
		is.Equal(t, "true", attribute(field, "aria-invalid"))
		message := elements(doc, withAttr("id", attribute(field, "aria-describedby")))
		is.Equal(t, 1, len(message))
		is.Equal(t, "alert", attribute(message[0], "role"))
		is.Equal(t, "That is not an email address.", textOf(message[0]))
		marked := elements(doc, withAttr("data-message", "email-invalid"))
		is.Equal(t, 1, len(marked))
		is.Equal(t, "That is not an email address.", textOf(marked[0]))
		// The refusal is an icon and a sentence, not a colour.
		is.Equal(t, 1, len(elements(marked[0], tagged("svg"))))
	})

	t.Run("says after asking that a link is on its way if the address is someone's in the group, and how long it works, with no field", func(t *testing.T) {
		doc := parsed(t, html.PrivatePage(html.PrivatePageProps{Sent: true}))

		sent := elements(doc, withAttr("data-message", "sent"))
		is.Equal(t, 1, len(sent))
		is.Equal(t, "If that address belongs to someone in the group, a link to sign in is on its way to it. It works once, for the next 15 minutes.", textOf(sent[0]))
		is.Equal(t, "status", attribute(sent[0], "role"))
		is.Equal(t, 0, len(elements(doc, tagged("form"))))
		links := elements(doc, tagged("a"))
		is.Equal(t, 1, len(links))
		is.Equal(t, "/sign-in", attribute(links[0], "href"))
		is.Equal(t, "Ask for another link", textOf(links[0]))
	})

	t.Run("says a notice above the field", func(t *testing.T) {
		doc := parsed(t, html.PrivatePage(html.PrivatePageProps{Notice: html.Message{Key: "link-invalid", Text: "This link does not work. Ask for a new one."}}))

		notice := elements(doc, withAttr("data-message", "link-invalid"))
		is.Equal(t, 1, len(notice))
		is.Equal(t, "This link does not work. Ask for a new one.", textOf(notice[0]))
		is.Equal(t, 1, len(elements(doc, tagged("form"))))
	})

	t.Run("puts what it is given under the field", func(t *testing.T) {
		page := render(t, html.PrivatePage(html.PrivatePageProps{Extra: A(Href("/somewhere"), Text("Somewhere"))}))

		is.True(t, regexp.MustCompile(`</form><p[^>]*><a href="/somewhere">Somewhere</a></p>`).MatchString(page), page)
	})
}

func TestInvitationInvalidPage(t *testing.T) {
	t.Run("says the invitation does not work, without naming the group, with a link to ask for a sign-in link instead", func(t *testing.T) {
		page := render(t, html.InvitationInvalidPage(html.PageProps{}))
		doc := parsed(t, html.InvitationInvalidPage(html.PageProps{}))

		is.True(t, strings.Contains(page, "<title>regroup</title>"), page)
		is.Equal(t, "This group is private.", textOf(elements(doc, tagged("h1"))[0]))
		notice := elements(doc, withAttr("data-message", "invitation-invalid"))
		is.Equal(t, 1, len(notice))
		is.Equal(t, "This invitation does not work. Ask the person who invited you for a new one.", textOf(notice[0]))
		is.EqualSlice(t, []string{"/sign-in"}, hrefs(doc))
		is.Equal(t, 0, len(elements(doc, tagged("form"))))
	})
}

// hrefs of every link under the given node.
func hrefs(n *nethtml.Node) []string {
	var values []string
	for _, a := range elements(n, tagged("a")) {
		values = append(values, attribute(a, "href"))
	}
	return values
}

func TestSignInLinkPage(t *testing.T) {
	t.Run("names the group, and has one button that signs in with the token, and no field", func(t *testing.T) {
		page := render(t, html.SignInLinkPage(html.SignInLinkPageProps{GroupName: "The Loft", Token: "abc"}))
		doc := parsed(t, html.SignInLinkPage(html.SignInLinkPageProps{GroupName: "The Loft", Token: "abc"}))

		is.True(t, strings.Contains(page, "<title>Sign in to The Loft</title>"), page)
		is.Equal(t, "The Loft", textOf(elements(doc, tagged("h1"))[0]))
		forms := elements(doc, tagged("form"))
		is.Equal(t, 1, len(forms))
		is.Equal(t, "/sign-in/link", attribute(forms[0], "action"))
		is.Equal(t, "post", attribute(forms[0], "method"))
		is.Equal(t, "sign-in", attribute(forms[0], "data-form"))
		inputs := elements(doc, tagged("input"))
		is.Equal(t, 1, len(inputs))
		is.Equal(t, "hidden", attribute(inputs[0], "type"))
		is.Equal(t, "token", attribute(inputs[0], "name"))
		is.Equal(t, "abc", attribute(inputs[0], "value"))
		is.EqualSlice(t, []string{"Sign in"}, texts(elements(doc, tagged("button"))))
		is.Equal(t, 0, len(elements(doc, tagged("a"))))
	})

	t.Run("has the first admin choose the name the group will see, and keeps a refused one with why", func(t *testing.T) {
		doc := parsed(t, html.SignInLinkPage(html.SignInLinkPageProps{GroupName: "The Loft", Token: "abc", Joining: true}))

		form := elements(doc, tagged("form"))[0]
		is.Equal(t, "join-first", attribute(form, "data-form"))
		field := elements(form, withAttr("name", "name"))[0]
		is.Equal(t, "text", attribute(field, "type"))
		is.Equal(t, "name", attribute(field, "autocomplete"))
		is.Equal(t, "Your name", labelFor(doc, field))
		is.EqualSlice(t, []string{"Join"}, texts(elements(doc, tagged("button"))))
		is.True(t, strings.Contains(textOf(doc), "Choose the name the group will see."))

		doc = parsed(t, html.SignInLinkPage(html.SignInLinkPageProps{GroupName: "The Loft", Token: "abc", Joining: true, Name: " ", Refusal: html.Message{Key: "name-empty", Text: "Write the name the group will see first."}}))
		field = elements(doc, withAttr("name", "name"))[0]
		is.Equal(t, " ", attribute(field, "value"))
		is.Equal(t, "true", attribute(field, "aria-invalid"))
		is.Equal(t, "Write the name the group will see first.", textOf(elements(doc, withAttr("id", attribute(field, "aria-describedby")))[0]))
		is.Equal(t, 1, len(elements(doc, withAttr("data-message", "name-empty"))))
	})
}

func texts(nodes []*nethtml.Node) []string {
	var values []string
	for _, n := range nodes {
		values = append(values, textOf(n))
	}
	return values
}

func TestInvitationPage(t *testing.T) {
	t.Run("names the group and who invited, and has the field for a name and one button that joins with the token", func(t *testing.T) {
		page := render(t, html.InvitationPage(html.InvitationPageProps{GroupName: "The Loft", Token: "abc", InvitedByName: "Signe"}))
		doc := parsed(t, html.InvitationPage(html.InvitationPageProps{GroupName: "The Loft", Token: "abc", InvitedByName: "Signe"}))

		is.True(t, strings.Contains(page, "<title>Join The Loft</title>"), page)
		is.Equal(t, "The Loft", textOf(elements(doc, tagged("h1"))[0]))
		is.EqualSlice(t, []string{"Signe"}, texts(elements(doc, hasAttribute("data-inviter"))))
		is.True(t, strings.Contains(textOf(doc), "Signe invited you to join. Choose the name the group will see."))
		forms := elements(doc, tagged("form"))
		is.Equal(t, 1, len(forms))
		is.Equal(t, "/invitation", attribute(forms[0], "action"))
		is.Equal(t, "join", attribute(forms[0], "data-form"))
		is.Equal(t, "abc", attribute(elements(doc, withAttr("name", "token"))[0], "value"))
		field := elements(doc, withAttr("name", "name"))[0]
		is.Equal(t, "name", attribute(field, "autocomplete"))
		is.Equal(t, "Your name", labelFor(doc, field))
		is.EqualSlice(t, []string{"Join"}, texts(elements(doc, tagged("button"))))
	})
}

// members of a made-up group: an admin, a moderator, and a member, in the order they joined.
var members = []model.Member{
	{ID: "u_markus", Name: "Markus", Role: model.RoleAdmin, Email: "markus@example.com"},
	{ID: "u_signe", Name: "Signe", Role: model.RoleModerator, Email: "signe@example.com"},
	{ID: "u_thomas", Name: "Thomas", Role: model.RoleMember, Email: "thomas@example.com"},
}

// withoutEmails of the given members, as someone who may not see them gets them.
func withoutEmails(ms []model.Member) []model.Member {
	var out []model.Member
	for _, m := range ms {
		m.Email = ""
		out = append(out, m)
	}
	return out
}

var waiting = []model.Invitation{
	{ID: "i_1", Email: "amira@example.com", InvitedBy: "u_signe", InvitedByName: "Signe", Expires: model.Time{T: time.Date(2031, 3, 20, 10, 0, 0, 0, time.UTC)}},
	{ID: "i_2", Email: "nadia@example.com", InvitedBy: "u_markus", InvitedByName: "Markus", Expires: model.Time{T: time.Date(2032, 1, 2, 9, 30, 0, 0, time.UTC)}},
}

func actor(id model.UserID, name string, role model.Role) model.User {
	return model.User{ID: id, Name: name, Role: role, Active: true}
}

func TestMembersPage(t *testing.T) {
	t.Run("lists the members with their names and roles in the order given, under the sign, with stable markers", func(t *testing.T) {
		page := render(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_thomas", "Thomas", model.RoleMember), Members: withoutEmails(members), Now: now}))
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_thomas", "Thomas", model.RoleMember), Members: withoutEmails(members), Now: now}))

		is.True(t, strings.Contains(page, "<title>Members of The Loft</title>"), page)
		is.True(t, regexp.MustCompile(`<a href="/members" aria-current="page" class="[^"]*">3 members</a>`).MatchString(page), page)
		is.True(t, regexp.MustCompile(`<h1[^>]*><a href="/" class="[^"]*">The Loft</a></h1>`).MatchString(page), page)
		is.EqualSlice(t, []string{"h1", "h2"}, headings(doc))

		rows := elements(doc, hasAttribute("data-member"))
		is.EqualSlice(t, []string{"u_markus", "u_signe", "u_thomas"}, attributes(rows, "data-member"))
		is.EqualSlice(t, []string{"admin", "moderator", "member"}, attributes(rows, "data-role"))
		is.EqualSlice(t, []string{"Markus", "Signe", "Thomas"}, texts(elements(doc, hasAttribute("data-name"))))
		is.EqualSlice(t, []string{"MarkusAdmin", "SigneModerator", "ThomasMember, you"}, texts(rows))
	})

	t.Run("shows a member no address and no control but signing out", func(t *testing.T) {
		page := render(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_thomas", "Thomas", model.RoleMember), Members: withoutEmails(members), Invitations: waiting, Now: now, Invite: html.InviteFormProps{Token: "t"}}))
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_thomas", "Thomas", model.RoleMember), Members: withoutEmails(members), Invitations: waiting, Now: now, Invite: html.InviteFormProps{Token: "t"}}))

		is.True(t, !strings.Contains(page, "@"), page)
		is.EqualSlice(t, []string{"/logout"}, attributes(elements(doc, tagged("form")), "action"))
		is.EqualSlice(t, []string{"Sign out"}, texts(elements(doc, tagged("button"))))
		is.Equal(t, 0, len(elements(doc, tagged("details"))))
		is.Equal(t, 0, len(elements(doc, tagged("input"))))
		is.Equal(t, 0, len(elements(doc, hasAttribute("data-invitation"))))
		is.True(t, !strings.Contains(page, "Invitations"), page)
	})

	t.Run("shows a moderator the field to invite and the invitations that are waiting, with who invited, when each runs out, and a button to withdraw it, and no addresses of members", func(t *testing.T) {
		props := html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_signe", "Signe", model.RoleModerator), Members: withoutEmails(members), Invitations: waiting, Now: now, Invite: html.InviteFormProps{Token: "t"}}
		doc := parsed(t, html.MembersPage(props))

		is.EqualSlice(t, []string{"h1", "h2", "h2"}, headings(doc))
		is.EqualSlice(t, []string{"/logout", "/invitations", "/invitations/withdraw", "/invitations/withdraw"}, attributes(elements(doc, tagged("form")), "action"))
		is.EqualSlice(t, []string{"sign-out", "invite", "withdraw-invitation", "withdraw-invitation"}, attributes(elements(doc, tagged("form")), "data-form"))
		is.Equal(t, 0, len(elements(doc, tagged("details"))))
		is.Equal(t, 0, len(elements(doc, hasAttribute("data-email")))-len(waiting))

		invite := elements(doc, withAttr("id", "invite"))[0]
		is.Equal(t, "t", attribute(elements(invite, withAttr("name", "token"))[0], "value"))
		field := elements(invite, withAttr("name", "email"))[0]
		is.Equal(t, "email", attribute(field, "type"))
		is.Equal(t, "off", attribute(field, "autocomplete"))
		is.Equal(t, "Email address to invite", labelFor(doc, field))
		is.EqualSlice(t, []string{"Invite"}, texts(elements(invite, tagged("button"))))

		rows := elements(doc, hasAttribute("data-invitation"))
		is.EqualSlice(t, []string{"i_1", "i_2"}, attributes(rows, "data-invitation"))
		is.Equal(t, "amira@example.comInvited by Signe. Runs out on Thursday 20 March at 10:00.Withdraw the invitation to amira@example.com", textOf(rows[0]))
		is.True(t, strings.Contains(textOf(rows[1]), "Runs out on Friday 2 January 2032 at 09:30."), textOf(rows[1]))
		is.EqualSlice(t, []string{"Signe", "Markus"}, texts(elements(doc, hasAttribute("data-inviter"))))
		is.EqualSlice(t, []string{"i_1"}, attributes(elements(rows[0], withAttr("name", "invitation")), "value"))
		for _, b := range elements(doc, tagged("button")) {
			is.True(t, strings.Contains(attribute(b, "class"), "cursor-pointer"), attribute(b, "class"))
		}
		// One orange button on the page.
		is.Equal(t, 1, len(elements(doc, func(n *nethtml.Node) bool {
			return strings.Contains(attribute(n, "class"), "bg-primary-500") && n.Data == "button"
		})))
	})

	t.Run("says there are no invitations waiting when there are none", func(t *testing.T) {
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_signe", "Signe", model.RoleModerator), Members: withoutEmails(members), Now: now}))

		is.True(t, strings.Contains(textOf(doc), "No invitations are waiting."))
	})

	t.Run("shows an admin the addresses, and a control to change the role of everyone but themselves, with a button for each other role", func(t *testing.T) {
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_markus", "Markus", model.RoleAdmin), Members: members, Invitations: waiting, Now: now}))

		rows := elements(doc, hasAttribute("data-member"))
		is.EqualSlice(t, []string{"markus@example.com", "signe@example.com", "thomas@example.com"}, texts(elements(elements(doc, withAttr("id", "members"))[0], hasAttribute("data-email"))))
		is.Equal(t, 0, len(elements(rows[0], tagged("details"))))
		is.Equal(t, 0, len(elements(rows[0], tagged("form"))))

		for i, expected := range [][]string{{"admin", "member"}, {"admin", "moderator"}} {
			row := rows[i+1]
			forms := elements(row, tagged("form"))
			is.Equal(t, 1, len(forms))
			is.Equal(t, "/members/role", attribute(forms[0], "action"))
			is.Equal(t, "change-role", attribute(forms[0], "data-form"))
			is.Equal(t, attribute(row, "data-member"), attribute(elements(forms[0], withAttr("name", "member"))[0], "value"))
			is.EqualSlice(t, expected, attributes(elements(forms[0], tagged("button")), "value"))
			is.EqualSlice(t, []string{"role", "role"}, attributes(elements(forms[0], tagged("button")), "name"))
			summary := elements(row, tagged("summary"))[0]
			is.Equal(t, "Change role of "+textOf(elements(row, hasAttribute("data-name"))[0]), textOf(summary))
		}
		is.EqualSlice(t, []string{"Make admin", "Make member", "Make admin", "Make moderator"}, texts(elements(elements(doc, withAttr("id", "members"))[0], tagged("button"))))
	})

	t.Run("keeps what was typed to invite after a refusal, says why under the field, and says when an invitation is on its way", func(t *testing.T) {
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_signe", "Signe", model.RoleModerator), Members: withoutEmails(members), Now: now,
			Invite: html.InviteFormProps{Email: "signe@example.com", Refusal: html.Message{Key: "already-member", Text: "In already."}}}))

		field := elements(doc, withAttr("id", "invite-email"))[0]
		is.Equal(t, "signe@example.com", attribute(field, "value"))
		is.Equal(t, "true", attribute(field, "aria-invalid"))
		is.Equal(t, "In already.", textOf(elements(doc, withAttr("id", attribute(field, "aria-describedby")))[0]))
		is.Equal(t, 1, len(elements(doc, withAttr("data-message", "already-member"))))

		doc = parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_signe", "Signe", model.RoleModerator), Members: withoutEmails(members), Now: now,
			Invite: html.InviteFormProps{Notice: html.Message{Key: "invited", Text: "On its way."}}}))
		notice := elements(doc, withAttr("data-message", "invited"))
		is.Equal(t, 1, len(notice))
		is.Equal(t, "status", attribute(notice[0], "role"))
		is.Equal(t, "", attribute(elements(doc, withAttr("id", "invite-email"))[0], "aria-invalid"))
	})

	t.Run("says a notice at the top", func(t *testing.T) {
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: actor("u_markus", "Markus", model.RoleAdmin), Members: members, Now: now, Notice: html.Message{Key: "own-role", Text: "Nobody can change their own role."}}))

		notice := elements(doc, withAttr("data-message", "own-role"))
		is.Equal(t, 1, len(notice))
		is.Equal(t, "alert", attribute(notice[0], "role"))
		is.Equal(t, "Nobody can change their own role.", textOf(notice[0]))
	})

	t.Run("shows someone no longer active no control at all", func(t *testing.T) {
		inactive := actor("u_markus", "Markus", model.RoleAdmin)
		inactive.Active = false
		doc := parsed(t, html.MembersPage(html.MembersPageProps{GroupName: "The Loft", Actor: inactive, Members: members, Invitations: waiting, Now: now}))

		is.EqualSlice(t, []string{"/logout"}, attributes(elements(doc, tagged("form")), "action"))
	})
}

func headings(doc *nethtml.Node) []string {
	var tags []string
	for _, n := range elements(doc, func(n *nethtml.Node) bool { return regexp.MustCompile(`^h[1-6]$`).MatchString(n.Data) }) {
		tags = append(tags, n.Data)
	}
	return tags
}

func attributes(nodes []*nethtml.Node, key string) []string {
	var values []string
	for _, n := range nodes {
		values = append(values, attribute(n, key))
	}
	return values
}

func TestPeoplePages_text(t *testing.T) {
	// pages with the given name for every person and the group, and the given address for every member
	// and invitation, and typed into every field, as the pages for people show them. It returns how many
	// elements and attributes they have, and the parsed pages.
	pages := func(t *testing.T, name string, address model.EmailAddress) (int, int, *nethtml.Node) {
		ms := []model.Member{
			{ID: "u_1", Name: name, Role: model.RoleAdmin, Email: address},
			{ID: "u_2", Name: name, Role: model.RoleMember, Email: address},
		}
		invs := []model.Invitation{{ID: "i_1", Email: address, InvitedBy: "u_1", InvitedByName: name, Expires: model.Time{T: now}}}
		doc := parsed(t, Group{
			html.MembersPage(html.MembersPageProps{GroupName: name, Actor: actor("u_1", name, model.RoleAdmin), Members: ms, Invitations: invs, Now: now,
				Invite: html.InviteFormProps{Email: string(address), Refusal: html.Message{Key: "k", Text: "Refused."}}}),
			html.InvitationPage(html.InvitationPageProps{GroupName: name, InvitedByName: name, Name: name, Refusal: html.Message{Key: "k", Text: "Refused."}}),
			html.SignInLinkPage(html.SignInLinkPageProps{GroupName: name, Joining: true, Name: name}),
			html.PrivatePage(html.PrivatePageProps{Email: string(address)}),
		})

		var elems, attrs int
		for n := range doc.Descendants() {
			if n.Type == nethtml.ElementNode {
				elems++
				attrs += len(n.Attr)
			}
		}
		return elems, attrs, doc
	}

	// check that the given name and address are in exactly the places they go, as exactly that text, and
	// that the pages have as many elements and attributes as with a plain name and address.
	plainElements, plainAttributes, _ := pages(t, "Signe", "signe@example.com")
	check := func(t testing.TB, name string, address model.EmailAddress, fail func(format string, args ...any)) {
		elems, attrs, doc := pages(t.(*testing.T), name, address)
		if elems != plainElements || attrs != plainAttributes {
			fail("the name %q and the address %q changed the pages from %v elements and %v attributes to %v and %v", name, address, plainElements, plainAttributes, elems, attrs)
		}

		var names, emails, values int
		for n := range doc.Descendants() {
			if n.Type != nethtml.ElementNode {
				continue
			}

			if n.Data == "input" && (attribute(n, "name") == "email" || attribute(n, "name") == "name") {
				values++
				expected := string(address)
				if attribute(n, "name") == "name" {
					expected = name
				}
				if got := attribute(n, "value"); got != expected {
					fail("expected the field to hold %q, got %q", expected, got)
				}
				continue
			}

			var expected string
			switch {
			case hasAttribute("data-name")(n) || hasAttribute("data-inviter")(n) || hasAttribute("data-person")(n) || n.Data == "a" && n.Parent.Data == "h1" || n.Data == "h1" && n.FirstChild != nil && n.FirstChild.Type == nethtml.TextNode && n.FirstChild.Data != "This group is private.":
				names++
				expected = name
			case hasAttribute("data-email")(n):
				emails++
				expected = string(address)
			default:
				continue
			}

			var got string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != nethtml.TextNode {
					fail("%q or %q put something other than text into %v", name, address, n.Data)
				}
				got += c.Data
			}
			if got != expected {
				fail("expected %q in %v, got %q", expected, n.Data, got)
			}
		}
		// The name: two members, the sign's person and group, and an inviter on the list, the group and
		// the inviter on the invitation page, and the group on the sign-in page. The address: two members and
		// an invitation. The fields: the invite field, two name fields, and the sign-in field.
		if names != 8 || emails != 3 || values != 4 {
			fail("expected the name in 8 places, the address in 3, and 4 fields, but found %v, %v, and %v", names, emails, values)
		}
	}

	t.Run("shows any name that can be chosen, and any address that can be given, as exactly that text, wherever it comes out, and never as an element or an attribute", func(t *testing.T) {
		hegel.Test(t, func(ht *hegel.T) {
			name, err := model.CleanName(hegel.Draw(ht, markup()))
			ht.Assume(err == nil)
			address, err := model.ParseEmailAddress(hegel.Draw(ht, addresses()))
			ht.Assume(err == nil)

			check(t, name, address, ht.Fatalf)
		})
	})

	for _, test := range []struct {
		name    string
		address model.EmailAddress
	}{
		{"<script>alert(1)</script>", "a&b@example.com"},
		{"<b>Signe</b>", "'onmouseover=alert'@example.com"},
		{"Signe & Thomas", "a`b=c/d@example.com"},
		{"\"><img src=x onerror=alert(1)>", "x@example.com"},
		{"</h1><h1>The Loft", "&amp@example.com"},
		{"&lt;b&gt;", "{a}|~@example.com"},
	} {
		t.Run("shows the name "+test.name+" and the address "+string(test.address)+" as exactly that text", func(t *testing.T) {
			_, err := model.CleanName(test.name)
			is.NotError(t, err)
			_, err = model.ParseEmailAddress(string(test.address))
			is.NotError(t, err)

			check(t, test.name, test.address, t.Fatalf)
		})
	}
}

// addresses that can be given, made of the characters an address may have that mean something in HTML,
// and others.
func addresses() hegel.Generator[string] {
	return hegel.OneOf(
		hegel.Emails(),
		hegel.Composite(func(tc hegel.TestCase) string {
			local := hegel.Draw(tc, hegel.Text().Alphabet("a.!#$%&'*+/=?^_`{|}~-").MinSize(1).MaxSize(20))
			return local + "@" + hegel.Draw(tc, hegel.SampledFrom([]string{"example.com", "a-b.example", "x.y.z"}))
		}),
	)
}
