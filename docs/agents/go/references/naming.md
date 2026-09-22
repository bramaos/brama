# Naming

Depth behind the Naming section of `docs/agents/go.md`. Our prose, not the upstream text.
Every claim links the section it came from. Where this file and the code disagree, the
code wins (see the Priority section of `go.md`).

## The principle

A name should not feel repetitive where it is used. It should take its context into account
and not repeat what that context already says. Go names are shorter than in many languages,
because the package, the receiver and the type already say a lot.
([Guide § Naming](https://google.github.io/styleguide/go/guide#naming),
[Effective Go § Names](https://go.dev/doc/effective_go#names))

## Case

Multi-word names are MixedCaps or mixedCaps, never snake_case, even for constants
(`MaxLength`, not `MAX_LENGTH`).
([Guide § MixedCaps](https://google.github.io/styleguide/go/guide#mixedcaps),
[Effective Go § MixedCaps](https://go.dev/doc/effective_go#mixed-caps),
[Decisions § Constant names](https://google.github.io/styleguide/go/decisions#constant-names))

Underscores appear only in `Test`/`Benchmark`/`Example` function names, in packages only
generated code imports, and in low-level OS interop. File names are not identifiers and may
use them. ([Decisions § Underscores](https://google.github.io/styleguide/go/decisions#underscores))

An initialism keeps one case throughout: `URL` or `url`, `ID` or `id`, never `Url` or
`Id`. So `userID` and `ServeHTTP`. With several initialisms, each keeps its own case
(`xmlAPI`). One with a lowercase letter in its usual spelling keeps it unless it starts an
exported name (`gRPC` → `GRPC`).
([Decisions § Initialisms](https://google.github.io/styleguide/go/decisions#initialisms),
[Code Review Comments § Initialisms](https://go.dev/wiki/CodeReviewComments#initialisms))

## Repetition

The package name is always visible at the call site, so don't repeat it:
`widget.New`, not `widget.NewWidget`. The same goes for the receiver's type in a method
name (`c.WriteTo`, not `c.WriteConfigTo`), for parameter names, and for the result type.
([Decisions § Package vs. exported symbol name](https://google.github.io/styleguide/go/decisions#package-vs-exported-symbol-name),
[Best Practices § Avoid repetition](https://google.github.io/styleguide/go/best-practices#avoid-repetition),
[Code Review Comments § Package names](https://go.dev/wiki/CodeReviewComments#package-names))

If a package exports one main type named after the package, its constructor is `New`.
([Decisions § Package vs. exported symbol name](https://google.github.io/styleguide/go/decisions#package-vs-exported-symbol-name),
[Effective Go § Package names](https://go.dev/doc/effective_go#package-names))

Leave the type out of a variable's name (`users`, not `userSlice`) unless one value exists
in two forms in the same scope (`limitRaw`, `limit`). Leave out what the surrounding
function or type already says.
([Decisions § Variable name vs. type](https://google.github.io/styleguide/go/decisions#variable-name-vs-type),
[Decisions § External context vs. local names](https://google.github.io/styleguide/go/decisions#external-context-vs-local-names))

## Functions and methods

A function that returns something gets a noun-like name. A function that does something
gets a verb-like one. ([Best Practices § Naming conventions](https://google.github.io/styleguide/go/best-practices#naming-conventions))

No `Get` prefix: the getter for `owner` is `Owner`, and its setter is `SetOwner`. The
exception is when the concept itself is "get", as in HTTP GET. If the call is expensive or
remote, a verb like `Fetch` or `Compute` warns the caller that it might block or fail.
([Decisions § Getters](https://google.github.io/styleguide/go/decisions#getters),
[Effective Go § Getters](https://go.dev/doc/effective_go#Getters))

Functions that differ only by the type they handle put the type at the end
(`ParseInt`, `ParseInt64`).
([Best Practices § Naming conventions](https://google.github.io/styleguide/go/best-practices#naming-conventions))

## Variables

Length follows scope: a name used far from its declaration needs more words. Very roughly,
a scope of up to 7 lines is small, 8–15 medium, 15–25 large, and anything beyond a page very
large. ([Decisions § Variable names](https://google.github.io/styleguide/go/decisions#variable-names),
[Code Review Comments § Variable names](https://go.dev/wiki/CodeReviewComments#variable-names))

Single letters suit loop indices, receivers, and familiar types (`r` for a reader, `w` for a
writer). Elsewhere, use the full word. Don't drop letters to save typing.
([Decisions § Single-letter variable names](https://google.github.io/styleguide/go/decisions#single-letter-variable-names),
[Decisions § Variable names](https://google.github.io/styleguide/go/decisions#variable-names))

Name a local for what it holds here, not for where it came from.
([Decisions § Variable names](https://google.github.io/styleguide/go/decisions#variable-names))

Don't shadow a package name you still need in that scope (`url := ...` hides `net/url`).
Pick package names that won't collide with good variable names.
([Best Practices § Shadowing](https://google.github.io/styleguide/go/best-practices#shadowing))

## Receivers

One or two letters, an abbreviation of the type, the same in every method of that type.
Never `this`, `self` or `me`. Omit the name if the method doesn't use it. In brama, revive
reports both a generic name and a receiver named differently across a type's methods.
([Decisions § Receiver names](https://google.github.io/styleguide/go/decisions#receiver-names),
[Code Review Comments § Receiver names](https://go.dev/wiki/CodeReviewComments#receiver-names))

## Constants

Name a constant for its role, not its value (`MaxPacketSize`, not `Twelve`). A value with
no role besides itself doesn't need a constant.
([Decisions § Constant names](https://google.github.io/styleguide/go/decisions#constant-names))

## Result parameters

Name results when two share a type (`left, right *Node`) or when the caller must act on
one (`cancel func()`). Don't name them only to allow a naked return, and use naked returns
only in very short functions.
([Decisions § Named result parameters](https://google.github.io/styleguide/go/decisions#named-result-parameters),
[Code Review Comments § Named result parameters](https://go.dev/wiki/CodeReviewComments#named-result-parameters))
In brama, `nakedret` reports a naked return in a function longer than 30 lines.

## Domain names

Identifiers that name a domain concept use `CONTEXT.md`'s term. The `_Avoid_` list under
each term applies to code as much as to prose. This is brama's rule
(`docs/agents/domain.md`), not Go's.
