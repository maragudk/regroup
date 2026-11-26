# Bombadil

Research note, 2026-11-26. Bombadil version evaluated: 0.8.0.

## What was asked and why

Markus wants regroup to be robust, and asked for property-based testing with hegel in the Go code, the usual unit, integration, and fuzz tests, and an evaluation of Bombadil, whose command-line tool he has installed. This note covers Bombadil only. It answers four questions: what Bombadil is, what happened when it was run against regroup, whether it fits regroup, and what regroup would need to provide for it.

Sources: the repository at https://github.com/antithesishq/bombadil (README, CHANGELOG, the manual in `docs/manual/src/`, the default specification in `lib/bombadil/src/specification/browser/defaults/`, and `examples/`), the GitHub Action at https://github.com/antithesishq/bombadil-action, the issue tracker, and the installed tool's help output.

## What Bombadil is

Bombadil is a property-based testing tool for web interfaces (and, experimentally, terminal interfaces), made by Antithesis and released under the MIT licence. It drives a real Chrome or Chromium browser through the DevTools protocol, picks actions at random (clicks, typing, scrolling, back, forward, reload, waiting), and checks a set of properties after every state it captures. It does not read regroup's code; it only sees the page, the browser console, uncaught exceptions, navigation history, and page metrics.

How it explores. In a loop, it captures the browser state, checks every property against it, asks every action generator for the actions possible in that state, picks one by weight, performs it, and waits for the next event (a navigation, a DOM change, or a timeout). Exploration stays on the starting URL's scheme, host, and port unless `--allow-url` widens it.

How properties are written. A specification is a TypeScript or JavaScript module that exports properties and action generators. State is read with `extract(state => ...)`, which runs in the browser and returns JSON. Properties are formulas in a form of linear temporal logic: `always`, `eventually(...).within(5, "seconds")`, `next`, `now`, combined with `and`, `or`, `implies`, and `not`. Action generators return the actions that are valid now, can be weighted, filtered, and mapped, and `registerCustomAction` runs arbitrary code in the page, which the manual suggests for getting past sign-in forms. The `@antithesishq/bombadil` package on npm supplies the types; the tool itself resolved the imports without it.

Built-in properties (the browser defaults):

- `noHttpErrorCodes`: the page's navigation response status is below 400. It reads the navigation timing entry, so it covers full page loads only, not `fetch` requests such as Datastar's.
- `noUncaughtExceptions` and `noUnhandledPromiseRejections`.
- `noConsoleErrors`: nothing is logged at error level.
- An optional `noResourceLeak` property for heap, DOM node, and event-listener growth.

Built-in actions: clicks on links, buttons, inputs, labels, and ARIA-clickable elements; typing into the focused input (text, email, number, textarea) and pressing Backspace, Tab, Enter, and Escape; scrolling; back, forward, and reload; and waiting. Shift-modified keys and right-clicks are not available (issue 251), and file inputs are skipped.

What it reports. Each violation is printed with the property's source, the time, and the extracted values that broke it. A run writes a compressed binary trace with a screenshot of every state. `bombadil browser inspect` opens a web viewer of the trace, `bombadil trace` lists states in the terminal (with `--json` and `--screenshots`), `--reproduce` replays a trace (best effort, it fails on divergence), and the experimental `shrink` command searches for a shorter trace that still fails. At the end of a run it prints the exact inspect and reproduce commands. The exit code is 0 with no violation, 2 with a violation, and 1 on an error.

Headless and CI. `--headless` runs without a window, `--time-limit` bounds the run, and `--exit-on-violation` stops at the first failure. The action `antithesishq/bombadil-action@v3` installs Chrome for Testing on a GitHub runner, runs the test, fails the job on a violation, and documents the pattern of starting a local server first. `--cookie` and `--header` set cookies and headers before the test, which is how it would be signed in. There is also a Docker image, and it runs inside Antithesis's own platform.

Maturity. Version 0.8.0 was released on 2026-11-19. The repository was created on 2025-12-17 and has had about 20 releases since April 2026, roughly every one to four weeks, with breaking changes between minor versions (0.8.0 changed the trace format and the meaning of `--output-path`). The README of the GitHub Action still says v3 runs 0.7.x. It had 13 open issues; the ones relevant here are the missing modifier keys (251), worker code not being instrumented (354), and the Chrome dependency not being documented (93). Shrinking, the `fuzz` command, and the faster `chrome-headless-shell` variant are marked experimental.

## What I ran and what happened

The installed tool is `/Users/maragubot/.local/bin/bombadil`, and `bombadil --version` printed `bombadil 0.8.0`. It used the Google Chrome installed in `/Applications`. All trace files are in the scratch directory `/private/tmp/claude-502/-Users-maragubot-Developer-regroup/2aaa5f42-49d5-4f4d-b314-004354df9083/scratchpad/bombadil-try/`, which does not persist.

Starting regroup. I copied `.env.example` to `.env` and ran `make watch > app.log 2>&1`. It started on the first try, and `app.log` showed `Starting server ... address=http://localhost:22711`. The app serves one placeholder page whose only link goes to another site.

Run 1, the default properties, with the configuration `.env.example` ships:

```
bombadil browser test --headless --time-limit 60s --output-path default.trace http://localhost:22711/
```

It found two violations on the very first state, in about one second:

- `noUncaughtExceptions`: `Uncaught Error: GenerateExpression` from `datastar.js`.
- `noConsoleErrors`: `EvalError: Evaluating a string as JavaScript violates the following Content Security Policy directive because 'unsafe-eval' is not an allowed source of script`.

That is a real fault: with `CSP_ALLOW_UNSAFE_EVAL=false`, as `.env.example` and `cmd/app/main.go` default to, Datastar cannot evaluate any expression, so the page's only Datastar feature does not run. It is already known (`docs/diary/2026-09-02-datastar-1.0.3-upgrade.md` records it and the fix, Datastar's nonce mode), but no Go test catches it, and Bombadil found it with no configuration at all. It then reloaded, found nothing else to do, and stopped with `Error: no actions available` and exit code 1, because the page has no link or control on its own origin.

Run 2, the default properties with `CSP_ALLOW_UNSAFE_EVAL=true` in `.env`:

```
bombadil browser test --headless --time-limit 30s --output-path default-eval.trace http://localhost:22711/
```

No violation; it again stopped with `no actions available` (exit 1).

Run 3, a custom specification, `spec.ts`:

```typescript
import { always } from "@antithesishq/bombadil";
import { actions, extract, registerCustomAction } from "@antithesishq/bombadil/browser";
export * from "@antithesishq/bombadil/browser/defaults";

// Every page regroup serves should have a non-empty <title>.
const title = extract((state) => state.document.title.trim());
export const pageHasTitle = always(() => title.current !== "");

// Every page should have exactly one h1.
const h1Count = extract((state) => state.document.querySelectorAll("h1").length);
export const exactlyOneH1 = always(() => h1Count.current === 1);

// The starter page has no same-origin links, so give Bombadil somewhere to go.
const visit = registerCustomAction("visit", async (_document, window, path: string) => {
  window.location.assign(path);
});
export const visits = actions(() => [visit("/"), visit("/does-not-exist"), visit("/?q=%3Cscript%3E")]);
```

```
bombadil browser test --headless --time-limit 60s --output-path custom.trace http://localhost:22711/ spec.ts
```

It ran for the full minute, captured 1,214 states (about 20 a second), and exited with code 2. `pageHasTitle` failed on the home page, whose `<title>` is empty, a small real fault that a Go test could also catch once someone thinks to write it. `noHttpErrorCodes` failed on `/does-not-exist`, which was expected, since I sent it there; a real specification would allow a 404 on a deliberate visit. `exactlyOneH1` held, including on the 404 page. Without `--exit-on-violation`, every state that breaks a property is reported again, so the console output ran to over 200 KB; the trace is the better thing to read (`bombadil trace custom.trace`).

Run 4, the same specification stopping at the first failure, then a replay:

```
bombadil browser test --headless --exit-on-violation --time-limit 30s --output-path custom-exit.trace http://localhost:22711/ spec.ts
bombadil trace custom-exit.trace
bombadil browser test 'http://localhost:22711/' spec.ts --headless --reproduce custom-exit.trace
```

The first printed `Test finished, finding 1 violation!` with the inspect and reproduce commands, and exited 2. `bombadil trace` showed one state with `violated: "pageHasTitle"`. The replay printed `Reproduction finished, finding 1 violation!`. I did not try `inspect` (it opens a browser window) or `shrink` (the failing trace was already one state long).

Afterwards I stopped `make watch`, deleted the `.env` I had created, and confirmed with `git status` that no tracked file had changed.

## Fit for regroup

Server-rendered pages with Datastar. Bombadil tests anything in the DOM, and regroup's full page loads, links, and forms are what its default actions are best at. The default properties will catch Datastar expression errors, Content Security Policy violations, and server errors on full page loads, as run 1 showed. They will not see a Datastar request that the server answers with an error, because `noHttpErrorCodes` only looks at page navigations. Covering that needs either a custom property that reads something the page shows on failure, or a check of the server's own log after the run.

Sign-in by an emailed link. Bombadil cannot read email. Three ways in, from simplest:

1. A seed step that creates a user and a sign-in token with a known value in a test database, and a start URL that is the sign-in link itself. Tokens are single-use, so this suits one run per token.
2. A test-only route, enabled only by an environment variable, that signs in a named seeded user and redirects to the group page. A custom action or the start URL calls it. The route must be impossible to enable in production.
3. A session cookie passed with `--cookie`. regroup's sessions are held by scs in the `sessions` table with gob-encoded data, and the cookie name is derived from the base URL, so a seed program written in Go that uses the app's own session code would have to create it; the first two are easier.

Bombadil must also be kept away from signing itself out, for example by filtering the `clicks` generator so it never clicks the sign-out button, or by a custom action that signs back in.

Several people with different roles. One run drives one browser as one person. Roles are covered by separate runs, one per seeded admin, moderator, and member, each with role-specific properties (a member never sees a moderation control, say). Bombadil cannot by itself check what one person's action means for another, such as a hidden post disappearing from every member's timeline; that needs Go integration tests, or a custom action that calls a test endpoint, which goes beyond what the tool is designed for.

What it can find that Go tests would not: JavaScript and Datastar errors in a real browser, Content Security Policy breakage, broken links and server errors reached by paths nobody wrote a test for, sequences such as back, reload, and double submission that break a form, pages that stop working after an unexpected order of actions, and memory or DOM growth in long sessions. Temporal properties fit regroup's product rules well, for example "nothing disappears silently": once a comment has been shown, it is never later absent from the page without a retraction or moderation note in its place.

What it cannot do: check correctness against the database or the server's internal state, judge whether a page is right for a person (Markus remains the oracle for product behaviour), read email or test digests, drive two people at once, test keyboard shortcuts that need modifier keys, or give deterministic, fast feedback like `go test`. Its random runs find different things each time, and replays can diverge.

What regroup would need to provide:

- A seeded database for test runs: a group with posts, comments, and one user per role, recreated before each run.
- A test-only way to be signed in, as above, that cannot be enabled in production.
- Stable element hooks for the things properties talk about, such as `data-testid` attributes or stable IDs on posts, comments, retraction notes, and moderation notes, so that properties do not depend on Tailwind classes or wording.
- A visible, machine-readable error state for failed Datastar requests, so a property can notice them.
- The Datastar Content Security Policy fix (nonce mode), or the tool will report the same violation on every page.
- In CI, a job that builds and starts the app on a local port, waits for it, runs Bombadil with a time limit, and uploads the trace as an artifact.

## Recommendation

Adopt Bombadil as a supplementary, browser-level check, not as a replacement for Go tests and not yet as a merge gate. Concretely:

1. Now, at almost no cost: a short run with the default properties against the locally started app, as part of verifying a piece of work, much as agents already open the app with playwright-cli. Run 1 shows that this alone catches a real class of fault.
2. When sign-in and the group page exist: add the seed data, the test-only sign-in, and stable hooks, and write a specification per role with regroup's product rules as properties. Run it in CI on `main` or nightly with a time limit of a few minutes, uploading traces, and make it a required check only once it has run without false alarms for a while.

Limits of this recommendation. The trial ran against a placeholder page with almost nothing to explore, so it says nothing yet about how well Bombadil explores a real timeline with forms and Datastar updates, how flaky it is there, or how long useful runs must be. The tool is young, releases often, and breaks compatibility between minor versions, so the version should be pinned. I did not run it in GitHub Actions, try `inspect`, `shrink`, `fuzz`, or `chrome-headless-shell`, or confirm how Datastar reports a failed request in the browser console.
