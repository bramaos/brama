# Context

Depth behind the Context section of `docs/agents/go.md`. Our prose, not the upstream text.
Every claim links the section it came from. Where this file and the code disagree, the
code wins (see the Priority section of `go.md`).

## It is passed, and it comes first

A `context.Context` carries deadlines, cancellation and request-scoped credentials down the
call chain. Go passes it explicitly, as the first parameter of every function that needs it.
([Decisions § Contexts](https://google.github.io/styleguide/go/decisions#contexts),
[Code Review Comments § Contexts](https://go.dev/wiki/CodeReviewComments#contexts))

The first-parameter rule holds for test helpers too: the context goes first, then `t`.
([Decisions § Contexts](https://google.github.io/styleguide/go/decisions#contexts),
[Decisions § Test helpers](https://google.github.io/styleguide/go/decisions#test-helpers))

It is immutable, so one context can be passed to several calls that share its deadline and
cancellation. ([Decisions § Contexts](https://google.github.io/styleguide/go/decisions#contexts))

## Where a context comes from

Only an entrypoint makes a root context: `main` with `context.Background()`, a test with
`t.Context()`, an HTTP handler from `req.Context()`. Code in the middle of a call chain
should almost never make its own. It takes the caller's, unless the caller's is the wrong
one. ([Decisions § Contexts](https://google.github.io/styleguide/go/decisions#contexts))

When unsure whether a function needs a context, take one. The default is to pass it.
([Code Review Comments § Contexts](https://go.dev/wiki/CodeReviewComments#contexts))

In tests, `t.Context()` (Go 1.24) is the starting context. It is cancelled just before the
test's `Cleanup` functions run, so cleanup can wait on work that stops when the context is
done. ([Go 1.24 § testing](https://go.dev/doc/go1.24#testingpkgtesting),
[testing § T.Context](https://pkg.go.dev/testing#T.Context),
[Decisions § Contexts](https://google.github.io/styleguide/go/decisions#contexts))

In brama the root is made once, where the command tree runs (`internal/cli/cli.go`), and
`brama server add` derives an interrupt-aware one with `signal.NotifyContext`
(`internal/cli/server_add.go`). Most of the test suite uses `t.Context()`.

## It is never stored

Don't put a context in a struct field. Give each method that needs one a `ctx` parameter.
The one exception is a method whose signature must match an interface you do not control.
([Decisions § Contexts](https://google.github.io/styleguide/go/decisions#contexts),
[Code Review Comments § Contexts](https://go.dev/wiki/CodeReviewComments#contexts))

## No custom context types

Don't define your own context type, and don't take any interface other than
`context.Context` where a context belongs. If every package had its own, every call across
a package boundary would need a conversion.
([Decisions § Custom contexts](https://google.github.io/styleguide/go/decisions#custom-contexts))

Application data goes in a parameter, the receiver, or a global. It goes in a context value
only if it truly belongs to the request.
([Decisions § Custom contexts](https://google.github.io/styleguide/go/decisions#custom-contexts),
[Code Review Comments § Contexts](https://go.dev/wiki/CodeReviewComments#contexts))

## Cancellation, errors and docs

A function that takes a context should usually return an `error`, so the caller can tell
the context was cancelled while it ran.
([Decisions § Returning errors](https://google.github.io/styleguide/go/decisions#returning-errors))

Cancellation interrupting the function is assumed, and the error it returns is
conventionally `ctx.Err()`. Don't restate that in the doc comment. Document the exceptions:
a different error on cancellation, another way to stop the function, or a requirement on
the context such as "must not have a deadline". Requirements like the last one are worth
avoiding in the first place.
([Best Practices § Contexts](https://google.github.io/styleguide/go/best-practices#contexts))

The cancel function returned by `WithCancel`, `WithTimeout` and `WithDeadline` must be
called when the context is no longer needed, or it leaks.
([Decisions § Named result parameters](https://google.github.io/styleguide/go/decisions#named-result-parameters))
`go vet`, which `make vet` runs, reports a cancel that is not called on every path
([lostcancel](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/lostcancel#pkg-overview)).

## Shadowing a context

`ctx, cancel := context.WithTimeout(ctx, d)` inside an `if` block declares a new `ctx` that
vanishes at the closing brace. Code after the block still sees the caller's context, with
no deadline. Declare `cancel` first and assign with `=`.
([Best Practices § Shadowing](https://google.github.io/styleguide/go/best-practices#shadowing))

## Since Go 1.21

- `context.WithoutCancel` returns a copy that is not cancelled when its parent is. It is
  the standard library's answer to detaching background work from a request.
- `context.AfterFunc` runs a function once a context is cancelled.
- `context.WithDeadlineCause` and `WithTimeoutCause` record why a deadline fired.
  `context.Cause` reads it back.

([Go 1.21 § context](https://go.dev/doc/go1.21#contextpkgcontext))
