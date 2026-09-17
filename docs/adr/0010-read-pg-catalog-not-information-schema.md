# Read pg_catalog, not information_schema, on PostgreSQL

The MySQL Introspector asks `information_schema`, as ADR 0009 says. The PostgreSQL one asks
`pg_catalog` instead. The two Introspectors end at the same Schema either way, so this is a
decision about which catalogue is trustworthy on which server, not about the model.

`information_schema` is the portable answer and on PostgreSQL it is the wrong one, for a
reason that is invisible until it matters: it shows a user only the objects that user holds
some privilege on. No error, no warning, no gap in the numbering — a connection that can
read most of a database gets back a Schema missing the tables and columns it cannot touch,
and it looks exactly like a database that does not have them. Brama's whole guardrail is
that every column is seen and every column is Classified, so a column the Introspector never
saw is a column nobody was asked to refuse on. Restricting the credentials Brama is given is
a reasonable thing for someone to do; silently returning a smaller Schema when they do is
not a reasonable thing for Brama to answer with. `pg_catalog` hides nothing.

Two smaller things follow the same way. `pg_catalog` has `format_type`, which prints a
column's type as declared — `numeric(10,2)`, `character varying(100)` — where
`information_schema` hands back precision and scale in separate columns and leaves
reassembling the declaration to whoever asked, which is the guesswork the Schema's `Declared`
field exists to avoid. And PostgreSQL enforces uniqueness with an index rather than a
constraint, so a bare `CREATE UNIQUE INDEX` constrains every value written to a column while
appearing in no constraint view; `pg_index` is where both it and a `UNIQUE` constraint can be
seen at once.

MySQL has none of these problems. `information_schema` there is not privilege-filtered in the
way that matters here, `COLUMN_TYPE` is the full declaration already, and `STATISTICS` lists
every unique index. So MySQL keeps it, and the two Introspectors are asymmetric on purpose.

## Consequences

- The two Introspectors share no SQL and are not meant to. What they share is the Schema they
  produce and the loop that reads rows; everything between those is that server's own dialect.
- PostgreSQL 12 or later. `pg_attribute.attgenerated`, which is how a generated column is
  recognised, does not exist before it.
- `pg_catalog` is a system catalogue, and its columns are documented but not standardised.
  A PostgreSQL major version may rename one, and the containerised PostgreSQL in
  `test/testenv` is pinned to a major version so that the queries are exercised against a
  known server rather than whichever one a laptop had.
- Reading `pg_catalog` needs no privilege beyond connecting. The Introspector therefore
  answers about tables the connected user cannot select from — which is correct: a column
  exists whether or not this connection may read it, and Brama has to see it to refuse on it.
