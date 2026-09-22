# Go guidelines

The Go contract for this repo. It is loaded into every session, so it holds only what most
Go changes need. The depth is in `docs/agents/go/references/`, one file per topic, and each
section below names its file. Why the guidance is split this way:
`docs/adr/0016-go-norms-for-agents-are-layered-by-how-often-they-are-needed.md`.

Examples are written `bad` → `good`. A rule without an example is one `make lint` or
`make vet` already enforces. It is listed so you know the check exists, not so you
remember it.

## Priority

When sources disagree, the one higher in this list wins:

1. **Repo config.** `.golangci.yml`, the `Makefile`, `.github/workflows/`. The only layer
   that fails a build.
2. **Existing code.** Consistency first. See the next section.
3. **Project rules.** `docs/agents/rules/`.
4. **These guidelines.**
5. **References.** `docs/agents/go/references/`.

Where a higher source is silent, a lower one applies. Skills are procedures, not norms, and
sit outside this chain. A skill that states a Go norm defers to the list above.

Tooling to know:

- `make check` is CI, in CI's order. Run it before you call Go work done.
- `make lint` runs golangci-lint (gofumpt, goimports, revive, wrapcheck, godot and the
  rest). `make lint-fix` applies what can be fixed mechanically.
- `make test` runs `go test -race ./...`.
- **The linter does not read `_test.go` files** (`run.tests: false`). Every testing rule
  below is on you.

## Consistency first

Before writing, read the code next to the change: the same file, then the same package.
Match its error wording, its test layout, its helper and receiver names. Code that looks
like its neighbours is easier to read than code that follows a better rule alone.
→ `references/packages.md`

- Don't add a second way to do something the package already does one way.
  `github.com/stretchr/testify/assert` in a stdlib-only suite → `if got != want { t.Errorf(...) }`
- Reach for the language, then the standard library, then an existing dependency. Only
  then consider a new module. `golang.org/x/exp/slices` → `slices`
- Don't copy a local mistake to stay consistent. If matching the neighbours would spread
  a bug or a lint violation, fix it or leave it alone. Don't repeat it.
  `err.Error() == "..."` because the test above does it → `errors.Is(err, config.ErrNotFound)`
- Comments say why, not what.
  `// loop over the tables` → `// Children first, so a foreign key never points at a missing row.`
- brama has no logger. Don't add `log` or `log/slog` calls to report progress or failures
  from inside a package. Return the error.
  `log.Printf("skipping %s: %v", table, err)` → `return fmt.Errorf("copying %s: %w", table, err)`
- Name domain concepts with `CONTEXT.md`'s terms. Its _Avoid_ lists apply to identifiers too.
  `func (s Stage) Name()` → `func (e Environment) Name()`

## Context

Enforced: `context.Context` is the first parameter; context keys are not basic types;
a discarded `cancel` is reported by `go vet`. → `references/context.md`

- Never store a context in a struct. Pass it to each method that needs it.
  `type Puller struct{ ctx context.Context }` → `func (p *Puller) Pull(ctx context.Context) error`
- Make a root context only at an entrypoint: the command setup in `internal/cli`.
  Everything below it takes the caller's context.
  `ctx := context.Background()` inside `Introspect` → `func (i *Introspector) Introspect(ctx context.Context)`
- In tests use `t.Context()`, as most of the suite does. It is cancelled before cleanup
  runs. `context.Background()` in a test → `t.Context()`
- A function that takes a context and can block returns an `error`, so the caller can
  tell it was cancelled. `func Wait(ctx context.Context)` → `func Wait(ctx context.Context) error`
- Pass data as parameters, not context values.
  `context.WithValue(ctx, envKey{}, env)` → `func Pull(ctx context.Context, env config.Environment)`
- Don't write in a doc comment that cancelling the context stops the function. Every
  context does that. Document only behaviour that differs.
  `// Pull stops when ctx is cancelled.` → drop the sentence

## Errors

Enforced: returned errors are checked (errcheck); error strings are lowercase with no
closing punctuation; sentinels are named `ErrFoo`; an error from outside
`github.com/bramaos/brama/internal/...` is wrapped before it is returned (wrapcheck);
the error branch returns early instead of using `else`. → `references/errors.md`

- Wrap with `%w`, adding what this frame knows and the caller does not. The repo wraps
  with `%w` throughout. A cause goes last, a sentinel naming the category goes first
  (`"%w: %s", ErrServerExists, name`). `fmt.Errorf("failed: %v", err)` → `fmt.Errorf("parsing %s: %w", Filename, err)`
- Don't wrap an error from brama's own packages just to wrap it. They already speak in
  brama's words. That is why wrapcheck skips them.
  `return fmt.Errorf("error: %w", err)` after `config.Load` → `return err`
- Check identity with `errors.Is` / `errors.As`. Never use `==` or the message text.
  `err == config.ErrNotFound` → `errors.Is(err, config.ErrNotFound)`
- A sentinel when callers branch on the condition. An error type when they need its
  fields. Document either one on the function that returns it.
  callers matching `"already registered"` in the text → `var ErrServerExists = errors.New("server already registered")`
- Return the `error` interface, never a concrete error type. A nil pointer inside an
  interface is not nil. `func Parse(b []byte) *ParseError` → `func Parse(b []byte) error`
- Report a missing result with a second value, not a magic one.
  `func prefix() string // "" if unknown` → `func prefix() (string, bool)`
- Handle an error once. Return it, or deal with it. Don't print it and also return it.
  `fmt.Fprintln(os.Stderr, err); return err` → `return err`
- `panic` is for programmer bugs, not input. `Must` helpers only on constants at init.
  `panic(err)` on a bad `brama.yaml` → `return fmt.Errorf("%s: %w", path, err)`

## Concurrency

brama runs no goroutines today. These rules are for the change that adds the first ones.
Read `references/concurrency.md` before writing it: sharing, cancellation and races are
covered there.

- Prefer synchronous functions. A caller can add concurrency, but can't remove it.
  `func Tables(ctx context.Context) <-chan Table` → `func Tables(ctx context.Context) ([]Table, error)`
- Every goroutine has a visible end. The function that starts it waits for it.
  `for _, t := range tables { go copyTable(t) }` → `for _, t := range tables { wg.Go(func() { copyTable(ctx, t) }) }; wg.Wait()`
- When goroutines can fail, use `errgroup` (`golang.org/x/sync`, already an indirect
  dependency) so the first error cancels the rest.
  errors collected in a mutex-guarded slice → `g, ctx := errgroup.WithContext(ctx)`
- Tests of timing use `testing/synctest`, never real sleeps.
  `time.Sleep(100 * time.Millisecond)` → `synctest.Test(t, func(t *testing.T) { ...; synctest.Wait() })`

## Interfaces

Enforced: an exported function doesn't return an unexported type. → `references/interfaces.md`

- Define an interface where it is used, with only the methods that code calls. The
  exception is an interface that is the product, a protocol other packages implement,
  like `schema.Introspector`. `type Store interface` beside its only `type store struct` → `func New() *Store`
- Accept interfaces, return concrete types.
  `func NewIntrospector(db *sql.DB) schema.Introspector` → `func New(db *sql.DB, database string) *Introspector`
- No interface without a second implementation or a real test seam.
  `type ConfigLoader interface` with one implementation and no fake → call `config.Load`
- Keep them small. A one-method interface is named for its method plus `-er`.
  `type TableDetection interface{ Detect(...) }` → `type Detector interface{ Detect(...) }`
- An implementation of a protocol interface asserts it at compile time, as `schema/mysql`
  does. No assertion, so a drifted method set fails at a distant call site → `var _ schema.Introspector = (*Introspector)(nil)`
- Don't export a test double from a production package.
  `func NewFakeRemote()` in `internal/state` → `type fakeRemote struct` in `state_test.go`

## Packages

→ `references/packages.md`

- Code lives under `internal/`, binaries under `cmd/`, and tests that need the container
  rig under `test/testenv` behind `//go:build testenv`. `pkg/preset` → `internal/preset`
- A new package holds one distinct concept and is named for what it provides. Never
  `util`, `common`, `helpers`, `models`.
  `internal/util.ReadPrefix` → a function in `internal/adapter`, which owns prefixes
- Don't repeat the package name in function names either. The linter only catches types.
  `preset.NewPreset`, `config.LoadConfig` → `preset.New`, `config.Load`
- No mutable package-level state. Pass dependencies in.
  `var db *sql.DB` set by `Init()` → `func New(db *sql.DB, schemaName string) *Introspector`
- No `init()` with side effects. No blank imports outside `main` and tests, except
  `embed`. `func init() { adapters = append(adapters, wordpress.Adapter{}) }` → build the list where it is used
- Break an import cycle by moving code, not by adding an interface to route around it.
  an interface in `config` so `preset` can call back into it → move the shared type to the package both import
- Exactly one file carries the package comment.
  `// Package config ...` atop `load.go` and `edit.go` → only in `config.go`

## Naming

Enforced: initialisms keep one case (`userID`, `URL`); receivers aren't `this` or `self`
and keep one name across a type's methods; an exported type doesn't repeat its package
(`config.ConfigFile`); MixedCaps, no underscores. → `references/naming.md`

- No `Get` prefix. `GetPrefix()` → `Prefix()`
- Don't repeat what the package, receiver or type already says.
  `func (c *Config) ConfigPath()` → `func (c *Config) Path()`
- A name's length follows its scope. Leave the type out of it.
  `tablesSlice`, `numTables` → `tables`, `n`
- Receivers are one or two letters, an abbreviation of the type.
  `func (config *Config)` → `func (c *Config)`
- Name constants for their role, not their value. `const Two = 2` → `const maxAttempts = 2`
- Don't shadow a package you still need. `url := env.URL` before calling `url.Parse` → `u := env.URL`

## Doc comments

Enforced: exported identifiers have a doc comment that starts with the name (exported
methods in `internal/cli` are exempt); every package has a package comment;
doc comments end in a period (godot).
→ `references/doc-comments.md`

- Say what the signature doesn't: why, edge cases, the errors returned, what the caller
  must clean up. `// Load loads the config.` → `// Load reads brama.yaml at or above dir. It returns ErrNotFound if there is none.`
- Unexported types and non-obvious unexported functions get comments too, starting with
  their name. `// helper for check` → `// declined reports whether every destination declined the keep.`
- Document an interface's contract on the interface: what an implementation must do, and
  which errors it returns. `// Detector detects.` → `// A Detector reports whether dir holds its framework's project. It reads files and nothing else.`
- Put the reason in the comment. The history belongs in the commit.
  `// changed after #58` → `// The prefix is read every time, never stored, so it cannot go stale.`

## Testing

Not linted. All of it is on you. → `references/testing.md`

- Use only the standard `testing` package. No assertion libraries.
  `assert.Equal(t, want, got)` → `if got != want { t.Errorf("Parse(%q) = %q, want %q", in, got, want) }`
- A failure names the function and input, then got, then want.
  `t.Errorf("wrong result")` → `t.Errorf("Resolve(%q) = %v, want %v", ref, got, want)`
- `t.Errorf` to keep going, `t.Fatalf` when nothing after it can mean anything: a failed
  setup, or an error where a value was needed. `t.Errorf("Load() = %v", err)` then using its result → `t.Fatalf("Load() = %v", err)`
- Table-driven when cases share logic. A `t.Run` name is a short phrase for the case, and
  the failure message still prints the input. `t.Errorf("case %d failed", i)` → `t.Run(tt.name, ...)` with inputs in the message
- Compare whole values, not field by field (`==`, `slices.Equal`, `maps.Equal`, or
  `reflect.DeepEqual` for nested data). `if got.Name != want.Name || got.Type != want.Type` → `if got != want`
- Test error identity with `errors.Is`. Check message text only when the wording is the
  behaviour under test, as it is for a message shown to the user.
  `strings.Contains(err.Error(), "not found")` → `errors.Is(err, config.ErrNotFound)`
- Helpers call `t.Helper()`, take `t` first (after a context), and fail with `t.Fatalf`
  saying what setup broke. `func writeConfig(t *testing.T, s string) error` → `func writeConfig(t *testing.T, s string) { t.Helper(); ... }`
- Keep tests hermetic. Use `t.TempDir()`, `t.Setenv`, `t.Chdir`, `t.Cleanup`, with no
  shared package-level state. `os.MkdirTemp` plus a deferred `os.RemoveAll` → `t.TempDir()`
- Don't add `t.Parallel()`. No test in the suite uses it, and a parallel test cannot use
  `t.Setenv` or `t.Chdir`. `t.Parallel()` at the top of a test → leave it out
- Never call `t.Fatal` from a goroutine the test started. `go func() { t.Fatal(err) }()` → `t.Error(err); return`
- A test that needs real servers goes in `test/testenv` with `//go:build testenv` and
  runs through `make testenv-test`. `make test` never compiles it.
  an `internal/ssh` test that dials the staging container → `test/testenv/…_test.go` with `//go:build testenv`
