---
name: go-cli
description: Procedure for a brama CLI command end to end, from file and registration to the rendered Result. Use when adding a command or subcommand to internal/cli, or changing an existing command's flags, arguments or output.
---

# Adding a brama command

A procedure. The norms it applies live in the rules that are already loaded, and each step
names the one it needs: `docs/agents/rules/commands.md`, `docs/agents/rules/output.md`,
`docs/agents/go.md`. Where this skill and a rule disagree, the rule wins and this skill is
the thing to fix.

Model the new command on the nearest one already in `internal/cli`:

- `version.go`: renders a constant, no `run<Command>`.
- `init.go`: reads the working directory and flags, never blocks.
- `server_add.go`: a subcommand with a positional argument, a context, a fakeable
  dependency and a Server to reach.

Changing a command rather than adding one: start at the step that owns what changes, then
do steps 7 to 10.

## 1. Name it

Choose the command path, `brama <group> <verb>`, in `CONTEXT.md`'s terms. Everything else is
named from it. For `brama server add`:

| Piece       | Name                                  |
| ----------- | ------------------------------------- |
| file        | `internal/cli/server_add.go`          |
| constructor | `newServerAddCmd`                     |
| run         | `runServerAdd`                        |
| Result type | `ServerAddResult`                     |
| JSON action | `server_add`, what `Action()` returns |
| test file   | `internal/cli/server_add_test.go`     |

Done when all six names are written down and none collides with an existing one in
`internal/cli`.

## 2. Place and register it

Create the file in `internal/cli`, `package cli`. The constructor takes `env *console`, plus
any build-time value the command needs (`version`), and returns `*cobra.Command`.

Register it where its parent is built:

- a top-level command: the `root.AddCommand(...)` call in `Main`, `internal/cli/cli.go`;
- a subcommand: its group's constructor, `newAnonymizeCmd` or `newServerCmd`;
- a new group: a constructor with no `RunE` that only calls `cmd.AddCommand`, like
  `anonymize.go`, registered on root.

Nothing in `cmd/brama/main.go` changes (`rules/architecture.md`).

Done when `go build ./...` passes and `go run ./cmd/brama <group> --help` lists the command.

## 3. Declare arguments and flags

Follow `references/cobra.md` for the fields every command sets, `Args`, and how flags are
declared and bound. `--json` and `--non-interactive` already exist on root and reach the
command through `env`. A flag that would skip a check is out (`rules/commands.md`).

Done when `Args` is set, every flag is bound to a variable local to the constructor, and
`--help` reads as a sentence per flag.

## 4. Write `RunE`

Apply the first two bullets of `rules/commands.md`: `RunE` reads flags, arguments and the
working directory, then returns one call to `run<Command>`, with `cmd.Context()` first if
the command can block. `server_add.go`'s `RunE` is the shape to copy.

Done when `RunE` holds nothing but reading inputs, building the production dependencies
(`sshInstaller(version)`), and that one `return`.

## 5. Write `run<Command>`

The command's work, every input a parameter: `ctx` first when it can block, then `env`, then
the inputs and fakeable dependencies in `runServerAdd`'s order. A dependency is a function
type or small struct declared in the same file (`dialer`, `installer`). Wrap `ctx` with
`signal.NotifyContext` where the command talks to a Server, as `runServerAdd` does. Reading
local files is not blocking in this sense: `runInit` takes no context.

The rules this step leans on:

- returning versus exiting, and Refusal versus error: `rules/commands.md`;
- asking a person: `env.Ask` and its nil case, `rules/commands.md`;
- wrapping and sentinels: `go.md` §Errors, and the wording of neighbouring commands;
- editing `brama.yaml`: `rules/config.md`;
- anonymization and Classification: `rules/architecture.md`.

Done when every path out of the function is one of: `nil` after rendering, a
`refusal.Refusal`, an error, or `reported(...)` after rendering.

## 6. Render the Result

Declare `<Command>Result` and give it the four `renderer.Result` methods: `Action`,
`Status`, `Headline`, `Fields`. Add `Notes()` for prose a person should read. Build fields
with `renderer.Fields{}.Add`, `AddOptional` and `AddContractOnly`. In `run<Command>`, pass
the Result to `env.Renderer.Result` and wrap its error as the neighbours do:
`rendering the server add result: %w`. A command with no `run<Command>`, like `version`,
returns that call from `RunE`.

Every rule for what goes in a Result is in `rules/output.md`. A new visual element, a
style, glyph or layout, goes in `internal/renderer`, never in the command; read
`references/ui.md` first.

Done when `--json` output carries every fact the headline states.

## 7. Test it

Test through `run<Command>`, as `rules/testing.md` and `go.md` §Testing set out, next to
the neighbouring command's tests.

Done when `go test ./internal/cli -run <Command>` passes and covers the success path, each
Refusal, and the decoded `--json` output.

## 8. Run it

Build and run the command three ways: in a terminal, with `--json`, and piped through
`cat`. Compare against `references/ui.md` §No terminal.

Done when the terminal run is styled, the JSON run prints one object, and the piped run
prints the terminal run's text with no escape codes.

## 9. Record it

A new command or a changed flag is user-facing. Add the `## [Unreleased]` entry that
`docs/agents/changelog.md` describes, and use the commit type `docs/agents/commits.md`
assigns.

Done when the entry is in `CHANGELOG.md`.

## 10. Verify

Run `make check`.

Done when it exits 0. A failure is fixed and `make check` run again, never skipped.
