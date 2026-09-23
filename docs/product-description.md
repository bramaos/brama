# BramaOS — Product Description

> Status: decided, v0.1 not yet implemented.
> Purpose: canonical context document. Read alongside `CONTEXT.md` and `docs/adr/`.
> Every decision here is settled. If a change is needed, change this file first, then the code.

---

## 1. One sentence

**Brama gives you a real copy of production to develop against, without a single real customer record ever leaving the server.**

Everything in this document follows from that sentence.

### Philosophy

> Code moves up. Data moves down. Raw production data never leaves the production server.
> Branches select intent, SHAs define reality, SSH owns identity, policies own safety, and Cloud adds coordination without becoming a dependency.

## 2. Names

| Thing | Value |
| --- | --- |
| Umbrella brand | **BramaOS** |
| CLI binary | `brama` |
| Domain | `brama.sh` |
| GitHub org | `github.com/bramaos` |
| Main repo | `github.com/bramaos/brama` |
| License | Apache-2.0 |

`BramaOS` is an **operations system for application environments** — a control layer. It is not an operating system and must never be described as one. The CLI is always `brama`, never `bramaos`.

Apache-2.0 rather than MIT: the explicit patent grant is what company legal review looks for, and companies are the buyer for the Cloud tier.

Future product names reserved under the umbrella: Brama Cloud, Brama Agent, Brama Enterprise, Brama Desktop.

## 3. What Brama is

Brama is a **control plane for application environments**. It owns the relationship between local, staging, and production:

```
              Brama
Local  <->  Staging  <->  Production
          databases
          files
          configs
          deployments
          recovery points
```

v0.1 owns exactly one edge of that picture: production to local, for data. Everything else is stated intent, not shipped software.

### Long-term vision (stated, not v1)

Brama becomes the **permission boundary between humans/AI agents and production infrastructure**.

```
AI agent / developer
        |
      brama
        |
   policy engine
        |
     shim (on the server)
        |
  servers, databases, applications
```

An AI agent must never get `ssh root@server`. It gets `brama db pull --json --non-interactive`, and Policy decides what actually runs.

**Stated limitation, never to be marketed away:** the boundary holds only while the agent does not also hold an unrestricted SSH key for the same Server. `brama doctor` warns when it finds one. Brama narrows the path an agent takes; it does not take away credentials the developer already has lying around.

## 4. Personas

**Primary — solo developer / small agency owner.**
A freelance developer managing 5–50 client projects on VPS infrastructure.
Pain: no safe way to get production data onto a laptop, hand-rolled sync scripts per project, fear of touching production, manual recovery points, inconsistent environments, and a growing awareness that copying customer data to a laptop is not legal.
Goal: one command that produces a working local environment containing no real customer data.

**Secondary — small engineering team.**
A ~10-person SaaS team that wants simple deployment workflows without building a DevOps platform.
Needs: approvals, audit logs, team access, Policy.

**Future — AI-assisted development team.**
A team letting coding agents touch infrastructure.
Needs: capability-based permissions, AI audit logs, safe automation.

v0.1 is designed for the primary persona only. The others constrain the architecture (server-side state, Executor abstraction, JSON output) but get no dedicated features yet.

## 5. Positioning

Brama does not enter the market as a deployment tool. Deployment is a commodity — Forge, Ploi, Deployer, Capistrano, Kamal, Dokku and Coolify all do it, most of them free — and leading with it invites "why not just use Kamal?" on day one.

Brama enters as the only tool that makes production data safe to develop against. Adjacent tools each solve a fragment: Neon and PlanetScale branch databases they host; Tonic.ai anonymizes at enterprise prices; Neosync was archived in July 2025; WP Migrate and `wp db export` move data with no anonymization at all. Nobody moves data *and* anonymizes it *before it leaves the source server*.

Marketing lines:

> Brama refuses to move production data it has not been told how to anonymize.

> Your local database behaves exactly like production. None of it is real.

Not "Brama has anonymization".

## 6. Core guarantees

Product promises, not implementation details. Violating one is a bug.

1. **Raw production values never leave the production server.** Anonymization runs in the Shim, before transfer.
2. **Anonymization is irreversible.** The mapping from real to fake exists only for the duration of the dump and is destroyed with it. There is no key.
3. **Brama refuses rather than guesses.** An Unclassified column, or an unrecognised Discriminator value, stops the operation.
4. **Data flows downward only.** production -> staging -> local. Never upward.
5. **Every destructive operation creates a Recovery point first** — including destructive operations on the developer's own machine.
6. **Brama never stores secrets.** Authentication is OpenSSH's. Environment secrets belong to the Server.
7. **No daemon, no background process, no open port, no runtime dependency on the Server.** The Shim is uploaded over SSH and runs only for the duration of an operation.

Guarantee 7 replaces the earlier "nothing is installed on the target server". See `docs/adr/0001-anonymize-on-the-server.md`.

## 7. v0.1 scope

**Core promise of v0.1: Brama puts a working copy of production on your laptop with none of the real data in it.**

Commands:

```
brama
  init
  server
    add
    list                                     # after add
    remove                                   # after add
  env
    list
    inspect
  anonymize
    init
    check
  db pull      --dry-run --json --non-interactive
  files pull   --dry-run --json --non-interactive --real-files
  doctor
  logs
```

Adapters in v0.1: **WordPress first, Laravel second.** WordPress leads because that is where the first real projects are, and because WordPress is the harder case — key/value tables, serialized PHP, URLs embedded in data. An adapter that survives WordPress will not be bent by Laravel.

Databases in v0.1: **MySQL and PostgreSQL.** PostgreSQL is not optional — without it, Brama is a PHP tool, and the JavaScript ecosystem is out of reach permanently.

Providers in v0.1: **none.**

### `db pull` flow

```
1. Connect to the Server over SSH
2. Upload or upgrade the Shim (visible step, recorded)
3. Shim introspects the schema
4. Compare schema against Classification — refuse if anything is Unclassified
5. Shim dumps table by table, anonymizing values in the same pass
6. Shim rewrites URLs, serialization-aware, in the same pass
7. Transfer the anonymized dump
8. Take a Recovery point of the local database
9. Import
```

The Shim discards its mapping and its temporary files when step 7 completes.

### `files pull` flow

The upload tree is recreated with Placeholders: images generated at their real dimensions, valid empty files for other types — a one-page blank PDF, a valid empty archive, a one-second video. Nothing 404s, nothing crashes a parser, no contract or passport scan leaves the Server. A 2.6 GB upload directory transfers as a few megabytes, which also makes it the fastest option.

`--real-files` transfers the originals. It warns, and it is recorded.

### Non-goals for v0.1

Explicitly do not build:

- Deployment, Releases, Rollback, remote Recovery points
- Identity, Policy engine, forced-command hardening
- Upward data sync
- Subsetting (row limits, referential subsets) — whole tables only
- AI-assisted Classification
- YAML policy language
- Web dashboard
- Multi-server orchestration
- Server provisioning
- Offsite backup storage

## 8. Anonymization model

The centre of the product. Everything else in v0.1 is transport.

### Classification

Every column gets exactly one answer:

| Answer | Meaning |
| --- | --- |
| `fake` | Replace with a fabricated value of the same shape |
| `keep` | Transfer as-is |
| `drop` | Transfer as empty / null |

There is no fourth state and no default. A column with no answer is Unclassified, and an Unclassified column refuses the Pull with exit code 42.

`keep` on a column Brama believes holds personal data is allowed — Brama is not the compliance police — but it warns at the time and records the choice in `brama.yaml`, where code review can see it.

### Discriminators

WordPress keeps most of its personal data in key/value tables, where one column holds many kinds of value:

```
wp_usermeta    meta_key = 'billing_phone'                 -> personal data
wp_usermeta    meta_key = '_edit_lock'                    -> harmless
wp_options     option_name = 'woocommerce_stripe_settings' -> live API keys
wp_postmeta    meta_key = '_billing_email'                -> personal data
```

Classifying the column `meta_value` as `fake` destroys the site. Classifying it `keep` leaks every order. So a table may declare a **Discriminator** — the column whose value selects which Classification applies to the row. The same fail-closed rule holds: an unrecognised Discriminator value refuses the Pull.

This is not a WordPress special case. Laravel `settings`, `meta`, and `options` tables have the same shape.

### Presets

Each Adapter ships a maintained Classification for the tables it already knows — WordPress core plus WooCommerce, Laravel's standard tables. The user classifies only what is theirs. This turns a long afternoon into twenty minutes, and the Preset improves with every project it meets.

### How values are faked

For each distinct real value, Brama rolls a random fake and remembers the pairing **for the duration of the dump only**. Every table that contains that real value receives the same fake, so joins on natural keys still line up and the local application behaves like production. When the dump ends, the mapping is destroyed.

There is no formula and no key, so nothing can reverse it. See `docs/adr/0002-discard-the-anonymization-mapping.md`.

The cost: fakes differ between Pulls. Local data is disposable, so this costs nothing real.

### Serialization-aware rewriting

WordPress stores URLs inside serialized PHP, where a naive string replace corrupts the byte-length prefixes and breaks the site. Brama walks the value, rewrites, and re-serializes correctly — in Go, in the same pass as Anonymization.

Consequence: Brama does not need WP-CLI on the Server, and is therefore immune to the well-known failure where WP-CLI's bootstrap prints a PHP warning into the dump and corrupts the SQL.

### What this does not solve

Irreversibility alone does not make a dataset anonymous. A row with `birth_date`, `zip_code` and `gender` left as `keep` identifies most people without needing a name. Free-text columns can hide anything. This is precisely why Classification covers every column rather than the obviously-sensitive ones — the mechanism cannot be safe on its own, only the completeness of the answers can.

## 9. Architecture decisions

| Decision | Choice | Why |
| --- | --- | --- |
| Language | Go, single static binary | No runtime on the dev machine, trivial `curl \| sh` and Homebrew distribution, and the Shim can be the same language |
| Execution | SSH, plus a Shim uploaded over it | Anonymization must run on the Server, so something of ours must run there |
| Server model | Attach to existing servers | The persona has running VPSes that need managing, not replacing |
| Project config | `brama.yaml`, committed to the repo | Versioned with the code, reviewable, clone-and-go on a new machine |
| Policy location | On the Server, never in the repo | An agent that can edit `brama.yaml` must not be able to widen its own permissions |
| Secrets | Server-owned | Zero secret-custody liability |
| Local target | Read from the project's own env config | Works for Docker, Valet, Herd, Lando and native installs without knowing about any of them |
| Recovery points | Local before import, retain 3 | Guarantee 5 applies to the developer's machine too |
| Environment model | One Environment = one Server | Covers the primary persona; keeps the engines sequential |
| Policy language | None | Hardcoded guardrails until real cases exist |
| Git strategy | `git fetch` + `git checkout <SHA>` | For v0.2. `git pull` is mutable and ambiguous |

### The Shim

The Shim is the Brama component that runs on a Server. It is uploaded over SSH, per architecture, and runs only for the duration of an operation. It is not an agent and not a daemon.

It lives at `~/.brama/shim`, in the home directory of the SSH user — server-global, not per-Environment, because one binary serves every Environment on the Server. `brama server add` installs it as part of registering a Server. See [ADR 0006](adr/0006-server-global-shim-home.md).

The builds are **embedded in the `brama` binary** and streamed down the connection that is already open. Nothing is fetched at install time: a Server's outbound network is the last thing Brama should need to widen, and an embedded build cannot disagree with the CLI that sent it. Brama ships `linux/amd64` and `linux/arm64`; a Server reporting anything else fails with the platform it detected, and that is a failure (exit 1), not a Refusal.

Installed builds are versioned, with a symlink at the path operations execute:

```
~/.brama/
├── shim -> shim-0.1.1
├── shim-0.1.0
└── shim-0.1.1
```

The swap is a rename, so an interrupted upload leaves the previous Shim intact rather than a half-written one at the live path. Two builds are kept. A Shim already installed at the CLI's version is not re-sent — but it is still run, because executing it is what proves the install rather than what the filesystem claims about it.

It upgrades itself when the CLI is newer, as its **own visible step before the operation**, never mid-operation. The upgrade prints what changed, and from the first operation that names an Environment it is recorded in that Environment's `state.json`. `brama server add` names no Environment, so its install is reported in the result and recorded nowhere — the installed version is readable from the Server at any time, since the symlink target carries it.

The step names one of four outcomes — `none`, `install`, `upgrade`, `replace`. A Shim that differs from the CLI's build but is not older than it — a Server a newer `brama` reached first, or either side built outside a tagged tree — is **replaced**, not refused: the Shim and the CLI must come from one build, and a Shim the CLI cannot drive protects nothing. See `docs/adr/0007-replace-a-shim-brama-cannot-call-older.md`. `--dry-run` reports the step it would take and takes none of it, which is the only way to see a pending upgrade before it happens.

In v0.2, when deployment ships, the Shim receives **structured operations** rather than shell commands:

```
{"op": "deploy", "environment": "production", "commit": "a82fd92"}
```

This makes "no arbitrary remote shell execution" structurally true rather than a promise — and it means Adapter logic lives in the Shim, not in the CLI. The Executor interface is therefore an operation channel, not `Run(cmd string)`.

### Server layout

```
/var/www/app
├── current -> releases/20260915_162400      # v0.2
├── releases/                                # v0.2
├── shared/
│   ├── .env
│   └── uploads/
└── .brama/
    ├── state.json
    └── policy.json                          # v0.2

~/.brama/                                    # the SSH user's home
├── shim -> shim-0.1.1
└── shim-0.1.1
```

`state.json` is the Server-side source of truth: operation history, Shim version, and — from v0.2 — Deployment records. It stays under the Environment it describes; only the Shim binary is server-global.

### State ownership

Two states, two owners. Never merge them.

| State | Lives in | Owned by | Means |
| --- | --- | --- | --- |
| **Desired state** | `brama.yaml`, in the repo | developers, via git | what this project *should* be |
| **Observed state** | `.brama/state.json`, on the Server | Brama, written on every operation | what the Environment *actually is* |

Policy is a third thing, and it lives only on the Server. It is not Desired state, because Desired state is editable by anything that can write to the repo.

### Internal package layout

```
cmd/brama
cmd/brama-shim
internal/
  cli/               # cobra commands, wrapped by fang
  config/
  core/
    pull/
    anonymize/
      classify/
      fake/
      serialize/
    files/
  schema/
    mysql/
    postgres/
  ssh/               # shells out to OpenSSH; no in-process ssh client
  shim/              # the embedded shim builds and their install
  executor/          # v0.2, when there are two reaches to abstract over
  adapter/
    wordpress/
    laravel/
  refusal/           # the declined-operation outcome
  renderer/
    human/           # lip gloss, huh
    json/            # the §10 contract
  scaffold/          # writes the starting brama.yaml
  policy/            # v0.2
docs/
examples/
tests/
```

### The rendering rule

The core never formats. Every operation returns a Result value — `PullResult`, `DoctorResult` —
and a Renderer turns it into either human output or the §10 JSON contract. Two consequences,
both load-bearing:

**`--json` cannot be forgotten.** A command that returns a Result gets machine output for free,
because it never had the option of printing prose instead.

**The Shim stays small.** `cmd/brama-shim` is uploaded over SSH and version-checked on every
operation, so it must not carry a terminal UI it can never use. Because the core formats nothing,
the Shim links `core/`, `schema/` and `executor/` without pulling in Cobra, Fang, Huh or Lip Gloss.

The rule that enforces both:

> Nothing under `core/`, `schema/`, `config/` or `executor/` may import `renderer/`, `cli/`, or any
> Charm package.

Deployment packages (`core/deploy/`, `core/backup/`) are deliberately absent rather than empty.
An empty package is not a seam — the seams are the `Executor` interface and the Result pattern,
which already exist. See `docs/adr/0004-ship-the-data-half-first.md`.

### Config sketch

```yaml
app:
  adapter: wordpress
  repository: git@github.com:company/project.git

environments:
  production:
    server: prod
    path: /www/htdocs/w01949b8/production
    url: https://www.example.com
  staging:
    server: staging
    path: /www/htdocs/w01949b8/staging
    url: https://staging.example.com
  local:
    url: https://example.local.test

servers:
  prod:
    host: example-prod
    user: deploy

anonymize:
  preset: wordpress+woocommerce

  tables:
    wp_users:
      user_email: fake
      user_pass: drop
      user_login: fake
      ID: keep

    wp_usermeta:
      discriminator: meta_key
      keys:
        billing_phone: fake
        billing_address_1: fake
        _edit_lock: keep
```

### SSH identity

Hard rule: **Brama does not own authentication.** OpenSSH does.

```yaml
# never
server:
  user: root
  password: xxx
  private_key: ~/.ssh/id_rsa

# always
servers:
  prod:
    host: production.example.com
    user: deploy
```

Authentication resolves through `~/.ssh/config`, `ssh-agent`, and hardware keys.

`brama server add` supports `--dry-run`, and the reasoning that once said it should not is worth
keeping: its substance is remote — it installs the Shim — so a dry run that skipped the install
would verify nothing, and one that performed it would not be dry. What changed is that the
version check gave the dry run something of its own to say. It connects, and it answers *which
Shim does this Server already run, and what would brama do about it* — a question that cannot be
answered from `brama.yaml`, and the only way to see a pending upgrade before it happens. It
sends nothing and writes nothing, in `brama.yaml` or on the Server.

### `--dry-run`

Every acting command whose effect is local supports it. It resolves everything and executes
nothing:

```
Environment:       production
Server:            example-prod
Adapter:           wordpress (bedrock)
Shim:              v0.1.2 -> v0.1.4 (will upgrade)
Tables:            63
Classified:        63
Unclassified:      0
Columns faked:     41
URL rewrite:       https://www.example.com -> https://example.local.test
Local recovery:    will dump 214 MB before import

Continue? [y/N]
```

### Server requirements

Linux, SSH access, and a database client. Nothing else — no PHP, no WP-CLI, no Node.

`brama doctor` validates this, and additionally warns when it finds an unrestricted SSH key for a Brama-managed Server in `~/.ssh`.

## 10. Agent interface

v0.1 ships `--json` and `--non-interactive` on every command that acts.

```
brama db pull production local --json --non-interactive
```

```json
{
  "action": "db_pull",
  "source": "production",
  "target": "local",
  "tables": 63,
  "unclassified": 0,
  "rows_faked": 184203,
  "recovery_point": "local-20260915162400"
}
```

Rules:

- Structured output and explicit exit codes for every action.
- `--non-interactive` suppresses prompts. It **must not** bypass guardrails. `--yes` never means "ignore everything" — that is the standard mistake in automation tooling and Brama refuses it.
- A Refusal is not an error. It is a distinct, documented outcome, so an agent can tell "I am not allowed" from "something broke".
- No command exposes arbitrary shell execution on the target Server.

Refusal example:

```json
{
  "status": "refused",
  "reason": "unclassified",
  "detail": "wp_usermeta.meta_key='stripe_customer_id' has no classification",
  "fix": "brama anonymize init"
}
```

Exit codes:

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Operation failed (error) |
| `42` | Refused — not an error, the operation was declined |

An MCP server is a plausible v0.2+ addition, not v0.1.

## 11. v0.1 guardrails

Hardcoded. No configuration.

- No upward data movement.
- No Pull while anything is Unclassified.
- No raw upload files without `--real-files`, which warns and is recorded.
- A Recovery point of the local database before any import.
- No arbitrary remote shell execution.

## 12. Roadmap

**v0.1 — Anonymized downward sync.** The differentiator, first.
`init`, `server add`, `anonymize init`, `anonymize review`, `anonymize check`, `db pull`, `files pull`, `doctor`, `logs`. WordPress. MySQL and PostgreSQL. The Shim.

**v0.2 — Safe deployment, and the boundary.**
`deploy`, `backup`, `rollback`, Releases, health checks, atomic symlink swap. Forced-command hardening at `server add`. `brama identity add` and the Policy engine, Server-stored. Structured operations. Laravel adapter.

**v0.3 — Environment control plane.**
`brama environment list`, `brama promote staging production`, `brama diff staging production`. Declarative Policy. Subsetting. AI-assisted Classification.

**Later.** Brama Cloud (hosted control plane, approvals, audit, team access), Brama Agent (live monitoring, drift detection, scheduled Recovery points), web UI, desktop app.

The roadmap is public as a GitHub Projects board fed by Issues. When `brama.sh` exists, its roadmap page is generated from the same Issues — one source, never two.

## 13. Business model

Open-source CLI as the distribution engine — free, forkable, no paid gate on the core loop. Revenue later from the control plane: Brama Cloud / Teams / Enterprise. Same shape as Sentry, Vercel, Grafana.

Architectural consequence: **the CLI must be fully useful with no account and no network service.** Anything that requires a Brama backend belongs in Cloud, not in the CLI. This is why AI-assisted Classification is not in v0.1 — Classification must work offline and deterministically.

> Brama CLI is complete software. Cloud adds coordination, not functionality.

| Open source, works forever | Brama Cloud |
| --- | --- |
| pull, anonymize, deploy, rollback, recovery points, logs, servers, env model, guardrails | team approvals, audit dashboard, fleet management, policy distribution, agent identities, notifications |

## 14. Vocabulary

Canonical terms live in `CONTEXT.md`. Use them exactly, in code and in docs. Decisions with real trade-offs behind them live in `docs/adr/`.

## 15. Success criterion for v0.1

An agency running client projects installs Brama, runs `brama init`, `brama server add prod`, `brama anonymize init`, and `brama db pull production local` — and ends up with a local site that behaves exactly like production and contains no real customer data, without having written a line of script.

If that path works, the rest of the roadmap has a foundation. If it does not, nothing further matters.
