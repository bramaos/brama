# Ship the data half before deployment

Brama was scoped to ship deployment first and Sync second. We reversed it: the first
release is `db pull` and `files pull` with Anonymization, and deployment follows. Deployment
is a commodity — Forge, Ploi, Deployer, Capistrano, Kamal, Dokku and Coolify all do it, most
of them free — so leading with it invites the question "why not use one of those?" on day
one. An anonymized downward Pull exists nowhere, is the pain the target user feels weekly,
and requires no one to abandon the deployment tooling they already have.

## Consequences

- The first release contains no destructive remote operation. Its safety promise is that
  nothing leaks, not that nothing is destroyed.
- Recovery points, Rollback, Releases and the Deployment record move to the second release.
- Identity and Policy move with them: a permission boundary guarding a single read-only
  operation is not worth building yet. The Shim still ships now, because Anonymization runs
  on the Server.
- Brama is adopted alongside an existing deployment tool rather than in place of one, which
  lowers the cost of trying it.
