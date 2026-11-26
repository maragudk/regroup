# People: signing in, roles, and invitations

What the group page needs before anyone can use it. Agreed with Markus on 2026-11-26.

## Agreed

- One instance holds one group.
- A member signs in with a link sent to their email address. There are no passwords.
- A group has three roles: admin, moderator, and member. Markus: "I think there'll be admins, moderators, members. Moderators and up can invite."
- People join by invitation from a moderator or an admin.

## Decided by the lead, for Markus to overrule

- An invitation goes to an email address, and the link in it lets that person in. There is no shareable invite link.
- A person who is invited joins as a member and chooses the name the group sees.
- The host names the first admin's email address in the configuration. That person signs in and invites the rest.
- The group's name comes from the host's configuration for now.
- Admins can change another person's role. Nobody can change their own.
- Asking for a sign-in link with an address the group does not know looks the same as asking with a known one, and sends nothing. So nobody can use the form to find out who is in the group.
- A sign-in link works once and for 15 minutes. An invitation link lasts 7 days. Signing in uses up the person's other sign-in links.
- A link in an email opens a page with one button, and pressing the button is what signs in or accepts. Mail scanners open links, and would otherwise use them up.
- At most 5 sign-in links go to one address in an hour. Asking for more looks the same and sends nothing.
- A person joins when they accept their invitation, not when it is sent. So nothing posted before they accepted is new to them.
- Inviting an address again replaces the earlier invitation. Moderators and admins can see the invitations that are waiting, and withdraw one.
- A name is up to 50 characters and need not be unique: two people may both be called Thomas.
- All members see the list of members with their names and roles. Only admins see members' email addresses. Moderators and admins see the addresses of invitations that are waiting.
- There is always at least one admin. It follows from the rule that nobody changes their own role.
- The preview sends no email. It shows what would have been sent on a page that only the preview build has.
- Someone who is not signed in sees "This group is private." and a field to ask for a sign-in link. The page does not name the group. The pages that a working link opens do name it, since the email already did.
- After asking for a link, everyone gets the same sentence, whether or not the address belongs to a member.
- The pages for people are plain forms that work without JavaScript, because a link in an email may open in any browser.
- The member count on the sign links to the list of members, where moderators and admins invite, and admins change roles. "Sign out" sits beside the person's name on the sign.
- The work is split across builders: the rules and emails first, then the pages (asking for a link, the page a link opens, accepting an invitation, the list of members with inviting and roles, and signing out).

## Open

- Removing a member. It belongs with moderation: the core values make it a last resort that the group can see.
- A page where an admin edits the group's name and other settings.
- What the emails say, and in which voice. The in-app voice is still undecided. The first wording is in step 2 of `docs/diary/2026-11-26-people.md`.
- A limit per visitor on asking for sign-in links. Today every request makes a job, and anyone can use up a member's five links an hour and keep them out for up to an hour.
- How a former member comes back. Their address cannot be invited again for now.
