# Discard the anonymization mapping when the dump ends

A fake value can be derived from the real one by a keyed function (HMAC), which keeps it
stable across Pulls and makes local data comparable over time. We roll a random fake per
distinct value instead, hold the mapping only for the duration of the dump so that joins
across tables still line up, and destroy it on exit — because a derived value keeps a link
back to the real person, which under GDPR is pseudonymization and still personal data.
Destroying the mapping is what lets us say Anonymization and mean it.

## Consequences

- Fake values differ between Pulls. Local data is disposable, so this costs nothing.
- The mapping is held in memory and spills to a temporary file on the Server for
  high-cardinality columns; it is removed when the operation ends.
- There is no key to store, rotate, or leak — which also avoids conflicting with Brama's
  rule that it never holds secrets.
- Irreversibility alone does not make a dataset anonymous: quasi-identifiers left as `keep`
  can still identify people. That is why Classification covers every column, not just the
  obvious ones.
