# Brama

Brama is a control layer for application environments. Code moves up, data moves down,
and raw production data never leaves the production server.

## Language

### Environments

**Environment**:
A named target Brama can act on — `production`, `staging`, `local`. Reached through exactly one
Executor.
_Avoid_: instance, stage, tier, site

**Reach**:
How Brama gets to an Environment — directly, on the machine it is running on, or across SSH to a
Server. Derived from whether the Environment names a Server, never declared on its own: anything
that can edit `brama.yaml` must not be able to change what an Environment is.
_Avoid_: class, type, locality, tier

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

**Schema**:
The structure of an Environment's database — its tables, their columns, the types and
constraints on those columns, and the foreign keys between them. Never any rows: a Schema is
what Classification is decided against, and deciding must not require reading production data.
_Avoid_: structure, catalog, metadata, DDL

**Introspection**:
Reading a Schema out of a live database, by asking the database itself. Never by parsing a
dump, and never through the framework — WP-CLI cannot answer for a site that is broken, and a
database with no Adapter has no framework to ask.
_Avoid_: discovery, reflection, scanning, sniffing

**Catalogue**:
The database's own account of itself, which Introspection reads — `information_schema` on
MySQL, `pg_catalog` on PostgreSQL. A Catalogue belongs to a database and is spelled that
database's way; a Schema is Brama's, and is the same whichever Catalogue answered. The one
place the word "catalog" is not a synonym for Schema.
_Avoid_: metadata, system tables, data dictionary

**Anonymization**:
Irreversibly replacing real values with fabricated ones, before the data leaves the Server.
_Avoid_: masking, obfuscation, sanitization, scrubbing, and especially pseudonymization —
that word names a weaker, reversible thing Brama deliberately does not do.

**Classification**:
The recorded decision of what happens to a column's values during Anonymization. Exactly
one of `fake.<generator>`, `keep`, or `drop`, written as a column's `action`. `fake` alone
is not one: which Generator fabricates the value is part of the decision.
_Avoid_: rule, policy, mapping, tag

**Generator**:
The named recipe a `fake` Classification fabricates its value with — `fake.email`,
`fake.full_name`. Brama owns the vocabulary, and a Generator declares which column types and
names it claims, so nothing is ever fabricated by resemblance.
_Avoid_: faker, formatter, strategy, and provider — that word already names an
infrastructure vendor.

**Correlation group**:
A set of columns sharing one identity, so a real value occurring in all of them becomes the
same fabricated value in all of them and the joins between them survive Anonymization.
_Avoid_: link, alias, identity map, seed

**Approval**:
An Environment's permission to receive the real values of a `keep` column. Distinct from
Classification: Classification settles what a column means, Approval settles where its real
values may go. Only a human grants one.
_Avoid_: consent, exception, allowlist, override

**Discriminator**:
The column whose value selects which Classification applies to a row, for tables that
store many kinds of value in one column — `wp_usermeta.meta_key`. The column it selects
*for* is named beside it as the table's `value` — `wp_usermeta.meta_value`.
_Avoid_: key column, EAV key, type column

**Preset**:
The Classification an Adapter ships for the tables it already knows. Knowledge, never
Approval: a Preset can say a column holds a public display name and still authorize no
Environment to receive it.
_Avoid_: template, profile, defaults

**Preset drift**:
A column the Preset and the recorded Classification disagree about, once upgrading Brama
has changed what the Preset says. The stricter of the two wins: a Preset moving a column
off `keep` is applied on its own, a Preset moving one onto `keep` is held until a human
passes it through review. The record is the baseline, so there is no version to pin and no
history to keep. Always said with "Preset" — drift between Environments is a different
thing and not this one.
_Avoid_: conflict, divergence, staleness, upgrade

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
The transport that carries an operation to an Environment — over SSH to a Server, or directly
when the Environment is `local`.
_Avoid_: runner, transport, connection

### Output

**Result**:
What an operation returns when it finishes — the facts about what happened, carrying no
formatting. `PullResult`, `DoctorResult`.
_Avoid_: response, report, output, summary

**Renderer**:
The component that turns a Result into output for one audience: a person, or the documented
JSON contract. The only part of Brama that knows a terminal exists.
_Avoid_: formatter, printer, view, presenter, UI
