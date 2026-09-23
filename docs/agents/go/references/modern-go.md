# Modern Go (1.21 and later)

The idioms newer than most Go written before 2023, which training data and older examples
tend to miss. brama's `go.mod` says `go 1.27.1`, so everything here is available. Our prose,
not the upstream text. Every claim links the release note or document it came from.
Where this file and the code disagree, the code wins (see the Priority section of
`docs/agents/go.md`).

Since Go 1.27, `go test` runs the `stdversion` vet check. It reports standard library
symbols newer than the file's Go version, so nothing below can slip in under an older
`go` line. ([Go 1.27 § go test](https://go.dev/doc/go1.27#go-test))

## Language

- **`min`, `max`, `clear`** (1.21) are built-ins. `clear(m)` empties a map, `clear(s)`
  zeroes a slice. ([Go 1.21 § Language](https://go.dev/doc/go1.21#language))
- **Per-iteration loop variables** (1.22). Each iteration of a `for` loop gets fresh
  variables, so a closure captures its own. Delete `v := v`. It does nothing now.
  ([Go 1.22 § Language](https://go.dev/doc/go1.22#language))
- **Range over an integer** (1.22). `for i := range n` instead of
  `for i := 0; i < n; i++`. ([Go 1.22 § Language](https://go.dev/doc/go1.22#language))
- **Range over functions** (1.23). `for` can range over `func(yield func(K, V) bool)` and
  its one- and zero-argument forms. The `iter` package names them `iter.Seq[V]` and
  `iter.Seq2[K, V]`. ([Go 1.23 § Language](https://go.dev/doc/go1.23#language),
  [Go 1.23 § Iterators](https://go.dev/doc/go1.23#iterators))
- **Generic type aliases** (1.24). A type alias can take type parameters.
  ([Go 1.24 § Language](https://go.dev/doc/go1.24#language))
- **`new(expr)`** (1.26). `new` takes an expression as the initial value, which fills an
  optional pointer field in one step: `Age: new(yearsSince(born))`.
  ([Go 1.26 § Language](https://go.dev/doc/go1.26#language))
- **Generic methods** (1.27). A method may declare its own type parameters. Interface
  methods may not, and a generic method can't implement an interface method.
  ([Go 1.27 § Language](https://go.dev/doc/go1.27#language))
- **Field selectors in struct literal keys** (1.27). A key may be any valid selector for
  the struct, not only a top-level field name.
  ([Go 1.27 § Language](https://go.dev/doc/go1.27#language))

A new language feature is a tool, not an obligation. Generics in particular stay subject to
"least mechanism". ([Decisions § Generics](https://google.github.io/styleguide/go/decisions#generics),
[Guide § Least mechanism](https://google.github.io/styleguide/go/guide#least-mechanism))

## Standard library

### slices, maps, iterators

- `slices` and `maps` (1.21) hold the generic slice and map operations that used to be
  hand-written loops or `golang.org/x/exp`. Use `slices.Contains`, `slices.Index`,
  `slices.Sort`, `slices.Equal`, `maps.Equal` and friends.
  ([Go 1.21 § slices](https://go.dev/doc/go1.21#slices),
  [Go 1.21 § maps](https://go.dev/doc/go1.21#maps))
- Since 1.23 both packages speak iterators. `slices.All`, `slices.Values`,
  `slices.Backward`, `maps.Keys`, `maps.Values` produce them. `slices.Collect`,
  `slices.Sorted` and `maps.Collect` consume them. Sorted map keys are
  `slices.Sorted(maps.Keys(m))`. `maps.Keys` returns an iterator, not a slice.
  ([Go 1.23 § Iterators](https://go.dev/doc/go1.23#iterators))
- `strings.Lines`, `strings.SplitSeq` and `strings.FieldsSeq` (1.24) iterate without
  building a slice. ([Go 1.24 § strings](https://go.dev/doc/go1.24#stringspkgstrings))
- `strings.CutLast` (1.27) splits around the last separator, where `LastIndex` plus slicing
  used to go. ([Go 1.27 § strings](https://go.dev/doc/go1.27#stringspkgstrings))

brama already uses `slices` (`slices.Contains`, `slices.Clone` in
`internal/generator/generator.go`). It has no iterators or generics yet.

### errors

- `errors.Is` and `errors.As` (1.13) look through `%w` wrapping. Use them instead of `==`
  and type assertions. ([Go 1.13 § Error wrapping](https://go.dev/doc/go1.13#error_wrapping))
- `errors.Join` and multiple `%w` (1.20) wrap several errors at once.
  ([Go 1.20 § Wrapping multiple errors](https://go.dev/doc/go1.20#errors))
- `errors.AsType[T]` (1.26) is the generic, type-safe and faster form of `errors.As`:
  `if pe, ok := errors.AsType[*fs.PathError](err); ok { ... }`.
  ([Go 1.26 § errors](https://go.dev/doc/go1.26#errorspkgerrors))

### context

`context.WithoutCancel`, `context.AfterFunc`, `context.WithDeadlineCause` and
`context.WithTimeoutCause` (1.21). See `context.md`.
([Go 1.21 § context](https://go.dev/doc/go1.21#contextpkgcontext))

### sync

`sync.WaitGroup.Go` (1.25) starts and counts a goroutine in one call.
([Go 1.25 § sync](https://go.dev/doc/go1.25#syncpkgsync))

### log/slog

`log/slog` (1.21) is structured, levelled logging in the standard library.
([Go 1.21 § log/slog](https://go.dev/doc/go1.21#slog))
brama has no logger. See `concurrency.md` § Logging before adding one.

### Filesystem, JSON and others

- `os.Root` (1.24) confines file operations to one directory. Paths that escape it,
  symlinks included, are refused.
  ([Go 1.24 § Directory-limited filesystem access](https://go.dev/doc/go1.24#directory-limited-filesystem-access))
- `encoding/json`'s `omitzero` tag option (1.24) omits zero values, including a zero
  `time.Time`, which `omitempty` never did.
  ([Go 1.24 § encoding/json](https://go.dev/doc/go1.24#encodingjsonpkgencodingjson))
- `encoding/json/v2` and `encoding/json/jsontext` (1.27). `encoding/json` itself now runs
  on the v2 implementation with v1 behaviour, but the exact text of its error messages may
  differ. Don't assert on them.
  ([Go 1.27 § encoding/json/v2](https://go.dev/doc/go1.27#jsonv2))
- `uuid` (1.27) generates and parses UUIDs.
  ([Go 1.27 § uuid](https://go.dev/doc/go1.27#uuid))

## Testing

- `t.Context()` (1.24): the test's context, cancelled before cleanup.
  `t.Chdir` (1.24): change directory for the test, restored afterwards.
  ([Go 1.24 § testing](https://go.dev/doc/go1.24#testingpkgtesting))
- `for b.Loop() { ... }` (1.24) replaces `for range b.N`. Since 1.26 it no longer blocks
  inlining. ([Go 1.24 § New benchmark function](https://go.dev/doc/go1.24#new-benchmark-function),
  [Go 1.26 § testing](https://go.dev/doc/go1.26#testingpkgtesting))
- `testing/synctest` (generally available in 1.25): `synctest.Test` runs a test on a fake
  clock that jumps whenever everything is blocked, and `synctest.Wait` waits for that.
  `synctest.Sleep` (1.27) combines the two. Don't use the 1.24 experimental `synctest.Run`.
  ([Go 1.25 § testing/synctest](https://go.dev/doc/go1.25#new-testingsynctest-package),
  [Go 1.27 § testing/synctest](https://go.dev/doc/go1.27#testingsynctestpkgtestingsynctest))
- `T.ArtifactDir` (1.26) gives a directory for test output files, kept when `go test` runs
  with `-artifacts`. ([Go 1.26 § testing](https://go.dev/doc/go1.26#testingpkgtesting))

## Tooling

- `go fix` (rewritten in 1.26) applies modernizers that rewrite code to newer idioms and
  library APIs without changing behaviour. It is built on the same analysis framework as
  `go vet`. Go 1.27 added more.
  ([Go 1.26 § Go command](https://go.dev/doc/go1.26#go-command),
  [Go 1.27 § go fix](https://go.dev/doc/go1.27#go-fix))
  Running it across brama is a change of its own, not something to do alongside a feature.
- `go vet` gained `waitgroup` (misplaced `WaitGroup.Add`) in 1.25.
  ([Go 1.25 § Vet](https://go.dev/doc/go1.25#vet))
