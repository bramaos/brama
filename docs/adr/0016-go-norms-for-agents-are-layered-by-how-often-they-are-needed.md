# Go norms for agents are layered by how often they are needed

An agent writing Go in this repo needs two kinds of knowledge that cost very different
amounts. The contract — context comes first, errors are wrapped once, interfaces belong to
the consumer, tests say what they got and what they wanted — is needed on nearly every
change, and an agent that has to go looking for it will often not look. The depth behind
that contract — when `%v` is right instead of `%w`, how to document a context that must not
have a deadline, what `testing/synctest` does to time — is needed rarely, and carrying it
into every session spends context on a question the session never asks.

So the norms are written in three layers, each loaded only as often as it is needed:

- **Project rules**, `docs/agents/rules/` — what this repo decided that Go did not decide for
  it. Written as rules as they come up; the directory does not exist yet.
- **Go guidelines**, `docs/agents/go.md` — the contract, loaded in every session.
  `CLAUDE.md` imports it, so it is in context before the agent reads a file. It is held to
  200–250 lines because every line is paid for on every task. Anything an agent needs in
  fewer than about four Go tasks in five belongs a layer down.
- **References**, `docs/agents/go/references/` — the depth, one file per topic, read when a
  task reaches it. They are our prose, not the upstream text, and every claim cites the
  section of the Google Go Style Guide, Effective Go, Go Code Review Comments or the Go
  release notes it came from, so that a reader can check it.

Two things already carry norms without being documents, and they outrank all three layers.
The precedence chain, most authoritative first:

1. repo config — `.golangci.yml`, `Makefile`, CI
2. existing code — consistency first
3. project rules — `docs/agents/rules/`
4. Go guidelines — `docs/agents/go.md`
5. references — `docs/agents/go/references/`

Config is first because it is the only layer that fails a build. A guideline that
contradicts the linter is wrong, not a rival reading of it. Existing code is second because
code that looks like the code around it is easier to read than code that follows a better
rule in isolation, and the guidelines are general where the code is particular. A later
layer yields to an earlier one. Where an earlier layer is silent, a later one speaks.

Skills are not in the chain. A skill is a procedure — how to triage an issue, how to run a
review — and it does not decide what good Go looks like. A skill that states a Go norm
either repeats `go.md` or contradicts it, and both are defects in the skill.

Project rules are kept apart from ADRs even though both record what this repo decided. An
ADR records why, at the time it was decided. It is read when working in its area. It is not
rewritten when it stops being true: a later ADR supersedes it, so the history stays readable.
A rule is the current instruction, short enough to apply mid-edit. It is rewritten in place
when the instruction changes. Putting rules in ADRs would make an agent read the history to
learn the present. Putting reasons in rules would make each rule too long to apply. A rule
can cite the ADR that justifies it, and that is the only link between them.

The alternatives were each worse for one of the two costs:

- **A single Go skill.** Skills load when a model judges them relevant, and a norm that
  applies to every edit cannot depend on that judgement. It would also put norms inside a
  procedure, which is the mixing the chain keeps out.
- **`.ai/rules/`.** A second home for agent documents beside `docs/agents/`, which
  `AGENTS.md` already points into, and a hidden directory that the people who review agent
  output are least likely to open. No tool this repo uses reads it natively, so it would
  need the same wiring `docs/agents/` already has.
- **Vendoring the Google documents verbatim.** Thousands of lines that could never be
  always-loaded, organised by upstream document rather than by the question an agent has.
  They also carry Google-internal guidance — `log.Fatal` from their logging package, Bazel
  targets, `cmp` in every test — that does not fit this repo and would read as instruction.
  A verbatim copy also goes stale without anyone noticing. A cited paraphrase goes stale at
  a link a reader can follow and check.
- **A `go-style` skill that exists only to hold files.** A directory with a trigger. It
  gives up always-loading, like the Go skill, and adds nothing to on-demand reading that a
  plain directory does not already give.

## Consequences

- `go.md` is always loaded only for agents that follow `CLAUDE.md`'s import. An agent that
  reads `AGENTS.md` alone gets a pointer to it and has to follow the pointer itself.
  `AGENTS.md` has no import syntax, so this cannot be closed from inside the repo.
- Growing `go.md` is a cost paid on every session. A new rule goes there only if it is
  needed on most Go tasks. Otherwise it goes into a reference, and `go.md` links to it.
- Every rule in `go.md` that the linter does not already enforce carries a one-line
  bad→good example, because the linter will not catch the mistake. A rule the linter
  enforces is named, not illustrated.
- Where `go.md` and the code around a change disagree, the code wins and `go.md` is the
  thing to fix. A guideline that the codebase does not follow is not a norm. It is a wish.
- References cite sections by anchor, and anchors rot when upstream pages are reorganised.
  A broken anchor is a defect in the reference, and is fixed by re-reading the source rather
  than by deleting the citation.
