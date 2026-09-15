# Brama

Brama is a control layer for application environments. Code moves up, data moves down,
and raw production data never leaves the production server.

## Language

### Environments

**Environment**:
A named target Brama can act on — `production`, `staging`, `local`. Bound to exactly one Server.
_Avoid_: instance, stage, tier, site

**Server**:
A registered SSH host that hosts one or more Environments.
_Avoid_: machine, box, node, VPS

**Adapter**:
The framework-specific knowledge Brama holds about a project — WordPress, Laravel.
_Avoid_: driver, plugin, integration

**Provider**:
An infrastructure vendor integration — Hetzner, DigitalOcean.
_Avoid_: cloud, host

**Desired state**:
What `brama.yaml` declares a project should be. Owned by developers, through git.
_Avoid_: config, spec

**Observed state**:
What Brama recorded about an Environment the last time it acted on it. Owned by Brama.
_Avoid_: status, actual state

### Data movement

**Sync**:
Copying data between Environments. Always downward: production to staging to local.
_Avoid_: migration, transfer, replication, mirroring

**Pull**:
A Sync initiated from the receiving side. The only form a Sync takes — Brama has no push.
_Avoid_: import, fetch, download

**Placeholder**:
A generated stand-in that occupies the path, name, and shape of a real upload without
carrying its contents.
_Avoid_: dummy, stub, mock, fixture

### Anonymization

**Anonymization**:
Irreversibly replacing real values with fabricated ones, before the data leaves the Server.
_Avoid_: masking, obfuscation, sanitization, scrubbing, and especially pseudonymization —
that word names a weaker, reversible thing Brama deliberately does not do.

**Classification**:
The recorded decision of what happens to a column's values during Anonymization. Exactly
one of `fake`, `keep`, or `drop`.
_Avoid_: rule, policy, mapping, tag

**Discriminator**:
The column whose value selects which Classification applies to a row, for tables that
store many kinds of value in one column — `wp_usermeta.meta_key`.
_Avoid_: key column, EAV key, type column

**Preset**:
The Classification an Adapter ships for the tables it already knows.
_Avoid_: template, profile, defaults

**Unclassified**:
A column, or a Discriminator value, found in the source with no Classification. Causes a
Refusal.
_Avoid_: unknown, unmapped, missing

### Safety

**Shim**:
The Brama component that runs on a Server. It performs operations there and is the only
thing that ever reads raw production data.
_Avoid_: agent, daemon, worker — it is none of these, it does not run continuously.

**Identity**:
A named actor holding its own credential and its own Policy — a developer, a CI job, an
AI agent.
_Avoid_: user, account, role, principal

**Policy**:
The rules governing which operations an Identity may perform against which Environment.
Held on the Server.
_Avoid_: permissions, ACL, rules

**Refusal**:
The outcome when Policy, or an Unclassified column, stops an operation. Distinct from a
failure: nothing went wrong.
_Avoid_: error, rejection, denial, block

**Recovery point**:
The copy of an Environment's data taken before a destructive operation, so the operation
can be undone.
_Avoid_: backup, snapshot, dump

### Deployment

**Release**:
An immutable directory built from one commit SHA at one moment.
_Avoid_: build, version, artifact

**Current**:
The symlink pointing at the live Release.

**Deployment**:
The record of one deploy — commit, Release, Recovery point, previous Release, outcome.
_Avoid_: deploy run, job

**Rollback**:
Repointing Current at an earlier Release, optionally restoring its Recovery point.
_Avoid_: revert, undo, downgrade

**Promote**:
Moving a validated Environment state forward, staging to production.
_Avoid_: ship, release (as a verb), push

**Executor**:
The transport that carries an operation to a Server.
_Avoid_: runner, transport, connection
