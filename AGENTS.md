# AGENTS.md

## Agent skills

### Issue tracker

Issues live in GitHub Issues at `bramaos/brama`, managed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default vocabulary: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Commits

Commit messages follow Conventional Commits 1.0.0. The type drives the SemVer bump. See `docs/agents/commits.md`.

### Changelog

`CHANGELOG.md` follows Keep a Changelog 1.1.0 + SemVer. Every user-facing change gets an `## [Unreleased]` entry in the same commit. See `docs/agents/changelog.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Agent architecture

Norms for agents are layered by how often they are needed: project rules and Go
guidelines loaded in every session, and Go references read on demand. Repo config and
existing code outrank all three. Skills are procedures and sit outside the chain. See
`docs/adr/0016-go-norms-for-agents-are-layered-by-how-often-they-are-needed.md`.

### Project rules

brama's own decisions are in `docs/agents/rules/`: architecture, commands, config, output,
testing. Each rule is one line citing where its reason lives. They outrank the Go contract.

### Go code

The Go contract is `docs/agents/go.md`. `CLAUDE.md` imports it and the rules. If your
harness doesn't follow `@` imports, read both before writing Go. Topic depth (context,
errors, concurrency, interfaces, packages, naming, doc comments, testing, modern Go) lives
in `docs/agents/go/references/`.

To add a command or change one's flags, arguments or output, follow the `go-cli` skill,
`.claude/skills/go-cli/SKILL.md`: the procedure from file and registration to the
rendered Result and `make check`.
