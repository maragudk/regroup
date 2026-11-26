---
version: 1
slug: "html-group-go"
primary_target: "html/group.go"
related_targets: []
---

# Group page

Scope: the group page, the first surface of regroup. Visitor mode: Operate.

## Audience and job

A member of a private group, at a desk or on a phone, looking in once a day or less, often from the daily digest email. They want to see what the group has said since they last looked, read it at their own pace, and write back.

## Content and structure

- One timeline per group, newest first, grouped by day. Things stay where they were posted. A new comment does not move a post.
- A post has an author, a time, and body text. It has no title. Comments are a flat list and open in place under the post.
- A retracted comment is folded away and marked, and the reader can open it.
- Anything with comments the reader has not seen carries a "new" mark. Only the reader sees their own marks.
- An event is a post with a date. Upcoming events also appear in a "Coming up" list beside the timeline.
- A post can carry a photo.
- The first build ships posts and comments. Events and photos follow, and the design already makes room for them.

## Constraints

- Minimal UI: as little interface as the task allows. No cards, panels, shadows, or boxes around posts.
- Colour comes from Tailwind's default orange and stone scales only, and pages name the accent through the `primary` colour names so that it can later be changed in one place. Orange 500 never carries text on white (about 2.9:1); orange text is orange 700 or darker.
- Fonts are served by the app itself. No request leaves for a font host or any other third party.
- WCAG AA contrast. State is carried in form as well as colour.

## Direction contract

THESIS: People are the loudest thing on the page. The timeline reads as named people talking, with every name set as display type and the interface nearly gone. It refuses the social feed of white cards on grey with avatars and counters.

OWN-WORLD: The Corner Shop, after painted neighbourhood shop signs. A white page under an orange 500 sign. A soft serif (Fraunces, softness at full, leaning letters off, set heavy) for the group's name, the day headings, and every author's name, with names in orange 700. One quiet sans (Source Sans 3) at one weight for everything else. Hairlines only. Pill-shaped field and buttons. Below the sign, orange 500 fills only the primary action and the "new" dot. Orange means one thing: the group, a person, or something new from one.

STORY: A member arrives, sees at once who has said something new, reads, and answers or says they are in.

FIRST VIEWPORT: The Sign: a full-width orange band about 150px tall carrying the group's name at about 96px in near-black, with "regroup", the member count, the member's name, and their notification setting small on the same band. Below it, on white, The Names: a one-line composer with "Add event", "Add photo", and the orange "Post"; day headings at about 64px, the largest type below the sign; each author's name at about 36px in orange 700 hung in a left margin column with the time small beneath; post text at reading width beside it. A side column with "Coming up" as a plain list. The sign lettering, day headings, and margin names share one left edge. The primary action is "Post".

FORM: The Corner Shop, candidate 6 of 7 on the second hand; seed:f1bbcc7f5861b410. Signature moves: the sign, and names as display type. Signature interaction: comments open in place under their post without leaving the timeline. Motion grammar: nothing moves on its own; openings are a short fade, and none under reduced motion. The chosen sketch is `.impeccable/mocks/decision/font-fraunces.html`, to be set heavier than drawn there.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Unresolved

- The look of a photo post and the details of an event post beyond the sketch.
- The layout on a phone.
- How the sign behaves on scroll and on a phone: it spends height on every visit, so it has to shrink or scroll away gracefully.
- The accent colour will later be configurable from Tailwind's default palette. See `docs/todo/accent-colour.md`.
