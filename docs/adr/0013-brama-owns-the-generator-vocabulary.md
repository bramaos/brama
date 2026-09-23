# Brama owns the Generator vocabulary

The set of Generators is closed and lives in Brama, not in Adapters and not in
`brama.yaml`. A config can therefore be validated without loading Adapter code or reaching
a database, which is what lets `brama anonymize check` mean something on a CI runner, and
it means an unrecognised Generator is a refusal at check time rather than a silent
degradation to "random string" halfway through a dump on a production Server. Letting
projects declare their own Generators would put fabrication logic written by a developer
into the one process that reads raw production data.

`fake.password` is the deliberate exception. The name is Brama's and validates like any
other, but the bytes come from the Adapter, because a password column holds a hash and only
the Adapter knows whether that means phpass or bcrypt. A hash-shaped random string leaves
staging with no way to log in; a `keep` leaks real hashes, which are personal data and
crackable offline. The Adapter produces a valid hash of one documented, fixed password
instead.

## Consequences

- Generators declare the column types and name patterns they claim, and `check` refuses a
  Generator whose output cannot fit a column's declared type or length — in the editor,
  not mid-dump.
- No two Generators may claim the same column. Disjointness is asserted in Brama's own test
  suite, so an overlap is caught before release; if one reaches runtime anyway, the column
  becomes a review item rather than a silent pick.
- A project needing a format Brama has no Generator for classifies the column `keep` and
  Approves it per Environment, or `drop`s it. It cannot invent a third option.
- `fake.password` behaves differently under different Adapters, and on a project with no
  Adapter it has no implementation at all. That is a refusal, not a fallback.
