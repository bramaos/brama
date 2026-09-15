# Charmbracelet Ecosystem Assessment for brama

An evaluation of Charm's open-source CLI/TUI libraries for a single static binary that pulls anonymized databases and files via SSH, serving both human and machine callers.

## Summary Table

| Package | Purpose | Latest Version | Released | Maintained | Non-TTY Behaviour | Verdict for brama |
|---------|---------|-----------------|----------|-----------|-------------------|------------------|
| **bubbletea** | TUI framework; Elm architecture; state machine for full-screen terminal apps | v2.0.9 | Aug 2025 | Yes | TTY only; caller must branch to avoid | **Maybe** (not for db pull progress) |
| **bubbles** | Reusable TUI components: spinner, text input, progress, table, list, file picker, etc. | v2.2.1 | Aug 2024 | Yes | TTY only; ANSI styling | **Maybe** (spinner/progress as fallback) |
| **lipgloss** | Terminal styling (CSS-like); automatic color downsampling; strips colors for non-TTY | v2.0.6 | Aug 2026 | Yes | Auto-detects TTY; strips colors if piped | **Adopt** (use Writer functions) |
| **huh** | Interactive form/prompt builder; supports accessible mode for screen readers | v2.0.3 | Mar 2026 | Yes | Accessible mode via `WithAccessible(true)` or `TERM=dumb`; outputs plain prompts | **Adopt** (for anonymize init classification walk) |
| **fang** | Cobra batteries-included: styled errors, version, manpages, completions | v2.0.1 | Mar 2026 | Yes | Inherits Cobra; styling via colorprofile, so TTY-aware | **Adopt** (returns the error; caller owns the exit code) |
| **log** | Minimal structured logging; JSON, text, logfmt formatters; slog-compatible | v2.0.1 | Sep 2026 | Yes | No TTY detection; formatters are pure | **Adopt** (with JSON formatter for --json flag) |
| **glamour** | Markdown renderer for terminal; stylesheet-based; ANSI output | v2.0.1 | Jun 2026 | Yes | Pure rendering; no TTY detection; use with Lip Gloss | **Avoid** (not needed for brama) |
| **vhs** | Terminal recording; converts .tape script to GIF/SVG; CI-friendly | v0.12.0 | Sep 2026 | Yes | Tool only; not a library; for demos not product | **Avoid** (tool, not library; for docs not code) |
| **gum** | Shell script CLI tool; wraps Bubbles/Lip Gloss; standalone binary | v2.0.1 | Sep 2026 | Yes | Tool only; auto-detects TTY in shell | **Avoid** (Go code can use libraries directly; gum is for shell scripts) |
| **wish** | SSH server framework; not SSH client | v2.0.4 | Sep 2024 | Yes | SSH server only; irrelevant for brama | **Avoid** (brama is SSH client, not server) |
| **harmonica** | Spring animation library; physics-based smooth motion; works in CLI | v0.2.0 | Apr 2026 | Yes | ANSI-based; TTY-dependent | **Maybe** (not needed for db pull; over-engineered) |
| **freeze** | Terminal snapshot tool; generates PNG/SVG/WebP of code/output | v0.2.2 | Apr 2025 | Yes | Tool only; for aesthetics not product | **Avoid** (tool, not library; for screenshots not code) |
| **x/ansi** | Experimental; ANSI parsing, term utilities, TTY detection | No releases | Unstable | Experimental | Includes `term.IsTerminal()` for TTY detection | **Maybe** (only if ansi parsing needed; unstable) |

## Module paths — use `charm.land`, not `github.com`

Verified 2026-09-15 by reading each repo's `go.mod` on `main`. The `module` line is authoritative: it *is* the import path.

| Package | Module path | Latest tag |
|---|---|---|
| huh | `charm.land/huh/v2` | v2.0.3 |
| lipgloss | `charm.land/lipgloss/v2` | v2.0.6 |
| bubbletea | `charm.land/bubbletea/v2` | v2.0.9 |
| bubbles | `charm.land/bubbles/v2` | v2.2.1 |
| fang | `charm.land/fang/v2` | v2.0.1 |
| log | `charm.land/log/v2` | v2.0.1 |

**This is not cosmetic.** The old `github.com/charmbracelet/...` paths still resolve on pkg.go.dev but serve stale versions — `github.com/charmbracelet/huh/v2` sits at a v2.0.0 pseudo-version from October 2025, and `github.com/charmbracelet/lipgloss/v2` at `v2.0.0-beta.3` from July 2025. Importing the github path silently pins a year-old beta.

**Trap:** fang's README still shows `import "github.com/charmbracelet/fang"` while its own `go.mod` declares `module charm.land/fang/v2`. The README is wrong; follow the `go.mod`.

Note: the "Go module: `/charmbracelet/<name>`" lines in the per-package sections below are *repository* names, not import paths. Use the table above.

## Per-Package Detail

### bubbletea

**What it does:** Elm-architecture TUI framework. State-driven, functional. Manages input, updates, and rendering in a loop at 60 FPS. Full-screen terminal control.

**Versions:** v2.0.9 (Aug 19, 2025). Actively maintained with recent fixes for keyboard, UI rendering. Go module: `/charmbracelet/bubbletea`.

**TTY behaviour:** **TTY-only.** Requires a terminal for rendering. No documented non-TTY fallback. When stdout is piped, the program must detect this *before* calling `tea.NewProgram()` and either skip TUI rendering or branch to plain output. Lipgloss layers on top and only controls color stripping, not rendering itself. Source: https://github.com/charmbracelet/bubbletea/blob/main/README.md

**Progress reporting:** Provides a `progress.Model` component in bubbles. Renders with ANSI; animates via spring physics. Works only in TTY mode. Not suitable for multi-step remote operations in non-interactive contexts; would require branching logic to fall back to `--json` table output.

**Composability:** Couples to Lipgloss, Bubbles, and the model pattern. Pulling in bubbletea for db pull `--dry-run` pre-flight table is overkill unless the table is interactive.

**License:** MIT. https://github.com/charmbracelet/bubbletea/blob/main/LICENSE

**Dependency weight:** Core framework; widely used; ~200 KB binary size impact (unverified).

---

### bubbles

**What it does:** Reusable TUI components for Bubble Tea: spinner, text input, text area, table, list, progress, paginator, file picker, timer, stopwatch, help, key management.

**Versions:** v2.2.1 (Aug 24, 2024). Recent bug fix for textarea. Go module: `/charmbracelet/bubbles`.

**TTY behaviour:** All components inherit Bubble Tea's TTY-only constraint. Render via `View()` method returning ANSI-styled strings. Spinner and progress emit plain text frames but styled with Lip Gloss ANSI codes. No non-TTY variants.

**Specific components for brama:**
- **Spinner:** Plain-text frames (".", "..", "...") but ANSI-styled. Could fallback to plain text frames in non-TTY.
- **Progress:** Spring-animated bar; ANSI-styled. Unsuitable for non-TTY.

**Composability:** Tight coupling to Bubble Tea's message loop and `Update/View` pattern.

**License:** MIT. https://github.com/charmbracelet/bubbles

**Verdict:** Useful for interactive prompts or display, but progress bar is not suitable for multi-step remote work in non-TTY contexts.

---

### lipgloss

**What it does:** CSS-like terminal styling. Borders, padding, colors, alignment. Auto-detects terminal color profile (ANSI 16, ANSI 256, TrueColor) and downsample colors to match. **Strips all ANSI codes when output is not a TTY.**

**Versions:** v2.0.6 (Aug 11, 2026). Latest; actively maintained. Go module: `/charmbracelet/lipgloss`.

**TTY behaviour:** **Auto-detects.** Uses `colorprofile.Detect(os.Stdout, os.Environ())` to sniff terminal capabilities. Writer functions (`Println`, `Fprint`, `Sprint`) automatically apply downsampling and strip colors for non-TTY.

**Key API for non-TTY safety:**
- Use `lipgloss.Println()`, `lipgloss.Fprint()`, `lipgloss.Sprint()` instead of direct `fmt.Print()`.
- These automatically detect TTY via the default `lipgloss.Writer = colorprofile.NewWriter(os.Stdout, os.Environ())`.
- When piped, colors are stripped; structure is preserved.
- Custom writer: `lipgloss.Writer = colorprofile.NewWriter(os.Stderr, os.Environ())`.

Source: https://github.com/charmbracelet/lipgloss/blob/main/README.md; https://github.com/charmbracelet/lipgloss/blob/main/_autodocs/writer-and-utilities.md

**Color downsampling:** Handled transparently. No `--no-color` flag needed (respects `NO_COLOR` env var if added by user).

**Composability:** Used by nearly all Charm libraries. Zero-dependency styling layer.

**License:** MIT. https://github.com/charmbracelet/lipgloss

**Verdict:** **Adopt.** Essential for safe, TTY-aware tables and formatting. Use Writer functions; never call `fmt.Print()` directly on styled output.

---

### huh

**What it does:** Interactive form/prompt builder. Fields: text input, select, multiselect, confirm, text area. Supports validation, custom keybinds, environment variable integration.

**Versions:** v2.0.3 (Mar 10, 2026). Recent fixes for multiline option cutoff. Go module: `/charmbracelet/huh`.

**TTY behaviour:** **Adaptive.** Has explicit `WithAccessible(true)` API to disable TUI and fall back to plain text prompts. Accessible mode reads from stdin, writes to stdout, and outputs simple "Key: ?" prompts.

**Accessible mode triggers:**
- Manual: `form.WithAccessible(true)`.
- Environment: `TERM=dumb` or `ACCESSIBLE=1`.

**API for accessible mode:**
```go
form := huh.NewForm(...).WithAccessible(true)
err := form.Run()  // Outputs plain prompts, no TUI
```

Source: https://github.com/charmbracelet/huh/blob/main/_autodocs/api-reference/form.md; https://github.com/charmbracelet/huh/blob/main/_autodocs/configuration.md

**Classification walk for `brama anonymize init`:** Huh is ideal. A loop of `Select` fields (fake/keep/drop) with optional grouping by table and discriminator key can be pre-filled from flags for scripted runs, then fall back to interactive TUI when called by a human.

**Composability:** Self-contained; depends on Bubble Tea and Lip Gloss under the hood, but abstraction is clean.

**License:** MIT. https://github.com/charmbracelet/huh

**Verdict:** **Adopt.** Primary choice for `brama anonymize init` classification walk. Accessible mode satisfies non-TTY requirement.

---

### fang

**What it does:** Cobra batteries-included. Wraps Cobra with styled errors, automatic `--version` flag, manpage generation, shell completions. Integrates Lip Gloss v2.

**Versions:** Repo created 2024-11-22. First release v0.1.0 on 2025-06-18; latest v2.0.1 on 2026-03-11. 1,942 stars, not archived, last pushed 2026-05-04. Go module: `/charmbracelet/fang`.

**Error handling & exit codes:** **No conflict with exit code 42.** `fang.Execute` contains no `os.Exit` call anywhere in its library code. Verified by reading the source:

```go
func Execute(ctx context.Context, root *cobra.Command, options ...Option) error {
	...
	if err := root.ExecuteContext(ctx); err != nil {
		w := colorprofile.NewWriter(root.ErrOrStderr(), os.Environ())
		opts.errHandler(w, makeStyles(mustColorscheme(opts.colorscheme)), err)
		return err
	}
	return nil
}
```

It returns the error to the caller, who decides the exit code. It also sets `root.SilenceUsage = true` and `root.SilenceErrors = true`, so Cobra does not print anything itself.

Source: https://raw.githubusercontent.com/charmbracelet/fang/main/fang.go (verbatim, verified 2026-09-15)

**Exported options:** `WithoutCompletions()`, `WithoutManpage()`, `WithColorSchemeFunc(ColorSchemeFunc)`, `WithTheme(ColorScheme)` (deprecated), `WithVersion(string)`, `WithoutVersion()`, `WithCommit(string)`, `WithErrorHandler(ErrorHandler)`, `WithNotifySignal(...os.Signal)`.

**Version & manpages:** Automatic `--version` (from `debug.BuildInfo()` unless overridden by `WithVersion`). Manpages via a hidden `man` subcommand. Both disableable.

**The real sharp edge:** fang's default error handler prints *every* returned error styled as an error. A Refusal is not an error — `CONTEXT.md` defines it as an outcome where nothing went wrong. So brama must supply `WithErrorHandler` that renders a Refusal differently, or return Refusals through a channel other than the error return.

**Verdict:** **Adopt.** It is compatible with exit code 42, and `--version` / manpages / completions / styled help are exactly the "less manual work" brama wants. Requires a custom `ErrorHandler` so Refusals don't render as failures.

---

### log

**What it does:** Minimal, colorful structured logging. Supports Text, JSON, and Logfmt formatters. Context integration, caller reporting, log levels (Debug, Info, Warn, Error, Fatal).

**Versions:** v2.0.1 (Sep 3, 2026). Recent fix for log level formatting. Go module: `/charmbracelet/log`.

**JSON output:** **Yes.** `log.JSONFormatter` emits one JSON object per line. Field order: time, level, msg, plus key-value pairs passed to log call.

Example:
```go
logger := log.New(os.Stderr)
logger.SetFormatter(log.JSONFormatter)
logger.Info("Event occurred", "user", "alice", "id", 42)
// Output: {"time":"2024-01-15 10:30:45","level":"info","msg":"Event occurred","user":"alice","id":42}
```

Source: https://github.com/charmbracelet/log/blob/main/_autodocs/api-reference/formatters.md

**Comparison to stdlib `log/slog`:** Charmbracelet/log *integrates* with slog. Can be used as an slog handler: pass logger to `slog.New(log.Handler(logger))`. Adds colorful human-readable output; slog is structured-log framework. Charm log is lighter-weight but slog is more standard. No strong reason to prefer over slog, but compatible.

**TTY behaviour:** No automatic TTY detection in log itself. Formatters are pure (always produce same output for same input). Color stripping is *caller's responsibility*; recommend using Lip Gloss for styled output.

**License:** MIT. https://github.com/charmbracelet/log

**Verdict:** **Adopt.** Use JSONFormatter for `--json` flag. Works well alongside plain text for human output. Lightweight; minimal dependency footprint.

---

### glamour

**What it does:** Stylesheet-based markdown renderer for terminal. Pure rendering; no TTY detection. Outputs ANSI-styled text.

**Versions:** v2.0.1 (Jun 12, 2026). Recent wrap-writer fix. Go module: `/charmbracelet/glamour`.

**Use cases:** CLI docs, README display, markdown in terminal output.

**TTY behaviour:** **Pure.** Does not detect TTY; always emits ANSI. Caller should use Lip Gloss Writer functions to strip colors if needed. README warns: "the renderer is pure and doesn't have access to terminal capabilities."

Source: https://github.com/charmbracelet/glamour (README)

**Relevance to brama:** Not needed. Brama is not a documentation viewer.

**License:** MIT. https://github.com/charmbracelet/glamour

**Verdict:** **Avoid.** Out of scope for brama.

---

### vhs

**What it does:** Terminal recording tool. Write `.tape` script (commands: Type, Enter, Sleep, Hide, Show, etc.). Run `vhs tape.tape` to generate GIF, SVG, or WEBP video of the terminal session. Official GitHub Action for CI.

**Versions:** v0.12.0 (Sep 9, 2026). Recent Windows console hang fix. Go module: `/charmbracelet/vhs` (tool, not importable library).

**CI integration:** Yes. Can be used to generate demo GIFs in CI via the `charmbracelet/vhs-action` GitHub Action.

**Relevance to brama:** Not for code; for documentation. Would generate a demo GIF of a brama workflow, not integrate into the binary itself.

**License:** MIT. https://github.com/charmbracelet/vhs

**Verdict:** **Avoid.** Useful tool for generating README demos; not a library to import.

---

### gum

**What it does:** Command-line tool (standalone binary) wrapping Bubbles/Lip Gloss. Shell script commands: choose, input, confirm, filter, write, table, etc. Zero Go code needed for interactive shell workflows.

**Versions:** v2.0.1 (Sep 11, 2026). Recent keyboard protocol fix. Go module: `/charmbracelet/gum` (binary, not library; optional Ruby wrapper available).

**Relevance to brama:** Gum is for shell scripts, not Go binaries. Brama should use Huh library directly, not shell out to gum.

**License:** MIT. https://github.com/charmbracelet/gum

**Verdict:** **Avoid.** Use Huh library instead; single static binary requirement rules out spawning gum subprocess.

---

### wish

**What it does:** SSH **server** (not client). Provides sensible defaults and middlewares for building custom SSH applications. Supports Bubble Tea, Git, Logging, etc.

**Versions:** v2.0.4 (Sep 10, 2024). SCP path handling fixes. Go module: `/charmbracelet/wish`.

**Relevance to brama:** Brama is an SSH *client* (pulls data over SSH). Wish is for building SSH servers. Opposite direction.

**License:** MIT. https://github.com/charmbracelet/wish

**Verdict:** **Avoid.** Architectural mismatch; brama consumes SSH, not serves it.

---

### harmonica

**What it does:** Spring animation library. Physics-based smooth motion via damped harmonic oscillators. Works in CLI (ANSI-based animations). Useful for polished TUI transitions.

**Versions:** v0.2.0 (Apr 15, 2026). Particle physics update. Go module: `/charmbracelet/harmonica`.

**Relevance to brama:** Over-engineered for a CLI tool. Useful for Bubble Tea TUI apps with smooth state transitions, not for a single-binary data-pulling tool.

**License:** MIT. https://github.com/charmbracelet/harmonica

**Verdict:** **Maybe.** Not essential. If progress bar needs smooth animation (unlikely for long-running remote operations), harmonica is the underlying engine. Use only if Bubbles progress bar is adopted, which is itself a weak choice for non-TTY.

---

### freeze

**What it does:** Generate PNG/SVG/WebP images of code or terminal output. Interactive TUI or CLI flags. Customizable styling.

**Versions:** v0.2.2 (Apr 1, 2025). Version flag fix. Go module: `/charmbracelet/freeze` (tool, not library).

**Relevance to brama:** Tool for aesthetics; not for code integration.

**License:** MIT. https://github.com/charmbracelet/freeze

**Verdict:** **Avoid.** Tool, not library.

---

### x/ansi (charmbracelet/x)

**What it does:** Experimental package repository. Includes: ANSI parsing, terminal utilities (`ansi`, `term`, `termios`, `xpty`, `vt`), color tools, UI helpers, testing tools.

**Versions:** No releases; explicitly unstable. Source-only. Go module: `/charmbracelet/x`.

**Stability:** "No promises of backwards compatibility." Packages may graduate or be removed.

**Relevant sub-packages:**
- **`x/term`:** `IsTerminal(fd uintptr) bool` for TTY detection.
- **`x/ansi`:** ANSI parsing and sequence handling.

Source: https://github.com/charmbracelet/x

**Use case:** If brama needs to parse ANSI sequences or needs platform-independent TTY detection, `x/term` is available. But Lipgloss already handles ANSI output safety.

**License:** MIT. https://github.com/charmbracelet/x

**Verdict:** **Maybe.** Only adopt if the unstable API cost is worth it. For TTY detection, consider using standard library `isatty` (cgo-free) or building your own check with Lip Gloss's color profile detection.

---

## Specific Questions Answered

### fang: exit code 42 and custom error handling

**Question:** Does fang handle `--version`, manpages, shell completions, styled errors automatically? CRITICALLY: does it alter process exit codes or intercept errors in a way that would break a custom exit code 42?

**Answer:** Yes to versioning, manpages, completions, styled errors. **And no, fang does not break exit code 42.** `fang.Execute` returns the error from `root.ExecuteContext(ctx)` and never calls `os.Exit`. The exit code is entirely the caller's:

```go
func main() { os.Exit(run()) }

func run() int {
	if err := fang.Execute(ctx, rootCmd, fang.WithErrorHandler(bramaErrors)); err != nil {
		if errors.Is(err, ErrRefused) {
			return 42
		}
		return 1
	}
	return 0
}
```

This keeps `os.Exit` out of every `RunE`, so deferred cleanup still runs — which matters for brama, where the Shim must remove its temporary mapping files on the way out.

Source: https://raw.githubusercontent.com/charmbracelet/fang/main/fang.go (verified verbatim, 2026-09-15)

**Verdict:** Fang is compatible. The earlier claim in this document that it "breaks exit code 42" was wrong and has been corrected.

---

### huh: accessible mode and form pre-fill

**Question:** Does huh have an accessible/non-TTY mode? Name the exact API. Can a form be pre-filled from flags so the same code path serves interactive and scripted use?

**Answer:** Yes. Exact API:

```go
form := huh.NewForm(...).WithAccessible(true)  // Manual trigger
// Or: TERM=dumb ./brama anonymize init        // Environment trigger
err := form.Run()
```

Pre-fill from flags: Not built-in, but achievable. Set field values before calling `Run()` via field methods (e.g., `field.SetValue(val)`). In accessible mode, huh reads/writes plain text prompts and respects pre-filled values.

Example (inferred from API docs):
```go
form := huh.NewForm(
    huh.NewGroup(
        huh.NewSelect().Value(&choice).Options(...),
    ),
).WithAccessible(accessible)  // branch on env or flag

if accessible {
    form.SetValue(prefilledValue)  // Pre-populate from flags
}
err := form.Run()
```

**Verdict:** Huh is suitable for classification walk that branches between interactive TUI and scripted accessible mode.

---

### bubbletea: non-TTY rendering

**Question:** What is the documented approach for "render a TUI when interactive, plain text when piped"? Is there a first-party helper or is it caller-branching?

**Answer:** No first-party helper. Caller must branch. Check TTY status *before* calling `tea.NewProgram()`:

```go
// Pseudo-code
if isatty.IsTerminal(os.Stdout.Fd()) {
    // Use Bubble Tea
    _, err := tea.NewProgram(model).Run()
} else {
    // Plain text output
    fmt.Println(model.View())  // Render once without TUI loop
}
```

Bubble Tea's `README.md` does not document this scenario. No `WithFallbackOutput()` or `RenderPlaintext()` mode exists.

**Verdict:** Bubbletea is not suitable for multi-step operations that must work in non-TTY. For `brama db pull --dry-run`, use Lip Gloss tables and plain progress feedback (`echo "Step 1..."`) instead.

---

### lipgloss: color detection and stripping

**Question:** How does it detect color support, and does it strip styling when piped? Name the API (renderer, color profile, termenv).

**Answer:** Uses `colorprofile.Detect(os.Stdout, os.Environ())` to detect color profile (ANSI 16, ANSI 256, TrueColor, or None). Automatically downsamples. **Strips all ANSI codes when output is not a TTY.**

Relevant APIs:
- `lipgloss.Writer`: Default writer; initialized as `colorprofile.NewWriter(os.Stdout, os.Environ())`.
- `lipgloss.Println()`, `lipgloss.Sprint()`, `lipgloss.Fprint()`: Use default writer; auto-detect TTY.
- `colorprofile.Detect()`: Explicit API for manual detection.
- `lipgloss.HasDarkBackground()`: Query background color at runtime.

**No `NO_COLOR` built-in, but `colorprofile` respects it.**

Source: https://github.com/charmbracelet/lipgloss/blob/main/README.md

**Verdict:** Safe to use. Always call Writer functions, never `fmt.Print()` on styled text.

---

### log: JSON output and slog comparison

**Question:** Can it emit structured JSON? How does it compare to Go's stdlib `log/slog`, and is there a reason to prefer it?

**Answer:** Yes, `log.JSONFormatter`. Emits one JSON object per line. Compatible with slog via `slog.New(log.Handler(logger))`.

Comparison:
- **Charmbracelet/log:** Lighter-weight; colorful human output; smaller dependency footprint.
- **log/slog:** Standard library; structured logging framework; broader ecosystem; more stable.

**No strong preference.** Choose based on code style. For brama, charmbracelet/log is fine; smaller binary impact. Use `log.JSONFormatter` for `--json` flag output.

Source: https://github.com/charmbracelet/log/blob/main/_autodocs/api-reference/formatters.md

**Verdict:** Charmbracelet/log is adequate. If slog is already in use elsewhere in brama, stick with slog.

---

### Progress reporting for multi-step remote operations

**Question:** Is there any first-party Charm package for progress reporting of a multi-step remote operation?

**Answer:** No single package. Options:
1. **Bubbles progress bar:** ANSI-animated; TTY-only; requires Bubble Tea event loop.
2. **Manual plain text:** `echo "Uploading shim... 1/6"` followed by newline on each step. Safe, simple, non-TTY-compatible. Can be suppressed with `--quiet`.
3. **Structured JSON:** Each step emits a JSON object (if `--json` flag set):
```json
{"step": 1, "stage": "upload", "status": "in-progress"}
{"step": 1, "stage": "upload", "status": "done"}
{"step": 2, "stage": "introspect", "status": "in-progress"}
```

Harmonica (spring animation) is not a progress reporter; it's for smooth animations only.

**Verdict:** Roll your own. For `brama db pull --dry-run`, output a plain table via Lip Gloss, then `fmt.Println("Continue? [y/N]")` instead of trying to animate it. When `--json` flag is set, emit JSON objects per step.

---

### Packages NOT appropriate for brama

**Which packages are NOT appropriate for brama, and why?**

1. **wish:** SSH server; brama is SSH client.
3. **vhs, freeze:** Tools, not libraries; for documentation, not product code.
4. **gum:** Shell script tool; brama is a single static Go binary.
5. **glamour:** Markdown rendering; out of scope.
6. **harmonica:** Over-engineered animation; not needed for data transfer.
7. **x:** Unstable, experimental; avoid unless absolutely necessary.

**Packages WORTH ADOPTING:**

1. **lipgloss:** For safe, TTY-aware table formatting and output.
2. **huh:** For interactive `brama anonymize init` classification walk with accessible fallback.
3. **log:** For structured logging with JSON formatter for `--json` flag.
4. **bubbles (conditional):** For spinner during long operations, **only if** a fallback plain-text message is output in non-TTY mode.

---

## Risks and Sharp Edges

### 1. Bubbletea TTY-only constraint
Bubble Tea has no non-TTY fallback. If `brama db pull --dry-run` is run in CI or piped, the program must detect this *before* calling `tea.NewProgram()`. Lipgloss and Bubbles components do not solve this; they only control color, not whether to render a TUI at all.

**Mitigation:** Check `isatty.IsTerminal()` or use Lipgloss's `colorprofile.Detect()` to branch logic early. Do not import Bubble Tea if non-TTY is possible.

### 2. Fang renders a Refusal as an error
Not an exit-code problem — fang returns the error and never exits. The problem is presentational: fang's `DefaultErrorHandler` styles anything non-nil as a failure, but `CONTEXT.md` defines a Refusal as an outcome where *nothing went wrong*. Shipping the default handler would contradict the glossary on screen.

**Mitigation:** Supply `fang.WithErrorHandler` that branches on a `ErrRefused` sentinel, or keep Refusals out of the error return entirely.

### 3. Lipgloss color detection — RESOLVED, no issue
`NO_COLOR` is honoured. Verified in `charmbracelet/colorprofile/env.go`:

```go
func envNoColor(env environ) bool {
	noColor, _ := strconv.ParseBool(env.get("NO_COLOR"))
	return noColor
}
```

and enforced in `colorProfile`: `if envNoColor(env) && isatty { if p > ASCII { p = ASCII }; return }`. Documented precedence: "NO_COLOR takes precedence over CLICOLOR/CLICOLOR_FORCE, and will disable colors but not text decoration". Non-TTY output independently returns the `NoTTY` profile, so piping also strips color.

**Mitigation:** None needed. A `--no-color` flag is optional convenience, not a correctness requirement.

### 4. Huh accessible mode is plain text, not JSON
If a caller (AI agent, CI) needs machine-readable output from `brama anonymize init`, accessible mode still outputs plain text prompts and answers. Huh does not have a JSON mode.

**Mitigation:** For machine-readable classification, either:
  - Add a `--json` flag that short-circuits Huh and reads classifications from stdin as JSON.
  - Wrap Huh output parsing (fragile).

### 5. Progress reporting has no blessed solution
Bubbles progress bar requires TTY. Plain text output (newline per step) works but is brittle and not machine-parseable by default.

**Mitigation:** Use `--json` flag for machine output; plain text for humans. Each step is a separate JSON object or a plain `echo` message.

### 6. x/ansi is unstable
No releases, no backwards compatibility guarantee. Packages may be removed or renamed.

**Mitigation:** Avoid unless necessary. Use standard library equivalents (e.g., `text/scanner` for ANSI parsing if needed).

### 7. All libraries auto-detect Lip Gloss colors; no global `--no-color` enforcement
Lipgloss respects terminal capabilities but individual libraries (log, glamour, etc.) may not. A global `--no-color` flag must be manually wired through to all Charm libraries.

**Mitigation:** Create a wrapper for color detection; pass result to all Charm libraries via their respective APIs.

---

## Open Questions

1. ~~Does Lipgloss respect `NO_COLOR`?~~ **RESOLVED: yes.** See Risks §3 for the quoted source.

2. **Can Huh pre-fill form fields from flags before running?**  
   API docs do not explicitly show field pre-fill methods. Inferred from builder pattern but not tested.

3. **Does Bubbletea have a headless or programmatic rendering mode?**  
   No evidence in docs. Rendering is tied to the event loop and TTY.

4. **What is the exact binary size impact of each library?**  
   Unverified. Brama aims for minimal dependencies; weight matters.

5. **Does Charm have a published guide for building CLIs that serve both humans and machines?**  
   Searched blog and docs; no explicit guidance found. Charm focuses on beautiful TUIs, not machine-readable APIs.

6. **Are there known issues in CI/non-TTY contexts for any Charm library?**  
   Issue trackers do not show obvious non-TTY bugs, but this may reflect lack of such usage rather than absence of issues.

7. **Does the x/term package's `IsTerminal()` work cross-platform without cgo?**  
   Likely (platform-independent interface documented), but implementation not reviewed.

8. ~~Can Fang's error handler emit custom exit codes?~~ **RESOLVED: the question was malformed.** Fang never exits; it returns the error and the caller picks the code. No API is needed.

9. **Does Huh's accessible mode read cleanly from a non-TTY stdin?** Accessible mode is documented as plain-text prompts, but whether a piped stdin drives it correctly is untested. Matters for `anonymize init` under `--non-interactive`.

---

## Licensing Summary

All Charm libraries reviewed are **MIT licensed**:

- bubbletea, bubbles, lipgloss, huh, log, glamour, vhs, gum, wish, harmonica, freeze, x

No GPL or restrictive licenses. Safe for commercial use in a single static binary.

---

## Recommendation Matrix

| Feature | Library | Verdict |
|---------|---------|---------|
| TTY-aware table formatting | lipgloss | **Adopt** |
| Interactive classification walk | huh | **Adopt** |
| Structured JSON logging | log | **Adopt** |
| Interactive TUI (full-screen) | bubbletea | **Avoid** (for db pull) |
| Progress bars | bubbles.progress | **Maybe** (with fallback) |
| Help, `--version`, manpages, completions | fang (over Cobra) | **Adopt** (needs custom `ErrorHandler`) |
| Error handling with custom exit codes | caller-owned, `os.Exit(run())` | **Adopt** (works with or without fang) |
| Markdown rendering | glamour | **Avoid** |
| SSH server | wish | **Avoid** |
| Shell script commands | gum | **Avoid** |
| Terminal recording/demos | vhs | **Avoid** |
| Animation | harmonica | **Maybe** |
| Terminal snapshots | freeze | **Avoid** |
| Experimental utilities | x | **Maybe** (only x/term if needed) |

---

## Conclusion

**Adopt Fang, Lipgloss, and Huh.** Between them: styled help / `--version` / manpages / completions for free, TTY-aware output that strips itself when piped, and an interactive classification walk with an accessible fallback. All MIT, all actively released, all CGO-free.

**Avoid Wish, Gum, Vhs, Freeze, and Glamour** — out of scope (Wish is a server; Gum is for shell scripts; Vhs and Freeze are tools, not libraries; Glamour renders markdown brama doesn't have). **Avoid `x`** as unstable.

**Do not call Bubble Tea directly.** It is linked into the binary regardless, because Huh is built on it — so the question is never "is bubbletea a dependency" but "does brama drive its event loop". For a multi-step remote operation with no non-TTY fallback, it shouldn't.

**Charmbracelet/log is optional, and must not own `--json`.** The `--json` payload is a documented contract with agents (§10), so it belongs to `encoding/json` over a versioned struct, on stdout. Logs are diagnostics, on stderr. Different channel, different mechanism.

**Maybe Bubbles (for a fallback spinner) and Harmonica** if the UX benefit justifies the complexity and TTY-branching logic. Low priority.

For `brama db pull --dry-run` multi-step progress, roll a custom solution: plain text newlines per step for humans, JSON objects per step for machines. Lipgloss for table pre-flight summary.

---

## Sources Cited

- https://github.com/charmbracelet/bubbletea/releases
- https://github.com/charmbracelet/bubbletea/blob/main/README.md
- https://github.com/charmbracelet/lipgloss/blob/main/README.md
- https://github.com/charmbracelet/lipgloss/blob/main/_autodocs/writer-and-utilities.md
- https://github.com/charmbracelet/bubbles/releases
- https://github.com/charmbracelet/huh/blob/main/_autodocs/api-reference/form.md
- https://github.com/charmbracelet/huh/blob/main/_autodocs/configuration.md
- https://context7.com/charmbracelet/fang/llms.txt
- https://github.com/charmbracelet/log/releases
- https://github.com/charmbracelet/log/blob/main/_autodocs/api-reference/formatters.md
- https://github.com/charmbracelet/glamour/releases
- https://github.com/charmbracelet/vhs/releases
- https://github.com/charmbracelet/gum/releases
- https://github.com/charmbracelet/wish/releases
- https://github.com/charmbracelet/harmonica/releases
- https://github.com/charmbracelet/freeze/releases
- https://github.com/charmbracelet/x
