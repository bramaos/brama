# Packages and consistency

Depth behind the Consistency first and Packages sections of `docs/agents/go.md`. Our prose,
not the upstream text. Every claim links the section it came from. Where this file and the
code disagree, the code wins (see the Priority section of `go.md`).

## Consistency and its limit

Google ranks readable code's qualities in order: clarity, simplicity, concision,
maintainability, consistency. Consistency is last, but it breaks ties. Consistency within a
package matters most in practice, because one problem solved two ways in one place is
jarring. ([Guide § Style principles](https://google.github.io/styleguide/go/guide#style-principles),
[Guide § Consistency](https://google.github.io/styleguide/go/guide#consistency))

Where the style guide is silent, match the code nearby, usually the same file or package.
Local consistency is not a license, though. A change that would spread a deviation to more
files, expose it in more API, or introduce a bug should fix the deviation, or at least not
make it worse. ([Guide § Local consistency](https://google.github.io/styleguide/go/guide#local-consistency))
In brama's precedence chain, existing code outranks `go.md` for exactly this reason. The
limit above is how a deviation stops spreading.

## Least mechanism

When there are several ways to do something, use the most ordinary one: a core language
construct first, then the standard library, then an existing dependency. Only then consider
a new one. Complexity is easy to add and hard to remove.
([Guide § Least mechanism](https://google.github.io/styleguide/go/guide#least-mechanism),
[Guide § Simplicity](https://google.github.io/styleguide/go/guide#simplicity))
A `map[string]bool` is usually enough for set membership.
([Guide § Least mechanism](https://google.github.io/styleguide/go/guide#least-mechanism))

Maintainable code minimises its dependencies, implicit and explicit.
([Guide § Maintainability](https://google.github.io/styleguide/go/guide#maintainability))

## Comments that explain why

The code shows what it does. Comments should say why, especially where a reader could
miss a subtlety of the language or the domain. A comment that restates the code, or
contradicts it, makes things less clear.
([Guide § Clarity](https://google.github.io/styleguide/go/guide#clarity),
[Guide § Why is the code doing what it does?](https://google.github.io/styleguide/go/guide#why-is-the-code-doing-what-it-does))

## Sizing a package

A package groups what its users need together. If a user must import two packages to do
anything useful with either, they are probably one package. Code in one package can share
unexported details without making them public API.
([Best Practices § Package size](https://google.github.io/styleguide/go/best-practices#package-size))

The other extreme is also wrong. One package for the whole project is too big, and a
conceptually distinct idea deserves its own small package, whose name and exported names
read well together (`bytes.Buffer`, `ring.New`).
([Best Practices § Package size](https://google.github.io/styleguide/go/best-practices#package-size))

Go has no one-type-per-file convention. Split files so a maintainer can guess which file
holds something. Avoid both a file of many thousands of lines and a scatter of tiny files.
([Best Practices § Package size](https://google.github.io/styleguide/go/best-practices#package-size))

A dependency cycle broken by an interface usually signals that the packages are split in
the wrong place. ([Best Practices § Avoid unnecessary interfaces](https://google.github.io/styleguide/go/best-practices#avoid-unnecessary-interfaces))

## Naming a package

Short, lowercase, one word where possible, no underscores or mixedCaps. The name is the
base name of its directory.
([Effective Go § Package names](https://go.dev/doc/effective_go#package-names),
[Decisions § Package names](https://google.github.io/styleguide/go/decisions#package-names))

Name it for what it provides. `util`, `common`, `helper`, `misc`, `model`, `types` tell the
call site nothing and invite import renames.
([Best Practices § Util packages](https://google.github.io/styleguide/go/best-practices#util-packages),
[Code Review Comments § Package names](https://go.dev/wiki/CodeReviewComments#package-names),
[Decisions § Package names](https://google.github.io/styleguide/go/decisions#package-names))

Avoid a name a caller will want for a local variable (`count`). Choose `usercount`.
([Decisions § Package names](https://google.github.io/styleguide/go/decisions#package-names))

## Global state

Don't make callers depend on package-level state. A package with state lets callers create
instances and pass them explicitly. Global registries, service locators and lazily created
singleton clients all make tests depend on order and on each other.
([Best Practices § Global state](https://google.github.io/styleguide/go/best-practices#global-state),
[Best Practices § Major forms of package state APIs](https://google.github.io/styleguide/go/best-practices#major-forms-of-package-state-apis))

Package-level state is tolerable when it is logically constant, when behaviour is
observably stateless (a private cache), or when it doesn't leak outside the process.
([Best Practices § Litmus tests](https://google.github.io/styleguide/go/best-practices#litmus-tests))

## Imports

Group imports: the standard library first, then everything else. `goimports`, run by
`make lint`, keeps the grouping.
([Decisions § Import grouping](https://google.github.io/styleguide/go/decisions#import-grouping))

Rename an import only to resolve a collision or replace a useless name, and rename the more
local one. Use the same local name everywhere.
([Decisions § Import renaming](https://google.github.io/styleguide/go/decisions#import-renaming))

Blank imports (`import _ "pkg"`) belong in `main` or in tests. The exception is `embed`
beside a `//go:embed` directive.
([Decisions § Import "blank"](https://google.github.io/styleguide/go/decisions#import-blank-import-_))
Don't use dot imports.
([Decisions § Import "dot"](https://google.github.io/styleguide/go/decisions#import-dot-import-),
[Code Review Comments § Import dot](https://go.dev/wiki/CodeReviewComments#import-dot))

## Package comments

Exactly one file per package carries the package comment, directly above the `package`
clause with no blank line. A long package comment may get its own `doc.go`.
([Decisions § Package comments](https://google.github.io/styleguide/go/decisions#package-comments),
[Best Practices § Package size](https://google.github.io/styleguide/go/best-practices#package-size))

## brama's layout

- `cmd/brama`, `cmd/brama-shim`: the two binaries.
- `internal/…`: everything else, one package per domain concept (`config`, `schema`,
  `adapter`, `preset`, `refusal`, …).
- `test/testenv`: tests against containerised servers, behind `//go:build testenv`, run by
  `make testenv-test` (see `docs/adr/0008-test-against-containerised-servers.md`).

The domain vocabulary for package and type names is `CONTEXT.md`.
