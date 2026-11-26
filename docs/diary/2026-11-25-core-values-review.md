# Diary: Core values review

The core values in `/docs/core-values.md` were copied from an older, closed-source version of regroup. Markus and the lead went through them one at a time to check they still hold, now that regroup is open source and built by agents. This diary records what was decided and why, and then the work of writing the agreed text into the file and taking visitor analytics out of the software.

## Step 1: Review the values with Markus and agree new wording

**Author:** main

### Prompt Context

**Verbatim prompt:** Let's talk through the core values one by one and check they're still good. I copied them from an older version of this project. If you need, interview me about the project first. One question at a time.
**Interpretation:** Walk through the six values in order, raise one decision at a time, and agree any rewording with Markus before anything is written to the file.
**Inferred intent:** Make the values a sound basis for building regroup from a blank template, so that later feature decisions can be checked against them.

### What I did

I read `/README.md` and `/docs/core-values.md`, then delegated two pieces of research to sub-agents. One studied how Markus works with agents in a sibling project, and reported the pattern I then followed: one small decision at a time, short lettered options with a recommendation, plain language, and no change of wording without his explicit OK. The other surveyed this repository and found it is still the unmodified starter template, with one placeholder page and no groups, posts or moderation. It also found three things that touch the values: a Fathom analytics script on every page (`/html/common.go`, allowed in `/http/csp.go`), tracing to an outside service with recipient email addresses in the email job's log (`/jobs/email.go`), and a marketing sender address configured for email (`/cmd/app/main.go`).

Markus asked me not to learn much about the old repository, so I stopped a third sub-agent that had been sent to read it, before it reported anything.

We then went through the values. The decisions:

- **Kindness.** The bullet that committed to hiding a comment once it had enough flags became a principle: a group can protect itself without waiting for a moderator, and whatever lets it do so must not become a way to silence a member. The pile-on bullet got examples that fit a groups product, namely no downvotes or negative reactions. A retracted comment is folded away and marked, and the group can still open and read it. "Not punitive" now says what it means: moderators can hide content and, as a last resort, remove a member, but nobody carries a lasting public mark.
- **Transparency** became **Trust and transparency**, with an opening sentence that ties the two together. Moderator actions now name who acted. Hard deletion leaves a note that something was removed.
- **Respect for attention.** One bullet added: each person sets their own notification frequency, and the software gives nobody else a way to override it.
- **Community ownership.** "The platform claims no license" could not literally be kept, so it now says what the software will not do with content and what we ask of hosts. Export gained a purpose: a group can leave a host and take its history along.
- **Privacy without surveillance.** The software contains no visitor analytics, no behaviour tracking and no advertising features. Groups are private.
- **Sustainability without exploitation** was dropped. Five values remain.

Three changes of scope came up during the review and shaped the wording throughout:

- **Regroup is now open source under the MIT licence.** Anyone can host it and anyone can change it, so the values can only constrain the software the maintainers publish. Promises about how an operator behaves became requests to hosts. The opening paragraph says this plainly.
- **Only private groups are being built.** The last Privacy bullet says so.
- **A tie-breaker.** Markus said that where one person and the group pull apart he leans towards the group, "but gracefully". That sentence is now in the opening.

Markus chose to leave logs and traces out of the values; what they contain is each host's business. He chose to take analytics out of the software.

I wrote the agreed text, exactly as approved, to a scratch file for the builder to copy into place.

### Why

The values are the yardstick for every feature that follows. Several of them were written for a company running a closed service, and would have been broken or meaningless on the first day of an open-source project that others host.

### What worked

Showing each reworded section in full and asking for one OK per section, instead of asking about wording bullet by bullet, kept the pace up. So did recommending an option every time.

Separating "what the software does" from "how whoever runs it behaves" turned a vague worry (we can't enforce any of this) into a rule that sorted every bullet.

### What didn't work

My first question on moderator transparency offered three options about who sees which part of a moderation notice. Markus answered: "You're making this too complicated. Named moderators moderate, that's okay." I was designing a feature inside a values review. After that I proposed keeping each section unless one bullet had a real problem.

On retraction I first recommended hiding the retracted words. Markus was torn between hiding and showing. The document's own word is "retract", not "delete", and Transparency already said the shared context is not rewritten, so the better answer was a third option I had not offered: folded away but still readable.

My first draft of the new opening said we can't control how others run the software. Markus pointed out that we can't control how they change it either.

### What I learned

The old values mixed three things: principles, feature specifications and business promises. Most of the review was sorting them apart. A values document that says "the feature loses" must not contain sentences that are themselves features.

Dropping the operator from the document made lines in already-approved sections stale. Two bullets in Respect for attention still spoke of regroup as a business, and had to be reworded after the fact. Any later change of framing needs the same sweep over the whole file.

### What was tricky

Kindness and Transparency pull against each other in two places, retraction and visible moderation. Markus's tie-breaker, the group over the individual but gracefully, resolved both.

Export now implies import: if a group can leave a host and take its history along, another host must be able to take it in. That is a feature the values now commit us to.

### What warrants review

`/docs/core-values.md` should match the text Markus approved, word for word. The approved text is in this conversation; the builder's step records how the copy was verified.

### Future work

- Take Fathom analytics out of the software (the next step in this diary).
- The email job logs recipient addresses, and a marketing sender address is configured. Markus left logs and traces to hosts, so neither is a values violation, but both are worth a look when email is built.
- `/AGENTS.md` describes the project with a sentence that is cut off: "This is a web app for".
- Import of an exported group, when export is built.

## Step 2: Write the approved values into the file

**Author:** builder

### Prompt Context

**Verbatim prompt:** (Task 1 of the lead's brief) "The approved text is at: /private/tmp/claude-502/-Users-maragubot-Developer-regroup/1d3a8f6e-3ea6-49ab-8d89-34fdc245fe99/scratchpad/core-values.md

Replace `docs/core-values.md` with that file byte for byte (use `cp`). Do NOT edit, reflow, "improve" or correct the wording in any way: every sentence was approved by Markus individually, including British spellings ("honour", "favours", "behaviour") and the one em dash. Verify with `diff` that the two files are identical. If you cannot read the scratch file, stop and say so in your report; do not reconstruct the text.

Then check that `README.md` still makes sense next to it (it links to the core values). Do not change the README unless something in it is now false; if you think something is, report it instead of editing."
**Interpretation:** Copy the scratch file over `/docs/core-values.md` without touching a character, prove the copy is exact, and read the README for anything the new values make untrue.
**Inferred intent:** The file in the repository must be exactly what Markus approved, so that nobody has to trust a summary of it.

### What I did

I read the scratch file in full, then ran `cp` from it to `/docs/core-values.md`. `diff docs/core-values.md <scratch file>` printed nothing, `cmp` agreed, and both files have the SHA-256 `0bc95ef271f042b829063908fd8a226c6900c921030d9ba28022c8491e38f069`.

I read `/README.md` next to the new values. It has a tagline, a link to the core values, and a consulting and contact line that ends "or perhaps an invoice to support this project?". Nothing in it is false. The dropped Sustainability value does not make the funding line untrue, so I left the README alone.

### Why

A copy that is checked by checksum cannot drift by a typo or a "helpful" correction of the British spelling.

### What worked

`cp` plus `diff` and `cmp`. No editor touched the text.

### What didn't work

Nothing went wrong in this step.

### What I learned

Nothing new; this step was mechanical by design.

### What was tricky

Not touching the README. The invoice line sits close to the dropped "Sustainability without exploitation" value, but asking for an invoice does not conflict with any of the five values that remain.

### What warrants review

Run `shasum -a 256 docs/core-values.md`. It should print `0bc95ef271f042b829063908fd8a226c6900c921030d9ba28022c8491e38f069`, the hash of the text Markus approved.

### Future work

None from this step.

## Step 3: Take Fathom analytics out of the software

**Author:** builder

### Prompt Context

**Verbatim prompt:** (Task 2 of the lead's brief) "Requirements:
- No page served by the app loads the Fathom script or any other third-party analytics.
- The content security policy no longer allows the Fathom origin in any directive. Tighten it only by removing what Fathom needed; do not otherwise redesign the CSP.
- Remove anything that existed only to serve Fathom: config, env vars (check `.env.example`, `cmd/`, Dockerfile, Makefile, workflows), helper code, tests or test expectations. If a dependency in `go.mod` becomes unused, tidy it.
- Do not remove or change OpenTelemetry tracing, logging, or email code. Markus explicitly decided those are out of scope.
- No new telemetry is needed for this change."
**Interpretation:** Delete the Fathom script tag and the Fathom origin from the CSP, leave every other directive exactly as it was, and look for any config or dependency that existed only for Fathom.
**Inferred intent:** Make the code match the new Privacy value ("It contains no visitor analytics"), and add tests so analytics cannot quietly come back.

### What I did

A case-insensitive search for "fathom" found only two places outside `/docs/diary/`. One was the script tag in `html.Page` (`/html/common.go`), which also carried the placeholder site ID `data-site="123"`. The other was `https://cdn.usefathom.com` in the `connect-src`, `img-src` and `script-src` directives in `http.CSP` (`/http/csp.go`). Fathom had no config, env vars, Dockerfile, Makefile or workflow entries, and no Go dependency, so `go mod tidy` changed nothing.

I wrote the tests first. A new subtest in `/html/common_test.go` checks that every `<script>` on the page loads from a path on the app's own origin. The new `/http/csp_test.go` runs `http.CSP` through the real `httph.ContentSecurityPolicy` middleware for all four flag combinations. It checks that every source in every directive is one of `'self'`, `'none'`, `'unsafe-inline'` and `'unsafe-eval'`, and pins the full default policy. Both tests failed on the old code, naming `https://cdn.usefathom.com`.

Then I deleted the script tag, along with the `opts.ConnectSrc` and `opts.ImgSrc` assignments, and changed `script-src` to start at `'self'`. httph v0.3.7 already defaults `connect-src` and `img-src` to `'self'` (`httph.go:196-204` in the module cache), so dropping the two assignments gives exactly the old values minus Fathom. The tests now pin those values, so a change in httph's defaults would show up.

To check in the running app, I started it with `make watch`. The "Starting server" line gave `http://localhost:26774`. `curl` on the home page returned 200 with two script tags, `/scripts/datastar.<hash>.js` and `/scripts/app.<hash>.js`, and this header:

`Content-Security-Policy: default-src 'none'; connect-src 'self'; font-src 'self'; img-src 'self'; manifest-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'`

The `'unsafe-inline'` comes from `CSP_ALLOW_UNSAFE_INLINE=true` in `/.env.example`, the local development setting. The only absolute URL in the HTML is the footer link to `https://www.maragu.dev`, which is a link, not something the page loads. I stopped the app afterwards.

`make test` and `make lint` (`golangci-lint run`, which is what CI's reusable lint workflow runs) both pass.

### Why

The new Privacy value says the software contains no visitor analytics. A test guards that sentence far better than a one-line deletion does.

### What worked

Testing the CSP through the real middleware instead of poking at the options struct. That way the tests see httph's defaults too, and assert on the header a browser actually receives.

After the review fixes, I put the Fathom tag and origins back temporarily (restored from a scratch copy afterwards) and confirmed that both test files fail with messages naming `https://cdn.usefathom.com`.

### What didn't work

The worktree had no `.env`. Without it, `SERVER_ADDRESS_LOCAL` is false and the app would not use this checkout's own port, so I created `/.env` from `/.env.example` (it is gitignored) before `make watch`.

### What I learned

httph's CSP middleware builds a strict default policy and lets the options function overwrite it. A directive the app sets explicitly to the default value is redundant, but deleting it hands control to the library's default, which is why the tests pin the values.

### What was tricky

Self-review (two competing reviewers through the code-review skill) found no problem with the production change, but agreed on four gaps in my first version of the tests, and I fixed all of them:

- `default-src 'none'` was never asserted. With it gone, `frame-src`, `object-src` and the others would allow everything, and the tests would still pass.
- The `style-src` half of `http.CSP` had no assertions.
- The assertions were substring matches that depended on the order httph prints directives in (`"script-src 'self';"` only matched because `script-src` is not last). The tests now parse the header into a map and compare exact values.
- "Every source starts with a quote" let `'strict-dynamic'` and nonces through. That is now an exact list of allowed sources.

Both also noted that the HTML test only looked at script tags with a `src`, so an inline loader snippet, the form most analytics vendors hand out, would pass. The test now also requires every `<script>` to have a `src`, and it rejects the `/\` prefix, which browsers treat as `//`. I left one suggestion from both reviewers alone: also checking `<img>`, `<iframe>` and `<link rel="preconnect">` for other origins. That reaches beyond scripts, and the CSP already blocks the first two.

### What warrants review

- `/http/csp.go`: the diff should only delete Fathom.
- `/http/csp_test.go`: the allowed-source list and the pinned default policy.
- To check by hand, run `make watch`, `curl -sI` the URL from the "Starting server" line in `/app.log`, and compare the `Content-Security-Policy` header with the one above.

### Future work

- The HTML test checks scripts only. If pages ever gain images or iframes from other origins, the CSP will block them, but a page-level test would give a clearer failure.

## Step 4: Carry the s4ql way of working into `AGENTS.md`

**Author:** main

### Prompt Context

**Verbatim prompt:** I'd like to work the same way in this project, so carry over what you need into AGENTS.md
**Interpretation:** Write regroup's agent instructions so that agents here work with Markus as they do in his s4ql project: agents own the code, he and the lead own the spec together, and he stays out of low-level detail.
**Inferred intent:** Make the collaboration repeatable across sessions and agents without Markus having to restate his rules.

### What I did

Just before this prompt Markus had asked whether I had studied how we work in `../s4ql`, and said: "you own the code, but we own the spec together, and no spec changes happen without my involvement. But I'm hands-off on the low-level details and trust you on that." I read s4ql's `AGENTS.md`, `docs/spec.md` and `docs/todo.md` myself, and sent three sub-agents to read its conversation exports (in two halves) and its diaries. They reported how the rules came about, where the lead had asked too much or overstepped, and how briefs to builders changed over time.

From that I chose what to carry over and briefed a writer, who drafted three files:

- `/AGENTS.md`: what regroup is, where documents live, the existing notes on running and access, and a Memory section of working rules. Each rule has its reason and a date, with Markus's own words where they carry the reason.
- `/docs/spec.md`: only the header saying nothing is added or changed without explicit agreement.
- `/.claude/settings.json`: turns the harness's built-in memory off for the project, so the Memory section is the only memory.

I used the memory skill for the section's format and for moving the two notes that the built-in memory held for this project (removing WAL files with the database, and reusing one pull request for upstream merges). A sub-agent confirmed the settings key against the current documentation.

I reviewed the draft and made four small changes: named where decisions are recorded, corrected how and when the values were approved (section by section, on 2026-11-26), gave the upstream-merge rule its reason, and added an entry that the first real group is for Markus's shared office space.

### Why

The rules in s4ql were learned the hard way over two weeks. Starting regroup without them would mean repeating the same friction: asking Markus about details, walls of text, briefs that are too large.

### What worked

Having sub-agents extract only Markus's own messages from the exports, and read around the ones where he corrected the lead, gave the rules with their reasons at a fraction of the size of the transcripts.

### What didn't work

I took "work the same way" as leave to push and merge the finished values branch, and tried to hand that to a sub-agent. The permission system refused: "Permission for this action was denied by the Claude Code auto mode classifier. Reason: [Destructive or Unwanted Actions]." Nothing was pushed. I asked Markus directly instead.

The writer would not read s4ql's files, because its scope kept it inside the worktree and a brief from another agent did not widen it. The quotes it used came from my brief, which I had taken from s4ql's `AGENTS.md` and the sub-agents' reports.

### What I learned

The memory skill asks for shorter entries than s4ql's file has, and for permanent conventions to sit in the body of the file. I kept the rules in the Memory section, as s4ql does, and condensed them.

s4ql's rule that the lead stays on `main` while each builder makes its own worktree conflicts with how this lead is set up, where the lead works in a worktree that a lone builder shares. I left that rule out.

### What was tricky

Several rules rest on things Markus said in another project. The file marks those quotes as carried over, and says once that he asked for it, so a later agent can tell them from what he said here.

The rule on committing and merging without asking grants autonomy that Markus has stated for s4ql but not yet confirmed in so many words for regroup. It is in the draft, and I am asking him before it is committed.

### What warrants review

Read the Memory section of `/AGENTS.md` for anything that does not sound like Markus. The entry "Commit and merge agreed work without waiting for Markus" is the one with consequences.

### Future work

- `docs/todo.md`, `docs/todo/`, `docs/research/` and `docs/conversations/` are named in `/AGENTS.md` and do not exist yet.
- The spec is empty. The first candidates for it came out of the values review: groups are private only, and regroup is software that others host.
- The entries on tests and on upstream merges may belong in Markus's global instructions, since they apply beyond this project.

## Step 5: Go through the rules with Markus, and move most of them to fabrik

**Author:** main

### Prompt Context

**Verbatim prompt:** Let's go from the top for the rules, and send some to fabrik
**Interpretation:** Take the drafted rules one at a time and decide for each whether it stays in regroup's `AGENTS.md`, moves to Markus's fabrik skills, or is dropped.
**Inferred intent:** Keep regroup's file to what is about regroup, and put rules that hold for every project where every project gets them.

### What I did

Markus asked for the rules one at a time. Of the 14 drafted in step 4, five remain in `/AGENTS.md`:

- Agents own the code; Markus owns the product with the lead.
- Changes to the spec and the core values go through the `spec` skill, a skill that does not exist yet and is to be added to fabrik. The entry keeps one sentence saying neither file changes without Markus's agreement on the wording, so the rule holds until the skill exists.
- Commit and merge agreed work without waiting for Markus. He confirmed it for regroup, and chose to keep the mechanics of merging safely here too.
- regroup.social is a marketing site only.
- The first real group is for Markus's shared office space.

Three were dropped: how to talk with Markus, the rule against consulting the old repository (he has deleted it), and reusing one pull request for upstream merges (it guards against a merge pull request sitting open, which the merge rule now prevents).

Six go to fabrik: keeping `AGENTS.md` and the README lean, delegating heavily with small builder tasks, arguing from correctness instead of human effort, tests that skip only under `-short` and never depend on timing, exported conversations in `docs/conversations/`, and removing a SQLite database's WAL and SHM files with it. The new `spec` skill makes seven items for fabrik.

With the merge rule confirmed, a small sub-agent took the values and analytics branch through pull request #2. All five checks passed and it was merged, and the CI, CD and Security runs on `main` passed after it.

### Why

A rule in regroup's file reaches only agents working on regroup. Most of what s4ql taught is about how a lead and its builders work anywhere.

### What worked

Asking "keep here or fabrik" with a recommendation for each rule settled most of them with a single letter.

### What didn't work

The GitHub command line in this checkout resolved to the template repository `maragudk/app`, the `upstream` remote, because no default repository was set. The sub-agent's `gh pr create` failed with "No commits between main and ...", and one `gh pr edit 2 --add-reviewer` ran against pull request #2 of `maragudk/app`, which was merged long ago. I checked that pull request afterwards: it has no review requests and nothing changed. I ran `gh repo set-default maragudk/regroup` so it cannot recur.

### What I learned

Markus's test for where a rule belongs is whether it is about this project. I had drafted 14 rules for regroup and nine of them were not about regroup.

### What was tricky

The entry on the `spec` skill names a skill that does not exist yet. Until fabrik has it, an agent that follows the entry finds no skill, so the entry itself carries the rule in one sentence.

### What warrants review

`/AGENTS.md` should now hold five Memory entries and nothing that is not about regroup.

### Future work

- Take the seven items to fabrik.
- A repository made from the template has an `upstream` remote, and the GitHub command line may pick it. That is worth a line in fabrik's git skill too.
