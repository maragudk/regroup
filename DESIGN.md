---
name: regroup
description: Private groups anyone can host. A white page under a painted shop sign, where people's names are the loudest thing.
colors:
  primary: "oklch(70.5% 0.213 47.604)"
  primary-hover: "oklch(75% 0.183 55.934)"
  primary-active: "oklch(64.6% 0.222 41.116)"
  primary-ink: "oklch(55.3% 0.195 38.402)"
  primary-tint: "oklch(95.4% 0.038 75.164)"
  paper: "#ffffff"
  ink: "oklch(21.6% 0.006 56.043)"
  muted-ink: "oklch(44.4% 0.011 73.639)"
  field-stroke: "oklch(55.3% 0.013 58.071)"
  underline: "oklch(70.9% 0.01 56.259)"
  hairline: "oklch(92.3% 0.003 48.717)"
typography:
  display:
    fontFamily: "Fraunces, ui-serif, Georgia, serif"
    fontSize: "96px"
    fontWeight: 620
    lineHeight: 0.8
    letterSpacing: "-0.02em"
    fontVariation: "\"SOFT\" 100, \"WONK\" 0"
  headline:
    fontFamily: "Fraunces, ui-serif, Georgia, serif"
    fontSize: "64px"
    fontWeight: 620
    lineHeight: 1
    letterSpacing: "-0.015em"
    fontVariation: "\"SOFT\" 100, \"WONK\" 0"
  title:
    fontFamily: "Fraunces, ui-serif, Georgia, serif"
    fontSize: "36px"
    fontWeight: 640
    lineHeight: 1
    letterSpacing: "-0.005em"
    fontVariation: "\"SOFT\" 100, \"WONK\" 0"
  title-small:
    fontFamily: "Fraunces, ui-serif, Georgia, serif"
    fontSize: "20px"
    fontWeight: 680
    lineHeight: "28px"
    fontVariation: "\"SOFT\" 100, \"WONK\" 0"
  body:
    fontFamily: "Source Sans 3, ui-sans-serif, system-ui, sans-serif"
    fontSize: "17px"
    fontWeight: 400
    lineHeight: 1.6
  label:
    fontFamily: "Source Sans 3, ui-sans-serif, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: "24px"
rounded:
  pill: "9999px"
  field: "26px"
spacing:
  gap-tight: "8px"
  gutter-phone: "20px"
  gutter-tablet: "32px"
  gutter-desktop: "72px"
  column-gap: "32px"
  post-gap: "36px"
  day-gap: "56px"
  name-column: "200px"
  reading-column: "608px"
components:
  sign:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.ink}"
    typography: "{typography.display}"
    padding: "18px 72px 30px"
    height: "152px"
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.pill}"
    padding: "0 22px"
    height: "52px"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "{colors.ink}"
  button-primary-active:
    backgroundColor: "{colors.primary-active}"
    textColor: "{colors.ink}"
  button-secondary:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.pill}"
    padding: "0 18px"
    height: "44px"
  input-composer:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.field}"
    padding: "12px 22px"
    height: "52px"
  input-comment:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.field}"
    padding: "8px 18px"
    height: "44px"
  author-name:
    textColor: "{colors.primary-ink}"
    typography: "{typography.title}"
    width: "200px"
  comment-author-name:
    textColor: "{colors.primary-ink}"
    typography: "{typography.title-small}"
  new-mark:
    textColor: "{colors.primary-ink}"
    typography: "{typography.label}"
  new-dot:
    backgroundColor: "{colors.primary}"
    rounded: "{rounded.pill}"
    size: "9px"
  failure-bar:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    typography: "{typography.body}"
    padding: "12px 0"
---

# Design System: regroup

## Overview

**Creative North Star: "The Corner Shop"**

A group's page is a painted neighbourhood shop sign over a plain white shop floor. The sign is a full-width band of orange carrying the group's name in a soft, heavy serif. Below it, people talk: every author's name is set in that same serif, large and in orange, so the page reads as named people speaking rather than as a feed of items. Everything else is one quiet sans at one weight, on white, with as little interface as the task allows.

The system has two display moves and only two: **the sign** and **the names** (with the day headings, which share the names' serif and sit between them as the largest type below the sign). Everything that is not a person, a day, or the group is interface, and interface stays small, grey, and out of the way. There are no cards, panels, boxes, or shadows; structure comes from type size, a shared left edge, and white space. Nothing moves on its own.

The page is self-contained: both fonts, the stylesheet, the scripts, and the icon are served by the app itself, and no page makes a request to any third party.

**Key Characteristics:**
- A full-width orange sign at the top of every group page, carrying the group's name at up to 96px in near-black.
- Names, day headings, and the group's name in Fraunces (softness at full, leaning letters off, set heavy); everything else in Source Sans 3 Regular.
- Author names in orange 700, hung in a 200px left margin column on wide screens.
- White page, near-black text, stone greys for secondary text and lines. No cards, boxes, or shadows.
- Pill-shaped field and buttons; one orange button per page.
- Orange 500 appears below the sign only on the primary action and the "new" dot.

## Colors

One accent from Tailwind's default palette, used sparingly and with fixed meanings, on a white page with Tailwind's stone greys.

The accent is named `primary` everywhere in the pages. In `tailwind.css`, `--color-primary-50` to `--color-primary-950` are aliases of Tailwind's default orange scale, so that the accent can later be made configurable from the same palette by changing one place (see `docs/todo/accent-colour.md`). The frontmatter records orange because that is the shipped default; a builder must write `primary-500`, `primary-700`, and so on, never `orange-*`. A test fails if a page names orange directly.

### Primary
- **Shop-Sign Orange** (primary, Tailwind orange 500): the fill of the sign, the fill of the one primary action ("Post"), and the "new" dot. Nothing else. It never carries text on white, because it reaches only about 2.9:1 against it.
- **Lit Sign** (primary-hover, orange 400): the primary button under the pointer.
- **Pressed Sign** (primary-active, orange 600): the primary button while pressed.
- **Lettering Orange** (primary-ink, orange 700): every orange word on white: authors' names, the "new" mark's word, the keyboard focus outline, a focused field's border, and the text caret.
- **Paper Tint** (primary-tint, orange 100): the background of selected text, behind near-black.

### Neutral
- **Paper** (#ffffff): the page. There is no other surface colour.
- **Shop Ink** (ink, stone 900): body text, the lettering on the sign, button labels (including on orange), and the failure bar's fill.
- **Pencil** (muted-ink, stone 600): times, dates beside "Today", the comment count, "Loading comments", and the lines that say there is nothing more.
- **Field Line** (field-stroke, stone 500): the 1px border of fields and secondary buttons, and placeholder text. Darker than a hairline on purpose, so that a field reaches 3:1 against the page.
- **Underline** (underline, stone 400): the 1px underline of links and of the comments summary at rest; it turns Shop Ink on hover.
- **Hairline** (hairline, stone 200): the single vertical rule to the left of an open comment thread.

### Named Rules
**The One Meaning Rule.** Orange stands for the group, a person, or something new from one. Below the sign, Shop-Sign Orange fills exactly two things: the primary action and the "new" dot. The only other orange is Lettering Orange, for names, the "new" word, and keyboard focus.

**The Orange 700 Text Rule.** Orange text is primary 700 or darker, always. Primary 500 is for fills and marks only.

**The Name-It-Primary Rule.** Pages name the accent as `primary`, never by its hue, so a configured accent changes every page at once.

**The Not-Only-Colour Rule.** State is carried in form as well as colour: "new" is a dot and a word; a refusal is an icon and a sentence; a disabled button is dimmed and loses its pointer.

## Typography

**Display Font:** Fraunces, variable (with ui-serif, Georgia, serif)
**Body Font:** Source Sans 3, regular only (with ui-sans-serif, system-ui, sans-serif)

**Character:** A soft, heavy, rounded serif that looks painted rather than printed, against a plain, friendly sans that keeps out of its way. Fraunces is always set with `font-variation-settings: "SOFT" 100, "WONK" 0` (softness at full, leaning letters off) and its optical size follows the font size. Both files are self-hosted in `public/styles/fonts/` with their licence texts, and preloaded by every page.

### Hierarchy
- **Display** (Fraunces 620, 96px, line-height 0.8, -0.02em): the group's name on the sign, and only there. On narrower screens it is `clamp(44px, 13vw, 96px)` with line-height 0.9; it balances and breaks anywhere rather than overflow.
- **Headline** (Fraunces 620, 64px, line-height 1, -0.015em; 40px below 1024px): day headings ("Today", "Yesterday", "Monday 23 November"), the largest type below the sign. The headline of a plain message page uses the same style. "Today" and "Yesterday" carry the date beside them in Pencil at body size in the sans. On a phone a written-out date keeps only the weekday in the serif and sets the rest in the sans, so the heading stays one line of large type.
- **Title** (Fraunces 640, 36px, line-height 1, -0.005em; 28px below 1024px): the author of a post, in Lettering Orange.
- **Title small** (Fraunces 680, 20px/28px): the author of a comment, in Lettering Orange. The member's own name on the sign is the same serif at 18px/24px, weight 640, in Shop Ink.
- **Body** (Source Sans 3 400, 17px, line-height 1.6): post and comment text, fields, buttons. The reading column is at most 608px. Text keeps its line breaks and wraps anywhere, so a long word never overflows.
- **Label** (Source Sans 3 400, 15px/24px): everything small on the sign, times, the comments summary, the "new" mark, loading and end lines. Times use tabular figures. Never uppercase, never letter-spaced.

### Named Rules
**The Names Are Display Rule.** Every person's name is set in Fraunces in Lettering Orange, wherever it appears in the timeline. A name is never body text, and never accompanied by an avatar.

**The One Weight Rule.** The sans has one weight, 400. Emphasis comes from the serif, from size, or from colour, never from bold sans. Only the regular weight is shipped.

**The Heavier When Smaller Rule.** Fraunces gets slightly heavier as it gets smaller (620 at 64-96px, 640 at 18-36px, 680 at 20px for comment names), so the names keep their painted weight at every size.

## Layout

Everything sits in one column, `max-width: 1440px`, centred, with side padding of 20px on a phone, 32px from 640px, and 72px from 1024px. The sign's lettering, the day headings, and the margin names share that left edge.

From 1024px a post is a two-column grid: a 200px name column, a 32px gap, and a 608px reading column, aligned on the baseline so the author's name and the first line of the post share one. The composer, the "Earlier posts" control, and the empty and end lines start at the reading column (offset 232px). Below 1024px the name column goes: each name sits above its post with the time beside it, and everything starts at the left edge.

Rhythm: 24px (32px from 1024px) from the sign to the composer; 48px (56px) between days; 24px (28px) under a day heading; 36px between posts; 8px between a name and its text on a phone and between a post and its comments summary; 16px between comments; 8px between a field and its button. An open thread is indented 20px (24px) behind its hairline.

The space to the right of the timeline on a wide screen is empty on purpose. It is reserved for the "Coming up" list of events, which is not built yet. It is not part of the composition; do not fill it with anything else, and do not centre the timeline to use it up.

The sign scrolls away with the page; nothing is sticky. On a phone it is at least 104px tall with its lettering at up to 44-50px; from 1024px it is at least 152px.

Breakpoints are Tailwind's defaults: phones below `sm` (640px), tablets at `md` (768px), sideways tablets at `lg` (1024px), laptops and desktops at `xl` (1280px). The layout changes once, at `lg`.

### Named Rules
**The One Left Edge Rule.** The group's name, the day headings, and the margin names start on the same vertical line. A new surface that has a sign keeps it.

**The Reserved Margin Rule.** The right side of a wide screen belongs to upcoming events. Leave it empty until they ship.

## Elevation & Depth

The system is flat. There are no shadows, no tonal layers, no cards, and no surfaces above the page except the failure bar, which is a flat band of Shop Ink fixed to the foot of the window. Grouping comes from type size, the shared left edge, and space; the only drawn lines are field and button borders, link underlines, and the hairline beside an open thread.

### Named Rules
**The Flat Shop Floor Rule.** Nothing on the page casts a shadow or sits in a box. A post is a name and its text, never a card.

## Shapes

Two shapes: the band and the pill. The sign and the failure bar are full-width rectangles with no rounding. Every control is a pill: the primary and secondary buttons are fully rounded, and fields are rounded at 26px, so a one-line field is a pill and a field that has grown with its text becomes a rounded rectangle. The "new" dot is a 9px circle. Borders are 1px.

## Components

### Buttons
Plain words in pills. One orange button per page.
- **Shape:** fully rounded pill.
- **Primary** ("Post"): Shop-Sign Orange fill, Shop Ink label, 52px tall, 22px side padding. It is the only filled button.
- **Hover / Active:** Lit Sign under the pointer, Pressed Sign while pressed. No movement, no shadow.
- **Secondary** ("Comment", "Earlier posts"): white with a 1px Field Line border and Shop Ink label, 44px tall, 18px side padding; the border turns Shop Ink on hover.
- **Disabled:** 60% opacity and a default cursor. A button is off while its own request is on its way, and its label may say what is happening ("Loading earlier posts").
- **Focus:** a 2px Lettering Orange outline 3px outside the shape; on the sign, the outline is Shop Ink.

### Inputs / Fields
- **Style:** a white pill with a 1px Field Line border, Shop Ink text, Field Line placeholder, and a Lettering Orange caret. One line tall at rest; it grows with what is written, up to half the window, and is never resized by hand. The composer is 52px with 22px side padding; a comment field is 44px with 18px. The button sits to the right, aligned to the field's bottom.
- **Hover:** border turns Shop Ink.
- **Focus:** the border turns Lettering Orange and the focus outline is drawn on it.
- **Labels:** visually hidden; the placeholder says what goes there ("Write to the group", "Write a comment").
- **Refusal:** a line under the field in Shop Ink, led by an 18px outline circle-with-exclamation icon drawn inline as SVG in `currentColor`. There is no red in the system.

### The Sign (signature component)
A full-width Shop-Sign Orange band at the top of a group page. Its top line, in the label size, carries "regroup" and the member count on the left and the member's own name (Fraunces) and any extra link on the right. Below it, the group's name in Display. All lettering is Shop Ink. The same sign, with "This group is private." as its name, is the page shown to anyone not signed in. Plain message pages (errors) have no sign: just a headline, a sentence, and a link back to the group.

### The Names (signature component)
A post's author in Title, in Lettering Orange, in the name column on a wide screen or above the text on a phone, with the time in Pencil beneath or beside it. A comment's author in Title small, with the time beside it. Names never carry avatars, roles, badges, or counts.

### New Mark
A 9px Shop-Sign Orange dot followed by a word in Lettering Orange at label size: "new" beside a post or comment, "2 new" beside a comments summary. Always the dot and the word together, so it does not rely on colour.

### Comments Summary and Thread
The comments of a post open in place under it, without leaving the timeline. The summary is a label-size underlined link ("Comment", "1 comment", "4 comments"), in Pencil, turning Shop Ink on hover, with a hit area stretched to 44px without taking up room. The open thread is a flat list behind a single Hairline rule on its left, then the comment field. The list fades in over 180ms, and not at all under reduced motion.

### Links
Shop Ink text with a 1px Underline-coloured underline, 4px below the text; the underline turns Shop Ink on hover.

### Failure Bar
When a request from the page fails, a flat band of Shop Ink with white body text appears at the foot of the window, in the page's column: "That did not go through. Try again, and reload the page if it keeps happening."

## Do's and Don'ts

### Do:
- **Do** name the accent as `primary-*`, never `orange-*`.
- **Do** set every person's name in Fraunces in primary 700, at Title size for authors of posts and Title small inside threads.
- **Do** keep primary 500 to the sign, the one primary action on a surface, and the "new" dot.
- **Do** put orange text in primary 700 or darker.
- **Do** carry state in form as well as colour: a dot and a word, an icon and a sentence.
- **Do** keep the sign's lettering, headings, and margin names on one left edge, and keep the reading column at most 608px.
- **Do** use pills for every button and field: one filled primary, everything else outlined in stone 500.
- **Do** serve every font, script, stylesheet, and image from the app itself.
- **Do** keep openings to a short fade (180ms), wrapped in `motion-safe`, and let nothing move on its own.
- **Do** leave the right side of a wide screen empty until upcoming events fill it.

### Don't:
- **Don't** put posts, comments, or anything else in cards, panels, boxes, or shadows.
- **Don't** add a second filled orange button to a surface, or use primary 500 for text on white.
- **Don't** add avatars, counters beside names, badges, or a grey feed background.
- **Don't** use bold or uppercase letter-spaced sans for emphasis or labels; the sans has one weight.
- **Don't** introduce a colour outside Tailwind's default orange (as `primary`) and stone scales, including red for errors.
- **Don't** load anything from a third party: no font host, no CDN, no analytics, no embedded widget.
- **Don't** show a control for something that is not built yet.
- **Don't** make the sign sticky; it scrolls away with the page.
