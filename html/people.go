package html

import (
	"fmt"
	"time"

	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"

	"app/model"
)

// Message on a page, such as why what was sent was refused. Key names it in the page's markers. A
// message with no text is no message.
type Message struct {
	Key  string
	Text string
}

// PrivatePageProps is what the private page shows.
type PrivatePageProps struct {
	PageProps

	// Email that was typed into the field to ask for a sign-in link, which is kept after a refusal.
	Email string

	// Refusal of what was typed into the field, if it was refused.
	Refusal Message

	// Notice above the field, such as that a link does not work.
	Notice Message

	// Sent, if true, makes this the answer to asking for a sign-in link, which says that one is on its
	// way if the address belongs to someone in the group, in place of the field.
	Sent bool

	// Extra is put under the field, if not nil.
	Extra Node
}

// PrivatePage says that the group is private, and has a field to ask for a link to sign in with. It names
// neither the group nor anyone in it, and says nothing about whether an address is in the group.
func PrivatePage(props PrivatePageProps) Node {
	return privateDocument(props.PageProps,
		notice(props.Notice),
		If(props.Sent, Group{
			P(Data("message", "sent"), Role("status"), Text("If that address belongs to someone in the group, a link to sign in is on its way to it. It works once, for the next "+model.LifetimeWords(model.SignInLinkLifetime)+".")),
			P(Class("mt-4"), a(Href("/sign-in"), Text("Ask for another link"))),
		}),
		If(!props.Sent, signInRequestForm(props.Email, props.Refusal)),
		Iff(props.Extra != nil, func() Node {
			return P(Class("mt-8 [&_a]:underline [&_a]:decoration-stone-400 [&_a]:decoration-1 [&_a]:underline-offset-4 [&_a]:hover:decoration-stone-900"), props.Extra)
		}),
	)
}

// InvitationInvalidPage says that an invitation does not work, without saying why, and without naming the
// group or anyone in it.
func InvitationInvalidPage(props PageProps) Node {
	return privateDocument(props,
		notice(Message{Key: "invitation-invalid", Text: "This invitation does not work. Ask the person who invited you for a new one."}),
		P(Text("If you have joined already, "), a(Href("/sign-in"), Text("ask for a link to sign in")), Text(".")),
	)
}

// privateDocument with the sign that says the group is private, and the given body under it. Its title is
// "regroup", which names nothing of the group.
func privateDocument(props PageProps, body ...Node) Node {
	props.Title = "regroup"

	return document(props,
		Main(
			Div(Class(sign),
				Div(Class(signInner),
					Div(Class("text-[15px]/6"), Text("regroup")),
					H1(Class(signName), Text("This group is private.")),
				),
			),
			Div(Class(wrap+" pt-6 pb-24 lg:pt-8"),
				Div(Class("max-w-[608px]"), Group(body)),
			),
		),
	)
}

// signInRequestForm is the field to ask for a sign-in link with, holding the given address, and the given
// refusal of it under it, if there is one. The browser does not check the address first, so that every
// refusal of it is said in the same way, under the field.
func signInRequestForm(email string, refusal Message) Node {
	return Form(ID("sign-in-request"), Data("form", "sign-in-request"), Method("post"), Action("/sign-in"), Attr("novalidate"),
		fieldRow(
			Label(For("sign-in-email"), Class("sr-only"), Text("Email address")),
			field(ID("sign-in-email"), Type("email"), Name("email"), AutoComplete("email"), Placeholder("Your email address"), Value(email), refusalOf(refusal, "sign-in-email-message")),
			Button(Type("submit"), Class(primaryButton), Text("Send me a link")),
		),
		refused("sign-in-email-message", refusal),
	)
}

// SignInLinkPageProps is what the page a working sign-in link opens shows.
type SignInLinkPageProps struct {
	PageProps

	// GroupName on the sign. The email with the link named it already.
	GroupName string

	// Token of the link, which the form sends along.
	Token string

	// Joining, if true, is for the first admin, who joins the group by signing in, and chooses a name.
	Joining bool

	// Name that was typed, which is kept after a refusal.
	Name string

	// Refusal of the name, if it was refused.
	Refusal Message
}

// SignInLinkPage has the one button that signs in with a link, which sends the link's token, and the
// field for a name for the first admin.
func SignInLinkPage(props SignInLinkPageProps) Node {
	props.Title = "Sign in to " + props.GroupName

	return linkDocument(props.PageProps, props.GroupName,
		Iff(props.Joining, func() Node {
			return Group{
				P(Class("mb-6"), Text("You are the first in the group, and its admin. Choose the name the group will see.")),
				Form(ID("sign-in"), Data("form", "join-first"), Method("post"), Action("/sign-in/link"), Attr("novalidate"),
					Input(Type("hidden"), Name("token"), AutoComplete("off"), Value(props.Token)),
					nameRow(props.Name, props.Refusal, "Join"),
				),
			}
		}),
		If(!props.Joining,
			Form(ID("sign-in"), Data("form", "sign-in"), Method("post"), Action("/sign-in/link"),
				Input(Type("hidden"), Name("token"), AutoComplete("off"), Value(props.Token)),
				Button(Type("submit"), Class(primaryButton), Text("Sign in")),
			),
		),
	)
}

// InvitationPageProps is what the page a working invitation opens shows.
type InvitationPageProps struct {
	PageProps

	// GroupName on the sign. The email with the invitation named it already.
	GroupName string

	// Token of the invitation's link, which the form sends along.
	Token string

	// InvitedByName is who sent the invitation.
	InvitedByName string

	// Name that was typed, which is kept after a refusal.
	Name string

	// Refusal of the name, if it was refused.
	Refusal Message
}

// InvitationPage says who invited the person, and has the field for the name the group will see and the
// one button that joins, which sends the invitation's token.
func InvitationPage(props InvitationPageProps) Node {
	props.Title = "Join " + props.GroupName

	return linkDocument(props.PageProps, props.GroupName,
		P(Class("mb-6"),
			Span(Data("inviter", ""), Class("font-serif text-xl/7 font-[680] wrap-anywhere text-primary-700"), Text(props.InvitedByName)),
			Text(" invited you to join. Choose the name the group will see."),
		),
		Form(ID("join"), Data("form", "join"), Method("post"), Action("/invitation"), Attr("novalidate"),
			Input(Type("hidden"), Name("token"), AutoComplete("off"), Value(props.Token)),
			nameRow(props.Name, props.Refusal, "Join"),
		),
	)
}

// linkDocument with the sign that carries the given name of the group, for someone who is not signed in
// but has a link that works, and the given body under it.
func linkDocument(props PageProps, groupName string, body ...Node) Node {
	return document(props,
		Main(
			Div(Class(sign),
				Div(Class(signInner),
					Div(Class("text-[15px]/6"), Text("regroup")),
					H1(Class(signName), Text(groupName)),
				),
			),
			Div(Class(wrap+" pt-6 pb-24 lg:pt-8"),
				Div(Class("max-w-[608px]"), Group(body)),
			),
		),
	)
}

// nameRow is the field for the name the group will see, holding the given name, with the button with the
// given label beside it, and the given refusal of the name under it.
func nameRow(name string, refusal Message, label string) Node {
	return Group{
		fieldRow(
			Label(For("name"), Class("sr-only"), Text("Your name")),
			field(ID("name"), Type("text"), Name("name"), AutoComplete("name"), Placeholder("Your name"), Value(name), refusalOf(refusal, "name-message")),
			Button(Type("submit"), Class(primaryButton), Text(label)),
		),
		refused("name-message", refusal),
	}
}

// MembersPageProps is what the list of members shows.
type MembersPageProps struct {
	PageProps

	// GroupName on the sign.
	GroupName string

	// Actor is who the page is for. What they may do decides which controls the page has.
	Actor model.User

	// Members of the group, in the order they joined, with their email addresses if the actor may see them.
	Members []model.Member

	// Invitations that are waiting, with their times in the group's time zone, if the actor may see them.
	Invitations []model.Invitation

	// Now in the group's time zone, which decides whether a time names the year.
	Now time.Time

	// Invite is the state of the field to invite with.
	Invite InviteFormProps

	// Notice at the top of the page, such as why a role was not changed.
	Notice Message

	// Extra is put beside the person's name on the sign, if not nil.
	Extra Node
}

// InviteFormProps is the state of the form to invite an email address with.
type InviteFormProps struct {
	// Email that was typed, which is kept after a refusal.
	Email string

	// Refusal of the address, if it was refused.
	Refusal Message

	// Notice under the field, which says something that is not wrong, such as that an invitation is on
	// its way.
	Notice Message

	// Token of the form, which it sends along, so that sending it again does not invite twice. Each form
	// that is shown needs a new one.
	Token string
}

// MembersPage lists the members of the group, with their names and roles, in the order given. What the
// actor may see and use decides the rest, by [model.Allowed]: email addresses, the control to change the
// role of everyone but the actor, the field to invite, and the invitations that are waiting, each with
// the button to withdraw it.
func MembersPage(props MembersPageProps) Node {
	props.Title = "Members of " + props.GroupName
	actor := props.Actor

	return document(props.PageProps,
		memberSign(signProps{GroupName: props.GroupName, Members: len(props.Members), UserName: actor.Name, Here: "/members", Extra: props.Extra}),
		Main(Class(wrap+" pt-6 pb-24 lg:pt-8"),
			notice(props.Notice),
			H2(Class(headline), Text("Members")),
			Ul(ID("members"), Class("grid max-w-[840px] gap-4"),
				Map(props.Members, func(m model.Member) Node {
					return memberRow(actor, m)
				}),
			),
			If(model.Allowed(&actor, model.ActionSeeInvitations, nil), invitations(props)),
		),
	)
}

// memberRow of the given member in the list, as the given actor sees it.
func memberRow(actor model.User, m model.Member) Node {
	target := &model.User{ID: m.ID, Active: true, Role: m.Role}
	id := m.ID.String()

	return Li(Data("member", id), Data("role", m.Role.String()), Class("flex flex-wrap items-baseline gap-x-4 gap-y-1"),
		Span(Data("name", ""), Class("font-serif text-xl/7 font-[680] wrap-anywhere text-primary-700"), Text(m.Name)),
		Span(Class("text-[15px]/6 text-stone-600"),
			Text(roleName(m.Role)),
			If(m.ID == actor.ID, Text(", you")),
		),
		If(m.Email != "" && model.Allowed(&actor, model.ActionSeeEmailAddresses, nil), Span(Data("email", ""), Class("text-[15px]/6 wrap-anywhere text-stone-600"), Text(m.Email.String()))),
		If(model.Allowed(&actor, model.ActionChangeRole, target), roleControl(m)),
	)
}

// roleControl is what changes the role of the given member: a line that opens to a button for each role
// they do not have. It works without a script, since it is a disclosure and a plain form.
func roleControl(m model.Member) Node {
	id := m.ID.String()

	var buttons []Node
	for _, r := range model.Roles {
		if r == m.Role {
			continue
		}
		buttons = append(buttons, Button(Type("submit"), Name("role"), Value(r.String()), Class(secondaryButton+" h-11"),
			Text("Make "+roleWord(r)),
			Span(Class("sr-only"), Text(", "+m.Name)),
		))
	}

	return Details(Class("group w-full sm:w-auto"),
		Summary(Class("relative inline-flex cursor-pointer list-none rounded-sm text-[15px]/6 text-stone-600 before:absolute before:inset-x-0 before:-top-2 before:-bottom-3 hover:text-stone-900 [&::-webkit-details-marker]:hidden"),
			Span(Class("underline decoration-stone-400 decoration-1 underline-offset-4 group-hover:decoration-stone-900"),
				Text("Change role"),
				Span(Class("sr-only"), Text(" of "+m.Name)),
			),
		),
		Form(Data("form", "change-role"), Method("post"), Action("/members/role"), Class("mt-3 flex flex-wrap gap-2"),
			Input(Type("hidden"), Name("member"), AutoComplete("off"), Value(id)),
			Group(buttons),
		),
	)
}

// invitations section of the list of members: the field to invite with, and the invitations that are
// waiting, each with a button to withdraw it.
func invitations(props MembersPageProps) Node {
	actor := props.Actor
	withdraw := model.Allowed(&actor, model.ActionWithdrawInvitation, nil)

	return Section(Class("mt-12 lg:mt-14"),
		H2(Class(headline), Text("Invitations")),
		If(model.Allowed(&actor, model.ActionInvite, nil), inviteForm(props.Invite)),
		If(len(props.Invitations) == 0, P(Class("mt-6 text-stone-600"), Text("No invitations are waiting."))),
		If(len(props.Invitations) > 0,
			Ul(ID("invitations"), Class("mt-8 grid max-w-[840px] gap-5"),
				Map(props.Invitations, func(inv model.Invitation) Node {
					return Li(Data("invitation", inv.ID.String()), Class("flex flex-wrap items-center gap-x-4 gap-y-2"),
						Div(Class("min-w-0"),
							P(Data("email", ""), Class("wrap-anywhere"), Text(inv.Email.String())),
							P(Class("text-[15px]/6 text-stone-600"),
								Text("Invited by "), Span(Data("inviter", ""), Class("font-serif font-[680] wrap-anywhere text-primary-700"), Text(inv.InvitedByName)),
								Text(". Runs out "+expiry(inv.Expires.T, props.Now)+"."),
							),
						),
						If(withdraw, Form(Data("form", "withdraw-invitation"), Method("post"), Action("/invitations/withdraw"),
							Input(Type("hidden"), Name("invitation"), AutoComplete("off"), Value(inv.ID.String())),
							Button(Type("submit"), Class(secondaryButton+" h-11"),
								Text("Withdraw"),
								Span(Class("sr-only"), Text(" the invitation to "+inv.Email.String())),
							),
						)),
					)
				}),
			),
		),
	)
}

// inviteForm is the field to invite an email address with. The address is someone else's, so the browser
// does not fill in the person's own.
func inviteForm(props InviteFormProps) Node {
	return Form(ID("invite"), Data("form", "invite"), Method("post"), Action("/invitations"), Attr("novalidate"), Class("max-w-[608px]"),
		Input(Type("hidden"), Name("token"), AutoComplete("off"), Value(props.Token)),
		fieldRow(
			Label(For("invite-email"), Class("sr-only"), Text("Email address to invite")),
			field(ID("invite-email"), Type("email"), Name("email"), AutoComplete("off"), Placeholder("Email address to invite"), Value(props.Email), refusalOf(props.Refusal, "invite-message")),
			Button(Type("submit"), Class(primaryButton), Text("Invite")),
		),
		refused("invite-message", props.Refusal),
		If(props.Notice.Text != "", P(Data("message", props.Notice.Key), Role("status"), Class("mt-2"), Text(props.Notice.Text))),
	)
}

// expiry of an invitation at the given time, as words, such as "on Thursday 3 December at 10:00". Now
// decides whether it names the year.
func expiry(t, now time.Time) string {
	if t.Year() != now.Year() {
		return t.Format("on Monday 2 January 2006 at 15:04")
	}
	return t.Format("on Monday 2 January at 15:04")
}

// roleName of the given role, as the list of members shows it.
func roleName(r model.Role) string {
	switch r {
	case model.RoleAdmin:
		return "Admin"
	case model.RoleModerator:
		return "Moderator"
	default:
		return "Member"
	}
}

// roleWord of the given role, as it follows "Make" on a button.
func roleWord(r model.Role) string {
	switch r {
	case model.RoleAdmin:
		return "admin"
	case model.RoleModerator:
		return "moderator"
	default:
		return "member"
	}
}

// signProps is what the sign over a page for someone signed in shows.
type signProps struct {
	GroupName string
	Members   int
	UserName  string

	// Here is the path of the page, so that its link on the sign says it is the page.
	Here string

	// Extra is put beside the person's name, if not nil.
	Extra Node
}

// memberSign is the band across the top of a page for someone signed in: "regroup", the count of
// members, which links to the list of them, the person's name with the button to sign out, and the
// group's name, which links to the group page.
func memberSign(props signProps) Node {
	members := fmt.Sprint(props.Members, " members")
	if props.Members == 1 {
		members = "1 member"
	}

	current := func(path string) Node {
		return If(props.Here == path, Aria("current", "page"))
	}

	// link is the style of a link on the sign, and of the button that looks like one.
	link := "underline decoration-stone-900/50 decoration-1 underline-offset-4 hover:decoration-stone-900"

	return Header(Class(sign),
		Div(Class(signInner),
			Div(Class("flex flex-wrap items-baseline gap-x-5 gap-y-1 text-[15px]/6"),
				Span(Text("regroup")),
				A(Href("/members"), current("/members"), Class(link), Text(members)),
				Div(Class("ml-auto flex min-w-0 flex-wrap items-baseline gap-x-5"),
					Span(Data("person", ""), Class("min-w-0 font-serif text-lg/6 font-[640] wrap-anywhere"), Text(props.UserName)),
					Form(Data("form", "sign-out"), Method("post"), Action("/logout"), Class("shrink-0"),
						Button(Type("submit"), Class("cursor-pointer "+link), Text("Sign out")),
					),
					Iff(props.Extra != nil, func() Node {
						return Span(Class("shrink-0 [&_a]:underline [&_a]:decoration-stone-900/50 [&_a]:decoration-1 [&_a]:underline-offset-4 [&_a]:hover:decoration-stone-900"), props.Extra)
					}),
				),
			),
			H1(Class(signName), A(Href("/"), current("/"), Class("rounded-sm decoration-2 underline-offset-[0.12em] hover:underline"), Text(props.GroupName))),
		),
	)
}

// headline is the style of a heading of a part of a page below the sign, at the size of a day's.
const headline = "mb-6 font-serif text-[40px]/none font-[620] tracking-[-0.015em] text-balance lg:mb-7 lg:text-[64px]/none"

// primaryButton is the one orange button on a page.
const primaryButton = "inline-flex h-[52px] shrink-0 cursor-pointer items-center rounded-full bg-primary-500 px-[22px] text-stone-900 hover:bg-primary-400 active:bg-primary-600 disabled:cursor-default disabled:opacity-60"

// fieldRow of a one-line field with its button to the right of it.
func fieldRow(children ...Node) Node {
	return Div(Class("flex items-end gap-2"), Group(children))
}

// field that is one line, a pill, with the given attributes.
func field(children ...Node) Node {
	return Input(Class("h-[52px] w-full min-w-0 rounded-full border border-stone-500 bg-white px-[22px] text-stone-900 caret-primary-700 placeholder:text-stone-500 hover:border-stone-900 focus-visible:border-primary-700 focus-visible:-outline-offset-1"), Group(children))
}

// refusalOf a field: if there is one, the field is invalid, described by the message with the given ID,
// and has the keyboard focus when the page opens, so that the refusal is said and the field is ready.
func refusalOf(refusal Message, messageID string) Node {
	if refusal.Text == "" {
		return nil
	}
	return Group{Aria("invalid", "true"), Aria("describedby", messageID), AutoFocus()}
}

// refused says why what was typed in a field was refused, under it, with the given ID. Nothing if there
// is no refusal.
func refused(id string, refusal Message) Node {
	if refusal.Text == "" {
		return nil
	}
	return Div(Data("message", refusal.Key), message(id, refusal.Text))
}

// notice at the top of a page's body, saying why what was sent or opened was refused, as a refusal beside
// a field says it. Nothing if there is no notice.
func notice(n Message) Node {
	if n.Text == "" {
		return nil
	}
	return Div(Data("message", n.Key), Class("mb-6 [&>p]:mt-0"), message("notice", n.Text))
}
