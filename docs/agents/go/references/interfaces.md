# Interfaces

Depth behind the Interfaces section of `docs/agents/go.md`. Our prose, not the upstream
text. Every claim links the section it came from. Where this file and the code disagree,
the code wins (see the Priority section of `go.md`).

## Don't create one until it is needed

The most common mistake with interfaces is writing one before anything needs it. A design
being called a "service" or "repository" is not a need. Start with the concrete type.
([Decisions § Interfaces](https://google.github.io/styleguide/go/decisions#interfaces),
[Best Practices § Avoid unnecessary interfaces](https://google.github.io/styleguide/go/best-practices#avoid-unnecessary-interfaces))

Without a real use, you can't tell whether an interface is needed, let alone which methods
it should have. ([Code Review Comments § Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces))

The needs that do justify one:

- two or more concrete types must go through the same code;
- a consumer uses one or two methods of a type with a large API, and wants to depend on
  only those;
- a dependency cycle between packages. Treat this one as a sign the packages are split
  wrong, and fix the split first if you can.

([Best Practices § Avoid unnecessary interfaces](https://google.github.io/styleguide/go/best-practices#avoid-unnecessary-interfaces),
[Best Practices § Package size](https://google.github.io/styleguide/go/best-practices#package-size))

An interface costs the reader. From a concrete type an editor jumps to the method and its
docs. From an interface it can only show the declaration.
([Guide § Maintainability](https://google.github.io/styleguide/go/guide#maintainability))

## Who defines it

The consumer defines the interface, in the package that uses it, with only the methods it
calls. The producer returns a concrete type, so it can add methods without breaking anyone.
([Decisions § Interfaces](https://google.github.io/styleguide/go/decisions#interfaces),
[Code Review Comments § Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces),
[Best Practices § Interface ownership and visibility](https://google.github.io/styleguide/go/best-practices#interface-ownership-and-visibility))

The producer defines it when the interface is the product: a protocol that several
implementations must follow, whose contract has to be written down in one place
(`io.Writer`, `hash.Hash`).
([Best Practices § Interface ownership and visibility](https://google.github.io/styleguide/go/best-practices#interface-ownership-and-visibility))
`schema.Introspector` is this case in brama. `internal/schema` states the contract, and the
MySQL and PostgreSQL packages under it implement it.

Keep an interface unexported if only its own package uses it. Exporting it promises to
maintain it. ([Best Practices § Interface ownership and visibility](https://google.github.io/styleguide/go/best-practices#interface-ownership-and-visibility),
[Decisions § Interfaces](https://google.github.io/styleguide/go/decisions#interfaces))

## Accept interfaces, return concrete types

Take interfaces as parameters and return concrete types. The caller then gets every method
and field, and can still pass the result wherever an interface is wanted.
([Decisions § Interfaces](https://google.github.io/styleguide/go/decisions#interfaces),
[Best Practices § Designing effective interfaces](https://google.github.io/styleguide/go/best-practices#designing-effective-interfaces))

Return an interface when:

- it is `error`;
- the function picks one of several concrete types at run time (a factory);
- the concrete type has methods that would let a caller break its guarantees;
- returning the concrete type would create an import cycle.

Don't wrap a value in an interface just to hide it.
([Best Practices § Designing effective interfaces](https://google.github.io/styleguide/go/best-practices#designing-effective-interfaces),
[Effective Go § Generality](https://go.dev/doc/effective_go#generality))

## Test doubles

Don't declare an interface on the implementing side only so tests can mock it, and don't
export a test double from a production package. Test through the real type's public API,
or let the consuming package define a small interface and a fake in its own tests.
([Code Review Comments § Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces),
[Best Practices § Avoid unnecessary interfaces](https://google.github.io/styleguide/go/best-practices#avoid-unnecessary-interfaces))

Where a real transport exists, prefer the production client connected to a test server
over a hand-written fake client.
([Best Practices § Use real transports](https://google.github.io/styleguide/go/best-practices#use-real-transports))
In brama the containerised servers under `test/testenv` play that part. See
`docs/adr/0008-test-against-containerised-servers.md`.

When a package of doubles is shared, name it for the package it doubles with `test`
appended (`creditcardtest`). Name each double for the behaviour it stands in for
(`AlwaysDeclines`), not with a generic `Mock` prefix.
([Best Practices § Test double and helper packages](https://google.github.io/styleguide/go/best-practices#test-double-and-helper-packages),
[Best Practices § Multiple test double behaviors](https://google.github.io/styleguide/go/best-practices#multiple-test-double-behaviors))

## Shape and names

Small interfaces are easier to implement and compose. The bigger the interface, the weaker
the abstraction.
([Best Practices § Designing effective interfaces](https://google.github.io/styleguide/go/best-practices#designing-effective-interfaces),
[Effective Go § Interfaces](https://go.dev/doc/effective_go#interfaces))

A one-method interface takes the method's name plus `-er`: `Reader`, `Detector`. Don't
reuse a well-known method name (`Read`, `String`, `Close`) unless it has the well-known
signature and meaning. A method that does mean it takes that name: `String`, not
`ToString`. ([Effective Go § Interface names](https://go.dev/doc/effective_go#interface-names))

Document an interface as its user manual: the contract, edge cases, and expected errors.
A single-method interface is documented on the type. Each method of a larger one gets its
own comment. Unexported interfaces deserve comments too.
([Best Practices § Designing effective interfaces](https://google.github.io/styleguide/go/best-practices#designing-effective-interfaces))

brama's implementations of a protocol interface assert it at compile time with
`var _ schema.Introspector = (*Introspector)(nil)` (`internal/schema/mysql/mysql.go`), so a
drifted method set breaks the build next to the type.

## Generics

Type parameters are allowed where they solve a real problem. Don't reach for them first. If
only one type is ever used, write the code for that type. Adding generics later is easier
than removing them.
([Decisions § Generics](https://google.github.io/styleguide/go/decisions#generics))
brama has no generic code yet.
