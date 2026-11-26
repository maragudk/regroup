# Verification

How we know regroup does what it should, given that Markus does not read the code. Discussed on 2026-11-26.

## Agreed

- Regroup has no test oracle. Nothing outside the project knows the right answer, as SQLite does for s4ql. For product behaviour, Markus is the oracle: "For product behavior, I guess I'm the oracle? But lean towards building something and then we can adjust. Nothing is in production until I say so, so we can break stuff and iterate."
- Markus judges each piece of work in two ways:
  - A preview: a regroup instance on the Mac mini, reachable over Tailscale, filled with a made-up office group and updated as work lands. Each report from the lead ends with a short list of things to try.
  - A private page of screenshots covering every new screen and state, on desktop and phone.

## Decided by the lead, for Markus to overrule

- While the product is still taking shape, automated checking stays light: the builders' ordinary tests, lint, CI on every pull request, a fresh reviewer for each feature, and for every fix a test shown to fail with the fault in place.
- Markus's verdicts are written into the to-do file of the piece of work. A behaviour he has approved gets a test, so that later changes cannot quietly undo it. Where a verdict is a guarantee the group can rely on, the lead proposes wording for `docs/spec.md`.
- The preview runs what is merged to `main`.

## Open

- What has to be true before Markus says "production". The lead's proposal, not yet discussed: a small model of a group checked against generated histories of made-up people; a sweep that every page refuses outsiders; reviews by other models with the spec's rules as the things to break; planted faults and mutation testing to prove the tests have teeth; a security review of signing in and invitations.
- Whether time, randomness, and email should be replayable from a seed from the start, as in s4ql, or added when the heavier checks are.
- How sign-in emails reach Markus on the preview.
- Checks for the promises in the core values, such as that no page asks a third party for anything.

## Found while evaluating tools

- With the settings in `.env.example`, the content security policy blocks Datastar, so nothing interactive can run. Bombadil reported it on its first run, and no Go test catches it. See `docs/research/bombadil.md`. It has to be fixed before the first interactive page.
