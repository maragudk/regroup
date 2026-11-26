# Verification

How we know regroup does what it should, given that Markus does not read the code. Discussed on 2026-11-26. The rules every builder follows are in the Testing section of `AGENTS.md`. This file holds the reasoning and what is not built or not decided.

## Agreed

- Regroup has no test oracle. Nothing outside the project knows the right answer, as SQLite does for s4ql. For product behaviour, Markus is the oracle: "For product behavior, I guess I'm the oracle? But lean towards building something and then we can adjust. Nothing is in production until I say so, so we can break stuff and iterate."
- Markus judges each piece of work in two ways:
  - A preview: a regroup instance on the Mac mini, reachable over Tailscale, filled with a made-up office group and updated as work lands. Each report from the lead ends with a short list of things to try.
  - A private page of screenshots covering every new screen and state, on desktop and phone.
- The app is to be robust from the first build. Markus: "I'd also like to use property-based testing with hegel in the Go code where it makes sense", and bombadil, "and also the usual unit and integration testing, and fuzz testing where it makes sense. Let's make this app robust." He agreed the plan below the same day.

| Kind of test | What it is for here | When it runs |
|---|---|---|
| Unit and integration, against real SQLite and the real router | Every behaviour, and every verdict Markus gives | Every pull request |
| Property-based with hegel, simple | Grouping by day across time zones, escaping of post text, paging, sign-in tokens | Every pull request |
| Property-based with hegel, stateful | Generated sequences of actions: people and roles, sign-in links working once and expiring, timeline order and the "new" marks | Every pull request, longer runs nightly |
| Go fuzzing | Anything that parses what a person sends: form values, paging cursors, later Markdown and photo uploads | Short on every pull request, longer nightly |
| Bombadil | Browser faults, and product rules as checks per role | A short run per piece of work, nightly on `main`, not a merge gate until it has run a while without false alarms |
| Screenshots and the preview | Markus's look | Every piece of work |

- The clock and randomness are passed in from the first build.
- There is one test-only way to sign in, used by bombadil and by the preview, and it is compiled out of production by a build tag.
- A nightly run opens an issue when it fails.
- Mutation testing, reviews by other models, and a security review wait for the gate before production.

## Decided by the lead, for Markus to overrule

- Markus's verdicts are written into the to-do file of the piece of work. Where a verdict is a guarantee the group can rely on, the lead proposes wording for `docs/spec.md`.
- The preview runs what is merged to `main`.

## Not built yet

- The preview instance. Its made-up office group and its one way to sign in as anyone are built behind the `preview` build tag (`make preview`).
- Longer nightly runs of the stateful hegel tests and of fuzzing. Bombadil's nightly run is built (`.github/workflows/bombadil.yml`).
- What bombadil still needs from the app: a seeded database with one person per role, stable markers on moderation notes, and stable markers on the name of the person signed in and on the names of authors, which `bombadil/page.ts` finds by the page's structure for now. Posts, comments, days, and the "new" mark have stable markers, and a failed Datastar request shows the message in `#request-failed`. They are listed in step 3 of `docs/diary/2026-11-26-first-group-page.md`.

## Open

- What else has to be true before Markus says "production", beyond mutation testing, reviews by other models, and a security review.
- How sign-in emails reach Markus on the preview.
- Checks for the other promises in the core values. That no page asks another host for anything is checked by tests, and for the built stylesheet by the Docker build.
- When bombadil has run long enough without false alarms to become a merge gate.
- How well hegel shrinks long stateful failures, and whether a failure can be replayed from its printed code.
