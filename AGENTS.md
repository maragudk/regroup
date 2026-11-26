# regroup

Regroup is an open-source (MIT) web app for private groups that anyone can host: "Like Groups, but it doesn't Zuck". Markus is the product owner, and the lead is the agent that works with him and directs the other agents.

Where things live:

- `docs/core-values.md`: the maintainers' values, which decide what goes into the software.
- `docs/spec.md`: the rules Markus and the lead have explicitly agreed.
- `docs/todo/`, indexed by `docs/todo.md`: what is agreed but not built, and open design questions. One file per piece of work or area. Delete an item when it is built.
- `docs/research/`: research notes by sub-agents.
- `docs/diary/`: the implementation narrative, written with the diary skill.
- `docs/conversations/`: raw exports of the lead's conversations with Markus.

`docs/todo/`, `docs/todo.md`, `docs/research/`, and `docs/conversations/` do not exist yet. Create each when it is first needed.

## Running

Start the app with `make watch`. It watches for file changes and rebuilds and restarts the app automatically, so it needs no manual restart. Logs are written to `app.log`.

## Access

Open the app in a browser with your `playwright-cli` skill. Each checkout gets its own port, so find the URL in the "Starting server" line in `app.log`.

Query the SQLite database directly with `sqlite3 app.db`. Always run `pragma foreign_keys = 1;` as the first statement after connecting.

## Memory

Agent memory for this project. Use the `memory` skill before adding, changing, or removing entries.

The harness's built-in memory is not used for this project, so anything worth remembering between sessions goes here. Quotes marked (s4ql) are Markus's words from his s4ql project, carried over on 2026-11-26 at his request: "I'd like to work the same way in this project".

### Agents own the code; Markus owns the product with the lead

Markus does not read code ("without my seeing any code", 2026-11-25). On 2026-11-26 he said: "you own the code, but we own the spec together, and no spec changes happen without my involvement. But I'm hands-off on the low-level details and trust you on that." Decide implementation details yourself (names, error messages, layout of code, which library call) and tell him in a line what you chose, as "decided by me, for you to overrule". Never put such details to him as questions. Bring him design, architecture, scope, goals, and anything that deviates from what was agreed. Record each decision in the diary, or in the file of its piece of work in `docs/todo/`, so that it can be changed later.

### Changes to the spec and the core values go through the `spec` skill

Invoke the `spec` skill before changing `docs/spec.md` or `docs/core-values.md`. Neither changes without Markus's explicit agreement on the wording (2026-11-26: "we own the spec together, and no spec changes happen without my involvement").

### Commit and merge agreed work without waiting for Markus

Markus (s4ql, 2026-11-10): "If it's what we agreed you can just commit. Less friction, I don't want to be the bottleneck", and "merge whenever you like. We can always revert. But I'd like to stay involved in high-level design, architecture, goals, etc." He confirmed the rule for regroup on 2026-11-26, and decided the same day that for code pull requests here it takes precedence over the `git` skill's rule that each merge needs an instruction naming the pull request. First check the work against what was agreed. Merge a pull request only after its checks have run and every one has passed. An empty list of checks means "not run yet", never "passed", so count the checks against the jobs CI runs. Put no pipe between a check and the command that depends on it, because a pipe hides the check's exit status. Confirm the pull request is merged before removing its branch or worktree.

### regroup.social is a marketing site only

Markus, 2026-11-26. The site is separate from this repository, and the software itself contains no visitor analytics (see `docs/core-values.md`).

### The first real group is for Markus's shared office space

Markus on 2026-11-26: "I intend to make one for my shared office space, that's it." Use that group as the test case when deciding what to build first.

### Conversation exports go straight to `main`

Commit an export to `docs/conversations/` on `main` and push it, with no branch and no pull request. Markus on 2026-11-26: "Why does it go into a branch? Just push it to main".

### Markus is the oracle for product behaviour; build first, then adjust

On 2026-11-26, asked what regroup's test oracle is, Markus said: "I don't think we can have a test oracle. For product behavior, I guess I'm the oracle? But lean towards building something and then we can adjust. Nothing is in production until I say so, so we can break stuff and iterate." So build a piece, show it to him, and adjust, instead of settling every rule or building heavy test machinery first. He chose the same day to judge each piece of work in two ways: a preview instance he uses himself, and a page of screenshots. What is agreed and what is open about verification is in `docs/todo/verification.md`.
