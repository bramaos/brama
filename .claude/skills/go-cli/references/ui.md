# Styled output: fang and lipgloss

How brama's output contract is implemented. The contract itself, what a command reports
and through what, is `docs/agents/rules/output.md`; this file is for changing
`internal/renderer` or the fang wiring in `internal/cli/cli.go`. Versions: `charm.land/fang/v2`
v2.0, `charm.land/lipgloss/v2` v2.0. For API detail beyond this file, fetch
`/charmbracelet/fang` or `/charmbracelet/lipgloss` from Context7.

## fang wiring

`Main` runs root through `fang.Execute(context.Background(), root, …)` with two options:

- `fang.WithVersion(version)`: the `--version` flag.
- `fang.WithErrorHandler(func(io.Writer, fang.Styles, error) {})`: deliberately empty.
  fang's default handler would print an error a second time, and would print a Refusal as
  a failure. `Main` renders both through `env.Renderer`, then maps the exit code.

What fang then does on its own: styles `--help` and usage, adds a hidden `man` command and
shell completion, sets `SilenceUsage` and `SilenceErrors` on root. brama passes no colour
scheme, so help uses fang's default. It passes no `WithNotifySignal` either: a command
that talks to a Server installs its own `signal.NotifyContext`.

## lipgloss styling

All styles are in `internal/renderer/human.go`, as package-level values built once. They
use terminal palette indices, not hex, so a person's terminal theme picks the shade:

| Style          | Colour       | Used for                                |
| -------------- | ------------ | --------------------------------------- |
| `successStyle` | `"2"` green  | `✓` before a successful headline        |
| `refusedStyle` | `"3"` yellow | `!` for a partial result, `✗ refused —` |
| `errorStyle`   | `"1"` red    | `✗` before an error                     |
| `labelStyle`   | `"244"` grey | field labels, `run:`                    |
| `noteStyle`    | grey italic  | Notes                                   |
| `fixStyle`     | `"6"` cyan   | the command that clears a Refusal       |

A new element reuses a style when its meaning matches one above, and otherwise adds one
beside them.

In lipgloss v2, `Style.Render` always returns full-colour ANSI. Colour is reduced only when
the string is printed through `lipgloss.Fprintf` / `Fprintln`, which wrap the writer in a
`colorprofile.Writer`. So every styled write goes through `lineWriter` in `human.go`,
which calls those. A rendered string written with `fmt.Fprint*` keeps its escape codes in
a pipe.

## Colour profile and width

- **Profile.** `colorprofile` detects per writer, on every print: whether it is a
  terminal, then `TERM`, `COLORTERM`, `NO_COLOR`, `CLICOLOR` and `CLICOLOR_FORCE`.
  `NO_COLOR` wins over the rest. brama never branches on the profile itself.
- **Background.** fang asks whether the background is dark only when stdout is a
  terminal, to choose help colours. brama's styles do not depend on it.
- **Width.** brama's renderer never measures the terminal. It aligns fields by padding
  each label to the longest one, using `len`, so labels must be ASCII. A new element that
  measures styled or non-ASCII text uses `lipgloss.Width`. fang's help reads the terminal
  width from stdout, caps it at 120, and uses 120 when stdout is not a terminal.

## No terminal

- **Output.** A writer that is not a terminal (a pipe, a file, a `bytes.Buffer` in a
  test) gets the no-TTY profile, and every escape code is stripped. The layout is
  unchanged, so a piped run is the terminal run as plain text. That is also why tests can
  compare human output without escape codes.
- **Streams.** A Result goes to `env.Out`; a Refusal or an error goes to `env.Err`.
  Stripping is decided per stream.
- **JSON.** `renderer.JSON` never styles anything, whatever the terminal.
- **Prompts.** `env.Ask` is nil unless both stdin and stdout are terminals and neither
  `--json` nor `--non-interactive` was passed (`interactive` in `cli.go`).
- **Help.** fang drops its code-block background when the profile is ASCII or below.
