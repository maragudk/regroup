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
