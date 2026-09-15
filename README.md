<h1 align="center">brama</h1>

<p align="center">
  <strong>A real copy of production to develop against — with none of the real data.</strong>
</p>

<p align="center">
  <a href="#status"><img alt="status" src="https://img.shields.io/badge/status-pre--alpha-orange"></a>
  <a href="LICENSE"><img alt="license" src="https://img.shields.io/badge/license-Apache--2.0-blue"></a>
  <a href="https://github.com/bramaos/brama/milestone/1"><img alt="milestone" src="https://img.shields.io/badge/milestone-v0.1-lightgrey"></a>
</p>

---

> **Status:** not implemented yet. This README describes software that is being built in the
> open, and says so rather than pretending otherwise. Follow the
> [roadmap](https://github.com/bramaos/brama/issues) to see where it is.

## The problem

Your local environment is a fiction. The data is fake, the edge cases don't exist, and the
bug you're chasing only happens on production.

So you pull the production database down. Now your laptop holds every customer's email
address, phone number, and billing address — which is a GDPR violation, and everyone does
it anyway, because the alternative is developing blind.

The tools that exist pick one half. `wp db export`, WP Migrate, and hand-rolled `mysqldump`
scripts move the data and anonymize none of it. Tonic.ai anonymizes at enterprise prices.
Neon and PlanetScale branch databases they host for you. Neosync was archived in July 2025.

Nobody moves the data **and** anonymizes it **before it leaves the source server**.

## What brama does

```console
$ brama db pull production local

  Server            elements-prod
  Adapter           wordpress (bedrock)
  Tables            63 · all classified
  Anonymizing       41 columns, 184,203 rows
  URL rewrite       https://www.example.com → https://example.local.test
  Recovery point    local database, 214 MB

  ✓ pulled in 1m 12s — 0 real customer records transferred
```

One command. A local site that behaves exactly like production, containing no real person's
data. The raw values never left the server.

## How

Three rules do the work.

**1. It runs on the server.** Brama uploads a small binary, and anonymization happens there,
during the dump. Not on your laptop afterwards — by then the leak has already happened.

**2. The mapping is destroyed.** Each distinct real value gets a *random* fake, remembered
only for the duration of the dump so the same customer gets the same fake in every table and
your joins still work. When the dump ends, the mapping is thrown away. There is no key and
no formula, so nothing can reverse it. That's the difference between anonymization and the
weaker thing most tools do.

**3. It refuses rather than guesses.** Every column is classified `fake`, `keep`, or `drop`.
A column with no answer stops the pull:

```console
$ brama db pull production local

  ✗ refused — wp_usermeta.meta_key='stripe_customer_id' has no classification
    run: brama anonymize init

$ echo $?
42
```

Every comparable tool — Greenmask, PostgreSQL Anonymizer, Snaplet, Percona — passes columns
it has no rule for straight through, unmasked. A column added by a migration six months from
now is exactly how a leak happens, so brama stops instead.

WordPress hides most of its personal data in key/value tables, so classification can key off
`meta_key` and `option_name`, not just the column. A bundled WordPress + WooCommerce preset
covers the standard schema — you classify only what's yours.

## Files, too

`brama files pull` rebuilds your uploads directory out of placeholders: images generated at
their real dimensions so layouts and `srcset` behave, valid empty files for PDFs, archives
and video. Nothing 404s, nothing crashes a parser, and no signed contract or passport scan
leaves the server.

A 2.6 GB uploads folder transfers as a few megabytes — which makes it the fast option as
well as the safe one.

## What it refuses to do

- **Move data upward.** Code goes up, data comes down. There is no `push`.
- **Pull anything unclassified.** See above.
- **Let `--non-interactive` mean "ignore everything".** Suppressing prompts never suppresses
  a guardrail — that's the standard mistake in automation tooling.
- **Hold your credentials.** Authentication is OpenSSH's: `~/.ssh/config`, `ssh-agent`,
  hardware keys. Brama stores no secrets.
- **Run arbitrary shell commands on your server.**

## For AI agents

An agent shouldn't have `ssh root@server`. It should have one command with a guardrail
behind it:

```console
$ brama db pull production local --json --non-interactive
```

Refusals are a documented outcome with their own exit code, so an agent can tell *"I'm not
allowed"* from *"something broke"* — and can't argue its way past the first one.

**The honest limit:** this holds only while the agent doesn't also hold an unrestricted SSH
key to the same server. `brama doctor` warns when it finds one. Brama narrows the path an
agent takes; it can't take away credentials already lying around on the machine.

## Roadmap

| | |
|---|---|
| **v0.1** | Anonymized downward sync — `db pull`, `files pull`. WordPress. MySQL + PostgreSQL. |
| **v0.2** | Safe deployment — releases, rollback, recovery points. Forced-command hardening, identities and policy. Laravel. |
| **v0.3** | Environment control plane — `promote`, `diff`, declarative policy, subsetting. |

Tracked as [issues](https://github.com/bramaos/brama/issues), grouped by
[milestone](https://github.com/bramaos/brama/milestones).

## Design documents

| | |
|---|---|
| [`docs/product-description.md`](docs/product-description.md) | What Brama is, what v0.1 contains, and why |
| [`CONTEXT.md`](CONTEXT.md) | The vocabulary, used exactly, in code and docs |
| [`docs/adr/`](docs/adr/) | Decisions that were hard to reverse, and what they cost |

## License

[Apache-2.0](LICENSE).
