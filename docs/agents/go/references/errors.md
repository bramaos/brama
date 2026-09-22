# Errors

Depth behind the Errors section of `docs/agents/go.md`. Our prose, not the upstream text.
Every claim links the section it came from. Where this file and the code disagree, the
code wins (see the Priority section of `go.md`).

## Returning them

`error` is the last result. On failure, callers treat every other result as unspecified
unless the function documents otherwise.
([Decisions § Returning errors](https://google.github.io/styleguide/go/decisions#returning-errors))

Exported functions return the `error` interface, not a concrete error type. A nil
`*MyError` stored in an `error` is a non-nil interface, so `err != nil` holds when nothing
failed. ([Decisions § Returning errors](https://google.github.io/styleguide/go/decisions#returning-errors),
[Best Practices § Documentation: Errors](https://google.github.io/styleguide/go/best-practices#errors))

Don't signal failure in-band with `-1`, `""` or `nil`. Return an extra `bool` (no
explanation needed) or `error` (explanation needed) as the last result. A caller who
forgets to check an in-band value passes it on, and the failure then shows up in the wrong
function. ([Decisions § In-band errors](https://google.github.io/styleguide/go/decisions#in-band-errors),
[Effective Go § Multiple return values](https://go.dev/doc/effective_go#multiple-returns))

## Error strings

Lowercase unless they start with a proper noun, an acronym or an exported name, and no
closing punctuation. They are almost always printed inside other text.
([Decisions § Error strings](https://google.github.io/styleguide/go/decisions#error-strings),
[Code Review Comments § Error strings](https://go.dev/wiki/CodeReviewComments#error-strings))

Where it helps, an error string names where it came from, such as the operation or the
package. ([Effective Go § Errors](https://go.dev/doc/effective_go#errors))

## Handling them

Every error gets a deliberate choice: handle it, return it, or, rarely, stop the program.
Discarding one with `_` is the exception. Google asks for a comment saying why it is safe.
([Decisions § Handle errors](https://google.github.io/styleguide/go/decisions#handle-errors),
[Code Review Comments § Handle errors](https://go.dev/wiki/CodeReviewComments#handle-errors))
brama's existing `_ =` discards carry no such comment (`internal/ssh/ssh.go`,
`internal/cli/cli.go`). When you touch one, adding the
reason is welcome.

Deal with the error first and return, so the normal path stays unindented. Don't put the
happy path in an `else`.
([Decisions § Indent error flow](https://google.github.io/styleguide/go/decisions#indent-error-flow),
[Code Review Comments § Indent error flow](https://go.dev/wiki/CodeReviewComments#indent-error-flow))

If you return an error, don't also log it. The caller decides whether it is shown, and
reporting it twice is noise.
([Best Practices § Logging errors](https://google.github.io/styleguide/go/best-practices#logging-errors))
brama has no logger (no package imports `log` or `log/slog`). Errors reach the user
through the command layer in `internal/cli/cli.go`.

When several operations run together and only the first failure matters, `errgroup`
collects it and cancels the rest.
([Best Practices § Error handling](https://google.github.io/styleguide/go/best-practices#error-handling))

## Structure: sentinels and types

If a caller needs to tell failures apart, give the error structure so it can check in code.
Never make it match on the message.
([Best Practices § Error structure](https://google.github.io/styleguide/go/best-practices#error-structure))

- A **sentinel** (`var ErrX = errors.New(...)`) is enough when the condition carries no
  data. Callers compare with `errors.Is`, which also sees through wrapping.
- An **error type** is for when the caller needs fields, as `os.PathError` carries the
  path. Callers extract it with `errors.As`, or with `errors.AsType` (Go 1.26), the typed
  and faster generic form.

([Best Practices § Error structure](https://google.github.io/styleguide/go/best-practices#error-structure),
[Effective Go § Errors](https://go.dev/doc/effective_go#errors),
[Go 1.13 § Error wrapping](https://go.dev/doc/go1.13#error_wrapping),
[Go 1.26 § errors](https://go.dev/doc/go1.26#errorspkgerrors))

Document the sentinels and types a function can return, and whether a type comes as a
pointer, so `errors.Is` and `errors.As` are used with the right target.
([Best Practices § Documentation: Errors](https://google.github.io/styleguide/go/best-practices#errors))

## Wrapping: what to add

Add what you know and the error does not. `os` errors already name the path, so repeating
the path adds only length. An annotation that just says "failed" adds nothing, and the
error should be returned as it is.
([Best Practices § Adding information to errors](https://google.github.io/styleguide/go/best-practices#adding-information-to-errors))
Some of brama's wraps name a path that `os` already names (`internal/config/load.go`).
Don't rewrite them in passing. In new code, prefer to add what the error lacks.

## Wrapping: `%w` or `%v`

- `%w` keeps the original in the chain, so callers can still use `errors.Is`/`errors.As`
  on it. That makes the wrapped error part of your API.
- `%v` flattens the original to text. Google uses it to annotate for humans, or to hide an
  implementation detail at a system boundary (RPC, storage) instead of exposing it.

([Best Practices § Adding information to errors](https://google.github.io/styleguide/go/best-practices#adding-information-to-errors),
[Go 1.13 § Error wrapping](https://go.dev/doc/go1.13#error_wrapping))

brama wraps with `%w` throughout (`internal/config/load.go` is typical). Keep doing so.
Reach for `%v` only when hiding the cause is itself the point, and say so in a comment.

Put `%w` at the end (`"reading %s: %w"`), so the printed text reads newest to oldest, the
same order as the chain. The exception is a sentinel that categorises the failure. That
one can lead (`"%w: invalid header"`), so the category is the first thing read.
([Best Practices § Placement of %w in errors](https://google.github.io/styleguide/go/best-practices#placement-of-w-in-errors),
[Best Practices § Sentinel error placement](https://google.github.io/styleguide/go/best-practices#sentinel-error-placement))
brama leads with sentinels this way (`internal/shim/shim.go`, `internal/config/edit.go`).
A few sites also lead with a plain cause and append detail after it
(`internal/ssh/ssh.go`, `internal/shim/ensure.go`). Leave those as they are, and don't copy
the shape into new code.

An error may wrap several errors: `fmt.Errorf` accepts more than one `%w`, and
`errors.Join` builds one from a list. `errors.Is` and `errors.As` search all of them.
([Go 1.20 § Wrapping multiple errors](https://go.dev/doc/go1.20#errors))

In brama, `wrapcheck` requires that an error from a package outside
`github.com/bramaos/brama/internal/...` be wrapped before it is returned. brama's own
packages are exempt, because their errors already read in brama's terms (`.golangci.yml`).

## Panics

Don't use `panic` for ordinary errors. Return an `error`.
([Decisions § Don't panic](https://google.github.io/styleguide/go/decisions#dont-panic),
[Code Review Comments § Don't panic](https://go.dev/wiki/CodeReviewComments#dont-panic))

Panics are for API misuse and broken invariants that tests should catch. A package may also
panic internally, as a parser might, if it always recovers before returning and never
lets a panic cross its API. Don't recover panics just to avoid a crash. The state
afterwards is unknown.
([Best Practices § When to panic](https://google.github.io/styleguide/go/best-practices#when-to-panic),
[Best Practices § Program checks and panics](https://google.github.io/styleguide/go/best-practices#program-checks-and-panics),
[Effective Go § Recover](https://go.dev/doc/effective_go#recover))

`MustX` helpers stop on failure. Call them only at program start, usually on constant
input (`regexp.MustCompile`), never on user input. The same shape is fine in test helpers
that call `t.Fatal`.
([Decisions § Must functions](https://google.github.io/styleguide/go/decisions#must-functions))
