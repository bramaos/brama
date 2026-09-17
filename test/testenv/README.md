# Containerised Servers

Two real **Servers** — Ubuntu 24.04, systemd, a real `sshd`, MariaDB, Apache, PHP-FPM,
WordPress on Bedrock with WooCommerce — that brama reaches exactly as it reaches a
rented Server: through the `ssh` binary, via a `~/.ssh/config` alias, with no brama
source changed and no credential stored.

Why they exist, and why they are not in CI:
[docs/adr/0008-test-against-containerised-servers.md](../../docs/adr/0008-test-against-containerised-servers.md).

| | |
|---|---|
| `brama-testenv-production` | 60 users, 40 products, 25 orders · <http://localhost:8081> · ssh on 2201 |
| `brama-testenv-staging` | 12 users, 8 products, 5 orders · <http://localhost:8082> · ssh on 2202 |

Both listen on the loopback interface only, log in as `deploy` with passwordless
sudo, and are the same image.

Alongside them, and not a Server: `brama-testenv-postgres`, a stock PostgreSQL on
5433. It has no site, no `sshd` and no seed. It exists so that the PostgreSQL
Introspector is measured against a real server rather than a fake, which WordPress —
MySQL only — gives no other way to do.

## Prerequisites

- **Docker with cgroup v1.** Check with `docker info | grep -i 'cgroup version'`. The
  Servers boot systemd as PID 1, and the invocation in `compose.yml` — privileged,
  tmpfs `/run`, the host hierarchy at `/sys/fs/cgroup` — is the one that works on v1.
  A v2 host needs the unified-hierarchy recipe instead, which this rig does not use.
- **Docker Desktop with WSL integration**, if you are on Windows. Run everything from
  inside WSL, not from PowerShell. Two things follow from it:
  - `make testenv-pull` fetches the base images before the build. `docker build`
    resolves `FROM` through the client's credential helper, which on this setup is
    `docker-credential-desktop.exe` — a Windows binary WSL cannot execute. `docker
    pull` goes through the daemon and has no such problem, so pulling first leaves
    the build nothing to resolve. `make testenv-up` does this for you.
  - The rig needs roughly 3 GB of RAM and 2 CPUs across both Servers. Raise the WSL
    limits in `.wslconfig` if Docker Desktop is squeezed.
- **About 4 GB of disk** for the image, and five to ten minutes for the first build.

## Bring it up

```sh
make testenv-up      # generate the keypair, build the image, start both Servers
```

It finishes by printing an `Include` line. **Add it to the top of `~/.ssh/config`
yourself** — nothing writes to that file for you, because that file grants production
access:

```
Include /path/to/brama/test/testenv/ssh/config
```

It has to be at the top: OpenSSH keeps the first value it obtains for each keyword.
Remove it when you are done with the rig.

Then:

```sh
ssh brama-testenv-production      # logs in as deploy, no password, no ssh-agent
make testenv-seed                 # load the WordPress data (a few minutes)
make testenv-test                 # run the tests that need the rig
make testenv-down                 # stop and remove both Servers
```

`make testenv-shell SERVER=staging` opens a root shell on a Server without SSH.

## What is where

| Path | |
|---|---|
| `Dockerfile` | the Server image: `base` (OS + LAMP) and `server` (users, units, site) |
| `compose.yml` | the two Servers and the PostgreSQL, their ports and their limits |
| `rootfs/` | files copied into the image at their final paths |
| `seed/seed.sh` | resets the database and installs WordPress + WooCommerce |
| `seed/seed.php` | generates the rows, in one `wp eval-file` rather than one per row |
| `bin/ssh-setup.sh` | generates the keypair and the ssh config fragment |
| `ssh/` | generated, gitignored: keypair, config fragment, `known_hosts` |
| `*_test.go` | behind the `testenv` build tag — see `doc.go` |

On a Server: the site is at `/srv/www/bedrock` owned by `deploy`, the database is
`wordpress` reachable over the local unix socket, and the Shim installs to
`/home/deploy/.brama/`.

MariaDB also listens on the container's own loopback interface, and no database port
is published to the host — the same as a rented Server. `schema_test.go` reaches it
with an `ssh -L` tunnel of its own, which is the test's plumbing and not brama's: on a
Server the introspector runs inside the Shim, with the database already local.

`brama-testenv-postgres` does publish its port, on `127.0.0.1:5433`. It is not a
Server and has nothing a tunnel would protect — no shell, no sudo, and a database
`schema_postgres_test.go` creates and drops its own fixture in. Override the port with
`POSTGRES_PORT` if 5433 is taken; the Makefile exports it to both compose and the
tests, so setting it once moves both.

## Reset

`make testenv-seed` is also the reset. It drops the database before it does anything
else, so a Server you have broken by hand comes back to a known state without a rebuild
— experiment on it freely.

`make testenv-down && make testenv-up` rebuilds the Servers themselves. Nothing is
persisted in a volume on purpose: a Server that survives a teardown is a Server whose
state nobody can account for.

## What "deterministic" covers

Seeding twice produces byte-identical `wp_users`, `wp_usermeta`, `wp_posts`,
`wp_postmeta` and `wp_woocommerce_order_items`. That is what a test comparing a dump
before and after a Pull will rely on, and `seed.php` goes out of its way to get there
— WordPress and WooCommerce stamp the clock onto their own install artifacts, and
those stamps are normalised at the end of the seed.

Two things are still clock-bound and deliberately left alone: `wp_options`, which
holds the cron schedule, and `wp_users.user_pass`, which WordPress salts. Exclude
them from any comparison.

## The data, and what it is for

The seed is shaped by what Anonymization will need to refuse and to keep, not by what
a demo site looks like:

- **Discriminator spread.** 35 distinct `wp_usermeta.meta_key` values, chosen so that
  one column holds values needing different Classifications — `billing_email` would
  be faked, `billing_country` kept, `session_tokens` dropped. Three keys could not
  tell those cases apart.
- **A planted Unclassified column.** `wp_users.legacy_crm_reference`, holding
  something that looks like personal data. Presets will eventually cover every column
  WordPress and WooCommerce ship, so a tidy seed could never produce a **Refusal** —
  this is the untidiness a real site has.
- **Two sizes.** Staging holds real rows so that overwriting them is something a
  **Recovery point** can be proved against. An empty target would prove nothing.

No Sync, Anonymization, Classification or Placeholder behaviour is asserted yet —
those operations do not exist. The rig prepares their target.

## Credentials

There are none worth protecting, and none are committed.

The SSH keypair is generated into `ssh/`, which is gitignored in full — public key
included, since a committed one would be an `authorized_keys` entry shared by every
checkout. The WordPress admin is `admin` / `brama-testenv` and the database password
is `brama-testenv`, both in plain sight in `compose.yml` and `seed.sh`: the Servers
listen on loopback only and hold nothing but rows this directory generated.

## Troubleshooting

**`make testenv-up` hangs at "Waiting" and then reports unhealthy.** The healthcheck
requires `systemctl is-system-running` to say `running`, not `degraded`. Look at what
failed:

```sh
docker exec brama-testenv-production systemctl --failed
docker exec brama-testenv-production systemctl status <unit> -l
```

**`ssh brama-testenv-production` cannot resolve the host.** The `Include` line is
missing from `~/.ssh/config`, or it is below an earlier `Host *` block. `ssh -G
brama-testenv-production` shows what OpenSSH actually resolved — if `port` is 22, the
fragment is not being read.

**Host key warnings after a rebuild.** Each rebuild gives the Servers new host keys.
They are pinned in `ssh/known_hosts`, which is the rig's own file and never yours;
`make testenv-down` deletes it.

**`make testenv-test` fails before connecting.** It says which alias did not resolve
and what to do. The tests fail rather than skip: nothing compiles them without
someone asking for the `testenv` build tag.
