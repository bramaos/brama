# Introspect the database directly

Brama reads a Schema by connecting to the database and querying its own catalogue —
`information_schema` on MySQL. Not through WP-CLI, not through the framework, and not by
parsing a dump.

Asking the framework is the obvious route and the wrong one. WP-CLI needs a WordPress that
boots: a fatal error in a plugin, a PHP version the site predates, or a `wp-config.php`
pointing at a database that moved, and the answer is an exception rather than a Schema —
on precisely the Server whose data someone is trying to get a safe copy of. It also needs
PHP on the Server at a version WP-CLI supports, which is one more thing that can be absent
or wrong. And it only ever answers for WordPress: every Adapter after it would need its own
route to the same facts, and Brama would hold as many introspectors as it holds frameworks.

Parsing a dump fails differently. `mysqldump` output is a dialect of DDL, so reading it
means writing a parser for a language nobody specified, kept in step with two vendors. It
also means producing the dump first — the full data, read and written, before Brama knows
which columns it was allowed to read.

The database itself has none of those problems. It is running, it is the authority, its
catalogue is standardised across MySQL and PostgreSQL in structure if not in detail, and it
answers about a database with no framework at all.

## Consequences

- Brama needs database credentials, and takes them from the project's own config — the
  path already recorded as `app.paths.config`. It stores none of its own.
- Introspection runs where the database is. On a Server that is the Shim, which reaches
  the database over its local socket, so no database port is ever exposed and ADR 0001
  holds unchanged.
- The Schema model is Brama's, not MySQL's. Each database system gets an Introspector that
  ends its own dialect at the same struct, which is what lets the Anonymization engine work
  without knowing which one answered.
- A site whose WordPress is broken can still be classified and still be Pulled. That is the
  case the decision is for.
- Type names stay in the database's own vocabulary. Normalising `bigint` and `int8` into an
  invented third set would lose the detail a fabricated value has to respect, so a consumer
  that interprets a type needs to know which system produced it.
