# AGENTS.md

## Agent skills

### Commits

Commit messages follow Conventional Commits 1.0.0. The type drives the SemVer bump. See `docs/agents/commits.md`.

### Changelog

`CHANGELOG.md` follows Keep a Changelog 1.1.0 + SemVer. Every user-facing change gets an `## [Unreleased]` entry in the same commit. See `docs/agents/changelog.md`.

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

## Where things are

- `cmd/brama/`: the binary's `main`, one line that calls `cli.Main`. Read when you change how the binary starts.
- `cmd/brama-shim/`: the Shim program that runs on a Server and links only the core. Read when you change what runs on a Server.
- `internal/cli/`: every Cobra command, the exit codes, and the renderer choice. Read when you add a command or change flags or output.
- `internal/config/`: reading, checking, and editing `brama.yaml`. Read when you change the config format.
- `internal/anonymize/`: `check`, `review`, approval, and coverage of column classifications. Read when you change how columns are classified.
- `internal/preset/`: the classifications that brama ships for each framework. Read when you add or change a preset.
- `internal/schema/`: database shape types, with `mysql/`, `postgres/`, and the shared `catalog/` loop. Read when you add a database driver.
- `internal/adapter/`: framework knowledge (`wordpress/`): how to find an install and its parts. Read when you add a framework.
- `internal/generator/`: the closed set of fake-value recipes. Read when you add a recipe.
- `internal/shim/`: building and installing the Shim on a Server.
- `internal/renderer/`: human and JSON output of a Result. Read when you change the output.
- `internal/prompt/`: keyboard checklist and single-choice prompts.
- `internal/state/`, `internal/ssh/`, `internal/scaffold/`, `internal/refusal/`, `internal/version/`: Observed state, OpenSSH calls, the starting `brama.yaml`, the Refusal outcome, and version ordering.
- `internal/arch/`: tests that keep the core and the Shim free of CLI and renderer imports. Read when you add a core package or change what the Shim imports.
- `test/testenv/`: end-to-end tests against two containerised Servers, behind the `testenv` build tag. Unit tests sit next to the code as `*_test.go`.
- `docs/`: ADRs in `adr/`, agent rules and Go guides in `agents/`, and `product-description.md`. Read when you need the reason for a decision.
