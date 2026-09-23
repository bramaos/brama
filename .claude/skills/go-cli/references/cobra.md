# Cobra, as brama uses it

Only the patterns brama has chosen, for `github.com/spf13/cobra` v1.10. For any other API
detail, fetch it from Context7 (`/spf13/cobra`) at the time of use rather than from memory.

## The command literal

- Built in its constructor, `new<Command>Cmd(env *console, …) *cobra.Command`. Flags bind
  to variables declared in the constructor and captured by `RunE`, never to package-level
  variables (`rules/architecture.md`).
- Sets `Use`, `Short`, `Args`, `SilenceUsage: true` and `RunE`. Adds `Long` when a
  sentence of `Short` is not enough. Never `Run`: `RunE` is how an error reaches `Main`.
- `Use` names positional arguments in angle brackets: `"add <name>"`.
- `Short` is one imperative phrase, capitalised, no closing period: `"Register a server
  and install the shim on it"`.
- A group is a command with no `RunE`, holding its subcommands through `cmd.AddCommand`.

## Arguments

- Always set `Args`: `cobra.NoArgs` or `cobra.ExactArgs(n)`. Cobra reports a wrong count
  before `RunE` runs.
- A positional argument names the thing the command acts on (`server add <name>`).
  Everything else is a flag.

## Flags

- Local to the command: `cmd.Flags().StringVar(&host, "host", "", "…")`. Names are
  kebab-case; help text is lowercase, has no closing period, and says what the flag does
  to this run.
- Required: `_ = cmd.MarkFlagRequired("host")`, right after the flag is declared.
- `--dry-run` is the name for "report what would happen, and change nothing".
- Persistent flags live on root only: `--json` and `--non-interactive`. Commands read
  them from `env.JSON` and `env.Ask`, never from the flag set.

## What root owns

- `Main` builds root and runs it with `fang.Execute`, which calls `ExecuteContext`, so
  `cmd.Context()` is never nil. No command calls `Execute`.
- Root's `PersistentPreRun` fills `env` from the persistent flags. Cobra runs only the
  nearest `PersistentPreRun`, so a subcommand that declared one would shadow root's and
  run with a nil `env.Renderer`. Put setup in `run<Command>`.
- `SilenceUsage` and `SilenceErrors` are set on root; `Main` renders every error.
