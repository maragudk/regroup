# Hegel for property-based testing in regroup

Research date: 2026-11-26. Machine: `go1.27.4 darwin/arm64`, `uv 0.13.9`, system Python 3. Written for the lead; it changes nothing in the code.

## 1. What Hegel for Go is and what it needs

- **Module and install.** `hegel.dev/go/hegel`, installed with `go get hegel.dev/go/hegel@latest`. The latest version on the Go module proxy is v0.10.3, released 2026-11-24 (`proxy.golang.org/hegel.dev/go/hegel/@latest`). The source lives in the monorepo `github.com/hegeldev/hegel` under `frontends/go`; the old `hegeldev/hegel-go` repository is archived.
- **Licence.** MIT (GitHub licence API for `hegeldev/hegel`), the same as regroup.
- **What runs at test time.** The Go library calls a native engine, libhegel, written in Rust, through `github.com/ebitengine/purego`, which loads a shared library without cgo. Tagged releases embed prebuilt libraries for darwin/arm64, linux/amd64, and linux/arm64 (`internal/libhegel/embed_*.go` in the module). On the first run the library is extracted to a per-user cache; on this machine that is `~/Library/Caches/hegel-go/libhegel`. Nothing else is needed: no server process, no Python, no uv, and no network beyond downloading the module once. Older Hegel versions ran a Python server through uv, and the installed skill text still hints at that; the current Go library does not.
- **Platforms.** Linux amd64 and arm64, macOS arm64, and Windows amd64 and arm64 (README). No Intel Mac. Both of regroup's targets, Apple Silicon for development and `ubuntu-latest` for CI, are covered.
- **Dependencies it adds.** `go get` added two requirements, `hegel.dev/go/hegel` and `github.com/ebitengine/purego`, which is pinned to an alpha pseudo-version (`v0.12.0-alpha.1.0.20261015130536-d456b1bb95e8`). The module graph (`go list -m all`) also lists Hegel's own tools, such as staticcheck, gomutants, and lib/pq, but they are not built into regroup. Because Hegel would be imported only from `_test.go` files, nothing of it reaches the production binary or the Docker image.
- **How it fits `go test`.** A property is an ordinary `func TestX(t *testing.T)` that calls `hegel.Test(t, func(ht *hegel.T) {...}, options...)`. Inside, `hegel.Draw(ht, generator)` draws a value, and `ht.Fatalf` fails the case. `go test -run`, `-race`, `-shuffle on`, and `-short` all work; `-short` lowers the number of cases. The default is 100 cases per property, changed with `hegel.WithTestCases(n)`.
- **Generators.** Integers, floats, booleans, text, binary, `SampledFrom`, `Lists`, `UniqueLists`, `Maps`, `Optional`, `OneOf`, `Recursive`, `FromRegex`, `Emails`, `URLs`, `Domains`, `UUIDs`, `Default[T]()` for a Go type, and the combinators `Map`, `Filter`, `FlatMap`, and `Composite`. Since v0.10.0 the package `hegel.dev/go/hegel/temporal` has `Dates`, `Datetimes`, `Durations`, `Offsets`, `Zones`, and `ZonedDatetimes`. `ZonedDatetimes` aims about one draw in twenty at a daylight-saving or offset change, and never produces a wall-clock time that the clocks skipped. That is directly useful for grouping posts by day.
- **Shrinking.** The engine is Hypothesis-style and shrinks the underlying sequence of choices. A failing test prints every draw with its source line, the failure message, and a base64 "reproduction blob".
- **Replay.** Locally, failing examples are saved in an example database and replayed first on the next run. By default that database is a `.hegel/` directory in the package being tested (observed in the trial), so it would need a line in `.gitignore`. `hegel.WithDatabase("")` turns it off. In CI it turns itself off. I found no public option in v0.10.3 to feed a reproduction blob back in; the blob is used internally to replay the failure once at the end of the run. So a failure found in CI is reproduced from its printed draws, best kept as a plain example test, or with `hegel.WithSeed`.
- **Stateful, model-based testing.** Supported. `hegel.RunStateful(tc, &machine)` treats methods named `Rule*` with the signature `func(hegel.TestCase)` as actions and methods named `Invariant*` as checks run between them. A rule skips itself with `tc.Assume(false)`; there is no separate precondition API. `hegel.NewPool` shares generated values, such as created accounts, between rules. `WithStatefulStepCount` sets the number of rounds (default 50). `WithConcurrency` and `WithBoundedConcurrency` run rules on concurrent goroutines, which finds races but does not replay goroutine interleavings exactly. Note that the `hegel.TestCase` passed to rules has `Errorf` and `FailNow` but no `Fatalf`; my first trial failed to compile on that.
- **Other entry points.** `hegel.Run` and `hegel.MustRun` run a property outside `go test`. `hegel.Workload` builds a standalone binary for long soak runs or for Antithesis.
- **Maturity.** Beta. The project says that `0.N.0` releases may break the API ("SemVer shifted right by one decimal place") and expects 1.0 "in a few months". Releases come several times a week; v0.10.0 (2026-11-19) moved the date generators to a new package and deprecated the old ones. Pin an exact version and expect small edits on upgrades.

## 2. Trial on this machine

A new module in `/private/tmp/claude-502/-Users-maragubot-Developer-regroup/2aaa5f42-49d5-4f4d-b314-004354df9083/scratchpad/hegel-try/`, with three properties: a passing one (sorting a list twice gives the same result as sorting it once), a deliberately false one ("no list of integers from 0 to 1000 sums to more than 100"), and a small state machine comparing a Go map with a list model, once correct and once with a planted bug (removing 7 from the map is forgotten).

```
$ go mod init example.com/hegeltry
$ go get hegel.dev/go/hegel@v0.10.3
go: downloading hegel.dev/go/hegel v0.10.3
go: added github.com/ebitengine/purego v0.12.0-alpha.1.0.20261015130536-d456b1bb95e8
go: added golang.org/x/sys v0.48.0
go: added hegel.dev/go/hegel v0.10.3
$ go mod tidy && go test -count=1 -v ./...
=== RUN   TestSortIdempotent
--- PASS: TestSortIdempotent (0.19s)
=== RUN   TestShrinkDemo
    xs := hegel.Draw(ht, hegel.Lists(hegel.Integers(0, 1000))) = []int{101}
    failure: sum 101 > 100 for [101]
    ...
    reproduction blob: AAMAAAABAQAKAQAAAGUBAA==
--- FAIL: TestShrinkDemo (0.01s)
```

The false property shrank to the smallest counterexample, `[]int{101}`. Further runs:

- `go test -race -run TestSortIdempotent` passed (1.9 s, mostly the race build).
- `go test -short` passed in 0.01 s.
- `CI=true go test -run TestShrinkDemo` found the same counterexample.
- The correct state machine passed 300 cases in 0.8 s. The buggy one failed in 2.3 s with a readable log of rounds (`Rule: RuleAdd`, the drawn value, ...) ending in `failure: real 5, model 4`. The shrunk history had six steps (five adds, including 7, then removing 7), not the minimal two. The shrinker did not remove the unrelated adds, probably because the drawn index of the removed element depends on the list's length. The report was still clear enough to see the bug.
- After the runs, `.hegel/examples/` existed in the module directory.
- Hegel with `mattn/go-sqlite3` (cgo, as regroup uses) in one test binary: a property that round-trips 500 generated strings through `select ?` on an in-memory SQLite database passed under `go test -race` (0.09 s for the test; the first build took about 80 s to compile SQLite).

Everything ran on the first attempt apart from the `Fatalf` compile error in the state machine, which was mine.

## 3. What s4ql concluded

`/Users/maragubot/Developer/s4ql/docs/research/testing-strategies.md` (2026-11-10) did not reject Hegel. It described it, with the same facts as above for v0.9.31, and recommended it: Hegel generators for SQL so that a mismatch against PostgreSQL shrinks, Hegel state machines with concurrency for transactions, and `hegel.Workload` for soak runs and Antithesis. Its only reservation was the FFI engine, with `pgregory.net/rapid` (pure Go, MPL-2.0) named as the fallback "if FFI is unacceptable".

s4ql has not adopted it in practice, and the reason is recorded elsewhere. A builder's report in `docs/conversations/2026-11-13-sql-front-end-and-parser.txt` says: "No property-based library was added. The repo has none; mutations and fuzzing cover generation for now." The diary entry `docs/diary/2026-11-16-parser-corpus-and-oracle.md` repeats that the repository has no Hegel, rapid, or `testing/quick`. So s4ql's reason was sequencing, not a fault in Hegel: it had a real oracle (SQLite, and a corpus), and Go's fuzzing plus mutation testing were enough for the parser it was building.

For regroup: the FFI concern is the same, and in the trial it caused no trouble. The sequencing reason does not carry over. Regroup has no outside oracle (`docs/todo/verification.md`), and its risks are in rules and sequences of actions (who may do what, a link that works once, marks of what is new), not in parsing bytes. Those are where property-based and model-based tests do better than fuzzing. The lead's own open proposal in `docs/todo/verification.md`, "a small model of a group checked against generated histories of made-up people", is a Hegel state machine.

## 4. What would change in CI and `make test`

CI calls `maragudk/workflows/.github/workflows/test.yml@main`, which runs on `ubuntu-latest` (linux/amd64), checks that `go.mod` and `go.sum` are tidy, builds, and runs `go test -race -shuffle on -tags sqlite_fts5,sqlite_math_functions,sqlite_foreign_keys ./...`. The Makefile's `test` target runs `go test -tags sqlite_fts5,sqlite_math_functions -coverprofile cover.out -shuffle on ./...` after starting versitygw.

- **Nothing in the workflows or the Makefile has to change** for Hegel tests to run. They are ordinary Go tests, the linux/amd64 library is embedded, and the runner can write to its cache directory.
- **`go.mod` and `go.sum`** gain the Hegel and purego entries. Commit them tidy, or the tidy check fails.
- **`.gitignore`** needs `.hegel/` (unanchored, since a directory appears in each tested package).
- **Case counts.** CI disables the example database, so each CI run explores fresh cases. Keep the default count of 100 for cheap properties, raise it on the ones with rich inputs, and keep the suite fast. A nightly job with high counts, or `hegel.Workload`, can come later and would need a new workflow.
- **Race detector.** CI already uses `-race`, and Hegel works with it. `make test` does not use `-race`; that is unrelated to Hegel.
- **Fuzz targets**, if added, run their seed corpus in plain `go test`; long fuzzing needs `go test -fuzz` per target, which neither CI nor the Makefile does today.

## 5. Where property-based tests fit regroup

The planned features, from `docs/todo/people.md` and `docs/todo/group-page.md`: sign-in by emailed single-use link, three roles (admin, moderator, member), invitation by moderators and admins, one group per instance, and a timeline of posts and flat comments that stays in time order, with a per-reader mark on what is new. The app is still close to its template (`model/auth.go` has only an admin role and permissions from `maragu.dev/glue`), so the properties below are for the code as it is built.

### Simple properties (one generated input, one check)

1. **Who may do what.** For every role, every action, and every target role, the permission function agrees with a small table written from `docs/todo/people.md`: moderators and admins may invite, members may not; only admins change roles; nobody changes their own role. The generator is just `SampledFrom` over roles and actions, so this is close to an exhaustive table test. Hegel adds little over a loop here; write it as a table test unless the inputs grow (for example inactive accounts, removed members, or the configured first admin).
2. **Sign-in tokens.** A generated token is never empty, has the expected length and alphabet, and two tokens drawn in one test never collide. Looking up a token that was never issued, a token of any generated text, or a token with one character changed finds nothing.
3. **Email addresses.** Normalising an address (case, surrounding spaces) is idempotent, and two spellings that normalise the same reach the same account. `hegel.Emails()` gives realistic addresses; add hostile text from `hegel.Text()`.
4. **Grouping by day across time zones.** For posts at times from `temporal.ZonedDatetimes()` and a reader's zone from `temporal.Zones()`: every post falls in exactly one day group; the groups are in date order and posts within a group in time order; a post's day is the day the reader's clock showed when it was posted, including just before and after a daylight-saving change and in zones with offsets such as +05:45 or +14:00; and grouping never loses or duplicates a post. This is the clearest win, because the bugs live in rare instants that example tests miss.
5. **Rendering text safely.** Any generated post or comment body, rendered with gomponents, appears escaped in the HTML: no generated `<script>` or attribute breaks out, and the text read back from the page equals the input. Line breaks and Unicode survive.
6. **Pagination of the timeline, when it exists.** Walking all pages from a cursor yields every post exactly once, in order, for any page size and any set of posts, including posts with equal timestamps.

### Stateful, model-based properties (generated sequences of actions)

These test the real service and SQLite database against a plain Go model of a group. Each rule performs one action through the service layer and updates the model; invariants compare the two.

7. **People and roles.** Rules: invite an address (by someone of a drawn role), accept an invitation, request a sign-in link (known or unknown address), use a link, change a role, advance the clock. Invariants: there is always at least one admin; nobody's role was changed by themselves or by a non-admin; everyone in the group came in by an invitation from a moderator or an admin, or is the configured first admin; a request with an unknown address sent no email and returned the same response as a known one.
8. **Links work once and for a short time.** In the same machine: a sign-in link succeeds at most once, fails after it expires, and an older link may or may not still work depending on what is decided, but the model and the app always agree. An invitation link outlives a sign-in link. The clock must be injected for this to be testable; `docs/todo/verification.md` already lists that as open.
9. **Timeline order and the "new" marks.** Rules: a member posts, a member comments on a drawn post, a member opens the timeline or a post, the clock advances. Invariants: the timeline is in posting order and a comment never moves its post; every post and comment appears once; a post is marked as having new comments for a reader exactly when it has a comment by someone else made after the reader last saw it; a reader's own comments never mark anything new for them; opening a post clears its mark for that reader only, and no reader's action changes another reader's marks.
10. **Concurrency (later).** The same machines with `WithBoundedConcurrency`, under `-race`, look for lost updates in marks and double use of a link when two requests arrive together. Treat these as bug finders: failures may not replay exactly.

### Better served by other kinds of test

- **Plain example tests:** each rule Markus approves, written as a named test (`docs/todo/verification.md` asks for this), the wording and content of emails, migrations, configuration loading, and every failure that Hegel finds, kept as a fixed regression test because CI does not keep Hegel's database.
- **Integration tests against SQLite:** queries and constraints (foreign keys, uniqueness of addresses and tokens) through the existing `sqlitetest` helpers. The stateful machines above are integration tests too and should use the same helpers.
- **Go's native fuzzing:** code that takes raw bytes or strings from outside and must never panic: parsing form values and query strings, a cursor or token decoder if tokens carry structure, and any Markdown or link detection added to post bodies. Fuzzing suits "never crashes" and round-trips on bytes; it is weak where the input must be a valid sequence of actions.
- **Browser-level tests (Bombadil, Playwright):** that the pages work in a real browser, that the "new" marks show and clear on screen, the layout on a phone, Datastar interactions, keyboard use, and the core-values check that no page asks a third party for anything. Hegel cannot see what the browser renders.
- **Not worth a property:** the role table if it stays at three roles and a handful of actions (a table test covers it exhaustively), HTTP routing, and the CSP header, which already has `http/csp_test.go`.

## What I could not verify

- How well Hegel shrinks larger stateful histories: in the trial it left four unneeded steps in a six-step failure.
- Whether a reproduction blob can be replayed through a public API; I found none in v0.10.3.
- Running on the CI runner itself: I checked only that a linux/amd64 library is embedded, not a run on `ubuntu-latest`.
- Hegel inside regroup's own module, with its build tags and `go mod tidy` check; the trial used a separate module, as asked.
