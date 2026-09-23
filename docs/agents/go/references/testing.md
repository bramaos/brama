# Testing

Depth behind the Testing section of `docs/agents/go.md`. Our prose, not the upstream text.
Every claim links the section it came from. Where this file and the code disagree, the
code wins (see the Priority section of `go.md`).

brama's `.golangci.yml` sets `run.tests: false`. Nothing below is checked by a linter.

## The standard library, and nothing else

Use the `testing` package. Don't write or import assertion libraries. They either stop at
the first failure or drop the context a good failure message needs, and a fleet of them
fragments how tests read. Write the check in Go.
([Decisions § Assertion libraries](https://google.github.io/styleguide/go/decisions#assertion-libraries),
[Decisions § Use package testing](https://google.github.io/styleguide/go/decisions#use-package-testing),
[Go Test Comments § Assert libraries](https://go.dev/wiki/TestComments#assert-libraries))

Google's own examples compare with `github.com/google/go-cmp`. brama doesn't depend on it.
Its tests compare with `==`, `slices.Equal`, `maps.Equal` and occasionally
`reflect.DeepEqual`, and adding `cmp` would be a new dependency to discuss first.
([Decisions § Equality comparison and diffs](https://google.github.io/styleguide/go/decisions#equality-comparison-and-diffs))
Google advises against `reflect.DeepEqual` for new code because it compares unexported
fields and is sensitive to implementation details.
([Decisions § Equality comparison and diffs](https://google.github.io/styleguide/go/decisions#equality-comparison-and-diffs))
brama has a few (`test/testenv/schema_postgres_test.go`). Don't spread it where `==` or
`slices.Equal` would do.

## Failure messages

Someone should be able to diagnose a failure without reading the test. Say which function
failed, with what input, what it returned, and what was wanted, in that order:
`Func(%v) = %v, want %v`. Use "got" and "want", not "actual" and "expected".
([Decisions § Useful test failures](https://google.github.io/styleguide/go/decisions#useful-test-failures),
[Decisions § Identify the function](https://google.github.io/styleguide/go/decisions#identify-the-function),
[Decisions § Identify the input](https://google.github.io/styleguide/go/decisions#identify-the-input),
[Decisions § Got before want](https://google.github.io/styleguide/go/decisions#got-before-want),
[Code Review Comments § Useful test failures](https://go.dev/wiki/CodeReviewComments#useful-test-failures))

`%q` makes string values stand out, and `%+v` names a small struct's fields. When output is
large, print a diff and label its direction, such as `(-want +got)`.
([Decisions § Level of detail](https://google.github.io/styleguide/go/decisions#level-of-detail),
[Decisions § Print diffs](https://google.github.io/styleguide/go/decisions#print-diffs))

Compare whole structures rather than field by field, unless irrelevant fields would hide
the point of the test. Don't compare output you don't own the stability of, such as
`json.Marshal` bytes. Parse it and compare meaning.
([Decisions § Full structure comparisons](https://google.github.io/styleguide/go/decisions#full-structure-comparisons),
[Decisions § Compare stable results](https://google.github.io/styleguide/go/decisions#compare-stable-results))

## Error or Fatal

Keep going: report a mismatch with `t.Error` so one run shows every broken check. Use
`t.Fatal` when continuing would be meaningless or misleading: setup failed, or an error
came back where a value was needed.
([Decisions § Keep going](https://google.github.io/styleguide/go/decisions#keep-going),
[Best Practices § t.Error vs. t.Fatal](https://google.github.io/styleguide/go/best-practices#terror-vs-tfatal))

Inside a `t.Run` subtest, `t.Fatal` ends only that case. In a plain loop over a table, use
`t.Error` and `continue`.
([Best Practices § t.Error vs. t.Fatal](https://google.github.io/styleguide/go/best-practices#terror-vs-tfatal))

`t.Fatal` / `t.FailNow` must be called on the test's own goroutine.
([Best Practices § Don't call t.Fatal from separate goroutines](https://google.github.io/styleguide/go/best-practices#dont-call-tfatal-from-separate-goroutines),
[testing § T.FailNow](https://pkg.go.dev/testing#T.FailNow))

## Errors under test

Don't check which error you got by comparing its text. The wording will change, and the
test becomes a change detector. Use `errors.Is` or `errors.As`. If you only care that some
error occurred, a `wantErr bool` is enough.
([Decisions § Test error semantics](https://google.github.io/styleguide/go/decisions#test-error-semantics),
[Go Test Comments § Test error semantics](https://go.dev/wiki/TestComments#test-error-semantics))

Checking that a message from the package under test has some property, such as naming a
parameter, is allowed.
([Decisions § Test error semantics](https://google.github.io/styleguide/go/decisions#test-error-semantics))
brama relies on this. Its errors and refusals are read by people, so many tests check that
a message names the column, the file or the command that fixes it. That is testing
behaviour, not identity.

## Table-driven tests and subtests

Use a table when many cases share one piece of checking logic. When some cases need
different logic, write separate test functions rather than branching inside the loop.
([Decisions § Table-driven tests](https://google.github.io/styleguide/go/decisions#table-driven-tests),
[Decisions § Data-driven test cases](https://google.github.io/styleguide/go/decisions#data-driven-test-cases),
[Go Test Comments § Table-driven tests vs multiple test functions](https://go.dev/wiki/TestComments#table-driven-tests-vs-multiple-test-functions))

Identify a failing row by its name or its input, never by its index.
([Decisions § Identifying the row](https://google.github.io/styleguide/go/decisions#identifying-the-row))
Name the fields in case literals once the table is long, or when adjacent fields share a
type. ([Best Practices § Use field names in struct literals](https://google.github.io/styleguide/go/best-practices#use-field-names-in-struct-literals))

Subtests must not depend on each other, so any one can run alone with `-run`. Subtest names
should be readable in output and easy to type on the command line. The runner turns spaces
into underscores, and slashes clash with `-run` patterns.
([Decisions § Subtests](https://google.github.io/styleguide/go/decisions#subtests),
[Decisions § Subtest names](https://google.github.io/styleguide/go/decisions#subtest-names),
[Go Test Comments § Choose human-readable subtest names](https://go.dev/wiki/TestComments#choose-human-readable-subtest-names))
brama's subtest names are either a table row's own name (`internal/…`) or a short phrase
with spaces ("generated columns", `test/testenv/schema_test.go`). Keep to that, and keep
slashes out.

## Helpers

A test helper does setup or cleanup. A failure inside it is a failure of the environment,
not of the code under test. It calls `t.Helper()` so the failure is reported at the
caller's line. It takes `t` after any context and before everything else, and fails with
`t.Fatalf` and a message saying which step broke.
([Decisions § Test helpers](https://google.github.io/styleguide/go/decisions#test-helpers),
[Best Practices § Error handling in test helpers](https://google.github.io/styleguide/go/best-practices#error-handling-in-test-helpers),
[Go Test Comments § Mark test helpers](https://go.dev/wiki/TestComments#mark-test-helpers))

An assertion helper that checks results and fails the test is not idiomatic. Keep the
check in the `Test` function. If several tests share validation logic, have it return an
error or a value, and let the test decide how to fail.
([Best Practices § Leave testing to the Test function](https://google.github.io/styleguide/go/best-practices#leave-testing-to-the-test-function))

A helper that stops the test on failure may be named `mustX`, the test-side counterpart of
`MustX`. ([Decisions § Must functions](https://google.github.io/styleguide/go/decisions#must-functions))
brama's `mustParse`, `mustLookup` and `mustContain` follow this
(`internal/config/anonymize_test.go`, `internal/config/edit_test.go`,
`internal/preset/preset_test.go`).

## Hermetic setup

Scope setup to the tests that need it. Loading shared data in `init` or a package variable
makes every test pay for it and couples tests together.
([Best Practices § Keep setup code scoped to specific tests](https://google.github.io/styleguide/go/best-practices#keep-setup-code-scoped-to-specific-tests))
A custom `TestMain` is a last resort, for expensive setup that every test needs and that
must be torn down.
([Best Practices § When to use a custom TestMain entrypoint](https://google.github.io/styleguide/go/best-practices#when-to-use-a-custom-testmain-entrypoint))

The `testing` package owns cleanup for you. `t.TempDir()` removes its directory,
`t.Setenv` and `t.Chdir` (Go 1.24) restore the environment and working directory, and
`t.Cleanup` runs registered functions when the test ends.
([testing § T.TempDir](https://pkg.go.dev/testing#T.TempDir),
[testing § T.Setenv](https://pkg.go.dev/testing#T.Setenv),
[testing § T.Chdir](https://pkg.go.dev/testing#T.Chdir),
[testing § T.Cleanup](https://pkg.go.dev/testing#T.Cleanup))

`t.Context()` (Go 1.24) gives the test a context that is cancelled just before cleanup
runs. ([testing § T.Context](https://pkg.go.dev/testing#T.Context),
[Go 1.24 § testing](https://go.dev/doc/go1.24#testingpkgtesting))

## Parallelism

`t.Setenv` and `t.Chdir` change the whole process, so neither can be used in a parallel
test or under a parallel parent.
([testing § T.Setenv](https://pkg.go.dev/testing#T.Setenv),
[testing § T.Chdir](https://pkg.go.dev/testing#T.Chdir))
brama's suite is sequential: no test calls `t.Parallel()`. Keeping it that way keeps
`t.Setenv` and `t.Chdir` available to any test that needs them.
([testing § T.Parallel](https://pkg.go.dev/testing#T.Parallel))

## Same package or `_test`

A test in the same package (`package foo`) can reach unexported identifiers. A test in
`package foo_test` exercises only the exported API, and is needed when the test would
otherwise create an import cycle.
([Decisions § Tests in the same package](https://google.github.io/styleguide/go/decisions#tests-in-the-same-package),
[Decisions § Tests in a different package](https://google.github.io/styleguide/go/decisions#tests-in-a-different-package))
brama uses both: `internal/cli` tests are in-package, `internal/preset` tests are
`preset_test`, and `internal/config` mixes them. A new file joins the clause its package's
existing tests use.

## Benchmarks

Write the loop as `for b.Loop() { ... }` (Go 1.24). Setup outside it runs once per
`-count`, and the compiler can't optimise the measured call away. Since Go 1.26 it no
longer blocks inlining in the loop body.
([Go 1.24 § New benchmark function](https://go.dev/doc/go1.24#new-benchmark-function),
[Go 1.26 § testing](https://go.dev/doc/go1.26#testingpkgtesting))
`make bench` runs them.

## Integration tests

Tests that need real servers live in `test/testenv`, behind `//go:build testenv`, and run
with `make testenv-test` against the containers `make testenv-up` starts. `make test` and
`make check` never compile them.
(`Makefile`, `docs/adr/0008-test-against-containerised-servers.md`)
