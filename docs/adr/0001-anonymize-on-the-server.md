# Anonymize on the server, before transfer

Anonymization could run on the developer's machine after the dump arrives, which would keep
all logic in one binary and leave the Server untouched. We run it on the Server instead,
inside the Shim, so that raw production values are never written to a laptop's disk at all —
that is the only version of the promise we can actually defend, and it is the product.

## Consequences

- Brama's original guarantee "nothing is installed on the target server" is false and has
  been replaced by "no daemon, no background process, no open port, no runtime dependency".
  The Shim is uploaded over SSH and runs only for the duration of an operation.
- The Shim is versioned software that must be kept in step with the CLI. It upgrades itself
  as a visible step before an operation, never during one.
- An interrupted Pull cannot leave unanonymized data behind, because none was ever sent.
