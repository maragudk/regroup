# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **Members** of a private group. Regroup is for any private community: people who already know each other, such as a shared office or a club, and online-only communities alike. A member joins because someone they trust invited them.
- **Group admins and moderators**, who are members looking after their own group.
- **Hosts**, who run a regroup instance. Anyone can host. Who typically hosts is not yet established.

The first real group is Markus's shared office space. It is the test case for deciding what to build first.

## Product Purpose

Regroup is an open-source (MIT) web app for private groups that anyone can host. It exists because the dominant platforms treat their users as products, and regroup does the opposite. It is infrastructure for self-governing groups, not a community itself.

What success means, beyond the shared office group using it, is not yet defined.

## Positioning

"Like Groups, but it doesn't Zuck."

- Groups are private and found only by invitation. There is no directory, discovery, trending page, or recommendation engine.
- Nothing in the software gains from holding attention. It has no advertising, no visitor analytics, and no measure of time spent.
- Anyone can host it and anyone can change it. Content belongs to the group, and a group can leave a host and take its history along.
- Where the interests of one person and the group pull apart, regroup favours the group, and does so gracefully.

## Operating Context

- The pace is closer to a forum or email than to a group chat. People take part on their own schedule.
- The default notification is a daily digest by email. Each person sets the frequency for themselves, per group, from no emails up to immediate. Nobody else can override it, not even a group admin.
- `regroup.social` is a marketing site only. It is separate from this repository.
- The repository is still close to its starter template: one placeholder page, and no groups, posts, or moderation yet.

## Capabilities and Constraints

Confirmed for a group:

- Discussions: posts and comments.
- Events.
- Photos. Generic file sharing is not part of the product.

Confirmed by the core values (`docs/core-values.md`), which win over any feature that conflicts with them:

- A member can retract a comment. It is folded away and marked as retracted by its author, and the group can still open and read it.
- Quoting someone to reply to them is fine.
- A group can protect itself from harmful content without waiting for a moderator, in a way that cannot be used to silence a member.
- Moderators can hide content and, as a last resort, remove a member. Each action is visible to the group: what happened, who did it, and why.
- Hard deletion is the exception, and a note remains that something was removed.
- Group admins can export their group's full history.

Will not be built:

- Downvotes, negative reactions, or carrying a post out of its conversation to mock it.
- Strike counts, badges of shame, or any record that follows a member into other groups.
- Algorithmic feeds, engagement-optimized notifications, read receipts, typing indicators, or presence signals.
- Visitor analytics, behaviour tracking, advertising, or any use of a group's content beyond showing it to the group.

Technical constraints: server-rendered HTML from Go with gomponents, Datastar for interactivity, Tailwind CSS, and SQLite.

Open decisions:

- What a group offers beyond discussions, events, and photos.
- How a group protects itself without a moderator.
- The voice of the app itself (see Brand Commitments).
- An accessibility standard.

`docs/spec.md` holds the rules Markus and the lead have explicitly agreed. Where it and this file disagree, the spec wins.

## Brand Commitments

- The name is regroup.
- The tagline is "Like Groups, but it doesn't Zuck." It is dry and a little cheeky. Whether the app itself speaks in that voice is undecided.
- `docs/core-values.md` is binding on everything the software does and says.

## Evidence on Hand

- `docs/core-values.md`: the five values, with their concrete commitments.
- `README.md`: the tagline.
- There is no logo, no screenshot of a working group, no testimonial, and no usage figure. Do not invent any. No group uses regroup yet.
- The fonts in `public/styles` and the indigo primary colour in `tailwind.css` come from the starter template. Nobody has chosen them for regroup.

## Product Principles

1. **The group comes first, gracefully.** When one person's interest and the group's pull apart, favour the group without humiliating the person.
2. **Kindness is the easy path.** Make it easy to be kind and awkward to be cruel.
3. **Nothing disappears silently.** Retractions, moderation, and removals stay visible to the group.
4. **Never compete for attention.** The least intrusive useful option is the default, and each person controls their own.
5. **The software serves the group, not the host.** Collect only what is needed to function, and keep groups private.
