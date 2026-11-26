# Group page

The page a member opens: the group's timeline. Agreed with Markus on 2026-11-26. The design brief for it is in `.impeccable/surfaces/`.

## Agreed

- A group has one timeline, not topics. Markus: "maybe it's more of a timeline", since events are coming.
- The timeline holds only what was posted, at the time it was posted. It does not run into the future.
- An event is a post with a date. Upcoming events also get their own list beside the timeline.
- Things stay where they were posted. A new comment does not move a post up. Instead the post is marked as having comments the reader has not seen. Only the reader sees their own marks.
- A group has discussions, events, and photos. It has no generic file sharing.
- The design covers posts, events, and photos. The first build ships posts and comments. Events come next, then photos.
- The look is "The Corner Shop": a white page with minimal interface, a soft serif for people's names and day headings, and Tailwind's default orange. Markus: "Let's just use the Tailwind default orange for it, so we don't have to reinvent colors."
- It is the bolder version of that look, combining two moves. The Sign: an orange band across the top carries the group's name very large. The Names: day headings are the largest type on the page, and each author's name hangs large in orange in the left margin. Markus: "Maybe both The Sign and The Names? But the sign a little smaller."
- The serif is Fraunces, under the SIL Open Font License. Markus: "Let's go with Fraunces, but don't load it from Google fonts." The research behind the choice is in `docs/research/soft-serif-fonts.md`.

## Decided by the lead, for Markus to overrule

- A post has no title, only body text.
- Comments are a flat list, not nested, and open in place under their post.
- The sans is Source Sans 3. Both fonts are served by the app itself, with their licence texts in the repository, so no request goes to a font host.
- Fraunces is set with its softness at full, its leaning letters off, and heavier than in the sketch, towards the weight of Young Serif, which Markus liked.
- Orange text uses orange 700, because orange 500 on white has too little contrast to read. Orange 500 is for fills and marks.

- What counts as new to a reader. A post by someone else is new if it was posted since the reader's previous look at the timeline. A comment by someone else is new until the reader opens that post's comments. Nothing a person wrote is ever new to them, and nothing from before they joined is new. This is a first version for Markus to try on the preview.
- Days are grouped in the group's own time zone, which the host sets in the configuration. One instance holds one group, and a group is usually in one place.
- The first build shows no control for anything that is not built. "Add event", "Add photo", "Coming up", and the notification setting appear when they work.
- The sign scrolls away with the page. On a phone it is smaller, and each name sits above its post, not in a margin.
- Posts and comments are plain text with line breaks kept. There is no formatting and no editing yet.
- Limits and details chosen by the first builder are in step 2 of `docs/diary/2026-11-26-first-group-page.md`: a post is at most 10,000 characters and a comment 2,000, a page holds 20 posts, and "yesterday" is the previous calendar date in the group's time zone.
- Details chosen by the page builder are in step 3 of the same diary: the wording on the page, what the sign carries, how comments and earlier posts are asked for, and what a browser without JavaScript gets.
- Opening or reloading the timeline counts as a look, so a reload clears the marks on posts. Posting, commenting, and loading earlier posts happen in place and do not count as a look.
- Until sign-in is built, a visitor who is not signed in sees only "This group is private." The people build decides what the sign-in page shows, including whether it names the group.
- The app's icon is a plain square in the primary orange, served by the app itself.
- The rest of the work is split across builders in this order: bombadil with its CI job and the nightly run; then the design review and the preview.

## Open

- The layout on a phone.
- What a photo post and an event post look like in detail.
- What the real office group is called. The sketches used the made-up name "The Loft".

## Known and not fixed

- A page left open over midnight keeps "Today" until someone posts or reloads.
- The not-found page is sent without `Cache-Control: no-store`. It shows nothing of the group. Fixing it needs a change in glue.
- The icon is a fixed orange square, so it will not follow a configured accent colour.
- The same post sent again is shown without the comments it got in the meantime, until a reload.
- After a post, the answer replaces the composer before it adds the post. If the answer breaks off in between, the field is empty, the post is not on the page, and the form has a new token, so writing it again makes a second post.
- Fraunces is served as TrueType, 384,052 bytes against 210,032 as WOFF2, and the compression in glue does not cover `font/ttf`, so it goes out uncompressed.
- A post sent again from a page that shows its day only in part can land above newer posts of that day until a reload. Since the browser no longer fills in a form's hidden fields again, only a crafted request or a clock that went back gets there.
- A post from a day older than the newest day the page shows is added only if the page has its day, and otherwise shows after a reload, though the field empties and screen readers hear that it is on the timeline. That happens when the host's clock goes back past midnight.
- A browser without `:has()` in CSS (Firefox before 121, Safari before 15.4) shows a new post only after a reload.
- A browser that can neither size a field to its content nor run the page's script keeps the fields one line tall, and they scroll.
- After the first post on an empty timeline, "Nothing was posted before this." shows only after a reload, since someone else may have posted in the meantime.
- On a phone, "Yesterday" with a long weekday, and a date written out with its year, put their date on a second, small line. Between 640 and 1024 pixels wide, a group name with a long word breaks inside the word.
- Comments whose opening never gets an answer cannot be opened again until the request ends.

## Not in the first build

Events, photos, retracting a comment, moderation, the daily digest, notification settings, and export.
