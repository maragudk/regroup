# Diary: The first group page

Regroup is still its starter template. This diary covers the work towards its first real surface, the group page: the product record and design direction agreed with Markus, the plumbing a group needs before anyone can use it (signing in, roles, invitations), and then the page itself.

## Step 1: Product record, design direction, and requirements

**Author:** main

### Prompt Context

**Verbatim prompt:** /impeccable:impeccable init

Then, after the product record was written: "Let's keep the PR open and continue work: first surface"
**Interpretation:** Capture regroup's durable product facts in `/PRODUCT.md` with the impeccable design skill, then decide with Markus what the first surface is, what it must do, and what it looks like, before any builder starts.
**Inferred intent:** Give the agents a settled product and design basis, so that the first build is the real product and not a restyled template.

### What I did

I ran the impeccable `init` flow. I read `/docs/core-values.md`, `/README.md`, `/docs/spec.md`, and the template's pages, interviewed Markus, and wrote `/PRODUCT.md`. It went up as pull request #5.

We then shaped the first surface. Markus's decisions, in order:

- Regroup is for any private community, not only people who know each other offline. The shared office stays the test case.
- A group has discussions, events, and photos, but no generic file sharing.
- The first surface is the group page. A group is one stream, and Markus turned that into a timeline: "Maybe not just posts? Since events are coming, maybe it's more of a timeline".
- The timeline holds only what was posted. An event is a post with a date, and upcoming events get their own list. Things stay where they were posted, and a post with unseen comments is marked for that reader only.
- The design covers posts, events, and photos. The first build ships posts and comments.

For the look I followed impeccable's new-work flow: seven candidate worlds of my own, a roll that assigns one, and catalogue challengers weighed against it. Sub-agents drew each full card as an HTML sketch of the same page with the same content, and Markus chose on impeccable's decision page.

- First hand: a risograph bulletin (assigned), a cabin logbook (my pick), airport signage (challenger), and the category standard. Markus: "I don't like these." He steered: Tailwind's default orange, friendly typography, minimal interface, or else a neo-grotesque.
- Second hand, all in Tailwind orange: The Corner Shop (assigned: a soft serif for people's names on a bare white page), Plain Orange, One Orange Key (neo-grotesque), and Orange Board. He chose The Corner Shop, then found it "a bit too bland".
- Three bolder versions, each with one decisive move: The Sign, The Names, and The Orange Room. He chose The Sign and The Names together, with the sign smaller.
- Fonts: his earlier design used Recoleta, which cannot ship in an open-source repository. He first proposed a switchable app-wide font, then withdrew it because a font "can change the layout drastically in a way color selection can't", and asked for a configurable accent colour from Tailwind's palette instead. A sub-agent researched open-licence soft serifs (`/docs/research/soft-serif-fonts.md`), and others set the combined design in Fraunces, Cooper*, Corben, and Young Serif. He chose Fraunces, self-hosted.

Last, the plumbing. Markus decided: sign-in by emailed link, three roles (admin, moderator, member) with moderators and admins inviting, and one instance holding one group.

I recorded the decisions in `/PRODUCT.md`, `/docs/todo.md`, `/docs/todo/group-page.md`, `/docs/todo/people.md`, `/docs/todo/accent-colour.md`, and the design brief with its direction contract in `/.impeccable/surfaces/html-group-go.md`. The to-do files separate what Markus agreed from what I decided for him to overrule.

### Why

Nothing existed beyond a placeholder page, so the first surface sets both the product's core loop and its look. Settling them with Markus first means builders get requirements rather than guesses, and the choices he made are written where a later session can find and change them.

### What worked

- Showing every direction as the same page with the same realistic content. Markus could reject a whole hand in one look and say why.
- Asking for one decisive move per bolder version. The combination he chose came from two of them.
- Choosing the font in place. The same design set in each candidate showed differences that a specimen would not.
- Having the font licences researched, not recalled. The note excluded Fontshare faces on licence grounds, which I would not have known to do.

### What didn't work

- My first commit ran `git commit -m "Add \`PRODUCT.md\` ..."` through the REPL's shell template, which strips the backslashes. The shell then treated the backticks as command substitution, printed `(eval):1: command not found: PRODUCT.md`, and committed with the subject "Add  with the product record for design work". The amend and force-push to fix it were refused by the permission classifier, so the subject stays. Commit messages now go in as an interpolated value.
- The decision page binds to `127.0.0.1`, and Markus works from another machine. My first relay was refused by the permission classifier. His SSH tunnel got a 403, because the page's server rejects any Host header other than `127.0.0.1:<port>` or `localhost:<port>`. After he asked for it ("Can't you just listen on all interfaces instead of localhost?"), a small relay that rewrites the Host header worked.
- Waiting for the page's answer in a script that had already returned failed: "This tool call did not run. The code that made it finished or was stopped before the call could start." The wait has to be awaited inside one script.
- In the first hand I told the logbook sketcher to put its ribbon, the signature move, below the first screen. The agent pointed it out and I had it moved.
- `impeccable shot <file>` writes to one shared path, so parallel sketch agents saw each other's screenshots. `--out <dir>` gives each its own directory; the value is a directory, not a file name.
- Fraunces's leaning letters did not show at the design's sizes. They appear only at the largest optical size and light weights, so the lean stays off.

### What I learned

- Markus rejects quickly and steers precisely once he sees something. Words alone had not surfaced that he already had a look in mind (orange, soft type, minimal interface).
- Tailwind's orange 500 on white is about 2.9:1, so it cannot carry text. Orange text is orange 700, and orange 500 is for fills and marks.
- The template has sessions, a logout route, a tokens table, a Postmark sender, and email templates, but no sign-in flow, no way to create a user, and no pages. A sub-agent's survey found this.

### What was tricky

- Keeping the sketches comparable. The page's organisation and content had to be fixed in one shared brief, with each world allowed only type, palette, density, and one signature move.
- The first screen is 900px tall, and the first content order hid the event post, the open comments, and the retracted comment below it. I reordered the content and made the composer compact for the later rounds.

### What warrants review

- `/PRODUCT.md`: the five product principles are my derivation from the core values, and Markus has not confirmed their wording.
- `/docs/todo/group-page.md` and `/docs/todo/people.md`: the "Decided by the lead" lists, especially that posts have no titles, that comments are flat, and who sees email addresses.
- `/.impeccable/surfaces/html-group-go.md`: the direction contract the group page build will be reviewed against.
- The sketches under `/.impeccable/mocks/decision/` are working material and are not committed. The chosen one is `font-fraunces.html`.

### Future work

- Build sign-in, roles, and invitations, then the group page.
- Propose spec wording for the rules agreed today, through the spec skill.
- Events, photos, and the configurable accent colour, each in its to-do file.
