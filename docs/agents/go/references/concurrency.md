# Concurrency

Depth behind the Concurrency section of `docs/agents/go.md`. Our prose, not the upstream
text. Every claim links the section it came from. Where this file and the code disagree,
the code wins (see the Priority section of `go.md`).

brama runs no goroutines today. Whoever adds the first ones sets the pattern the rest will
copy, so read this before writing them.

## Prefer synchronous APIs

A synchronous function returns its result and finishes any callbacks or channel work before
it returns. Prefer it. Goroutines stay inside one call, where their lifetime is easy to
see, and a test can pass input and check output without polling. A caller who wants
concurrency can run a synchronous function in a goroutine. Taking unwanted concurrency out
of an asynchronous API is hard, sometimes impossible.
([Decisions § Synchronous functions](https://google.github.io/styleguide/go/decisions#synchronous-functions),
[Code Review Comments § Synchronous functions](https://go.dev/wiki/CodeReviewComments#synchronous-functions))

## Every goroutine has a visible end

When you start a goroutine, make clear when it exits. A goroutine blocked on a channel is
never collected, even when nothing else can reach the channel. One left running after its
result stopped mattering can race on inputs the caller has moved on from, send on a closed
channel and panic, or hold memory without bound.
([Decisions § Goroutine lifetimes](https://google.github.io/styleguide/go/decisions#goroutine-lifetimes),
[Code Review Comments § Goroutine lifetimes](https://go.dev/wiki/CodeReviewComments#goroutine-lifetimes))

The usual shape keeps the synchronisation inside one function. It starts the goroutines,
passes them the context, and waits for all of them before it returns. If the lifetime still
isn't obvious from the code, document when and why the goroutines exit.
([Decisions § Goroutine lifetimes](https://google.github.io/styleguide/go/decisions#goroutine-lifetimes))

`sync.WaitGroup.Go` (Go 1.25) starts and counts a goroutine in one call, so there is no
`Add`/`Done` pair to get out of step.
([Go 1.25 § sync](https://go.dev/doc/go1.25#syncpkgsync))
`go vet` has reported a misplaced `WaitGroup.Add` since Go 1.25
([Go 1.25 § Vet](https://go.dev/doc/go1.25#vet)).

When several goroutines can fail and only the first failure matters, `errgroup` returns it
and cancels the rest through the context.
([Best Practices § Error handling](https://google.github.io/styleguide/go/best-practices#error-handling))
In brama, `golang.org/x/sync` is already an indirect dependency in `go.mod`. Using
`errgroup` makes it direct. It does not add a new module.

Since Go 1.22 each loop iteration has its own variables. A goroutine that closes over `v`
in `for _, v := range xs` sees its own `v`, and the old `v := v` copy is no longer needed.
([Go 1.22 § Language](https://go.dev/doc/go1.22#language),
[Go 1.22 § Vet: loop variables](https://go.dev/doc/go1.22#vet-loopclosure))

## Sharing

Go's default is to pass a value over a channel so that only one goroutine holds it at a
time: share memory by communicating, don't communicate by sharing memory. It is not a
dogma. A counter is better as an integer behind a mutex.
([Effective Go § Share by communicating](https://go.dev/doc/effective_go#sharing))

A buffered channel can bound how many goroutines do something at once. Gate the creation
of goroutines, not only the work inside them, or a burst of input starts unbounded
goroutines that then wait on the gate.
([Effective Go § Channels](https://go.dev/doc/effective_go#channels))

A goroutine stops when its context is cancelled: it selects on `ctx.Done()` rather than
polling a flag. ([Decisions § Goroutine lifetimes](https://google.github.io/styleguide/go/decisions#goroutine-lifetimes))

Guard shared state with a mutex, or don't share it. A struct that holds a `sync.Mutex` or
another type that must not be copied takes pointer receivers. ([Decisions § Receiver type](https://google.github.io/styleguide/go/decisions#receiver-type),
[Code Review Comments § Copying](https://go.dev/wiki/CodeReviewComments#copying))

When a function parameter is a channel, give it a direction (`<-chan T`, `chan<- T`), so the
signature says who sends and who receives.
([Best Practices § Channel direction](https://google.github.io/styleguide/go/best-practices#channel-direction))

## Documenting it

Readers assume read-only operations are safe for concurrent use and mutating ones are not.
Don't restate either. Document it when it's unclear whether an operation mutates (a cache
lookup that reorders an LRU list), when the API provides its own synchronisation, or when an
interface you consume must be safe for concurrent use.
([Best Practices § Documentation: Concurrency](https://google.github.io/styleguide/go/best-practices#concurrency))

## Testing it

`make test` runs `go test -race` (`Makefile`). A data race fails the suite and is not a warning
to ignore.

`t.Fatal` and `t.FailNow` must run on the test's own goroutine. From a goroutine the test
started, report with `t.Error` and return.
([Best Practices § Don't call t.Fatal from separate goroutines](https://google.github.io/styleguide/go/best-practices#dont-call-tfatal-from-separate-goroutines),
[testing § T.FailNow](https://pkg.go.dev/testing#T.FailNow))

`testing/synctest` (generally available in Go 1.25) runs a test in a bubble with a fake
clock. Time jumps forward whenever every goroutine in the bubble is blocked, and
`synctest.Wait` waits for them all to block. Code that waits on time can then be tested
without real sleeps. Use `synctest.Test`. The experimental Go 1.24 API (`synctest.Run`) was
kept only behind `GOEXPERIMENT=synctest`, and was scheduled for removal in Go 1.26.
([Go 1.25 § testing/synctest](https://go.dev/doc/go1.25#new-testingsynctest-package),
[Go 1.24 § testing/synctest](https://go.dev/doc/go1.24#testing-synctest))
Go 1.27 adds `synctest.Sleep`, which sleeps and then waits for the bubble to settle.
([Go 1.27 § testing/synctest](https://go.dev/doc/go1.27#testingsynctestpkgtestingsynctest))

## Logging

brama has no logger. `log/slog` (Go 1.21) is the standard library's structured logger
([Go 1.21 § log/slog](https://go.dev/doc/go1.21#slog)). Adding it would be a decision about
how brama reports progress, which goes through its output layer today. That is a
project-level choice, not a concurrency detail. Raise it rather than slipping it in.
