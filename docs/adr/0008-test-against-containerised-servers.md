# Test against containerised Servers

Brama's whole value is what happens on a Server, and until now
none of it was tested against one. The SSH Executor and the Shim install were
exercised only by in-process fakes. We add a rig of two real Servers — Ubuntu
containers running systemd and a real `sshd` — and reach them exactly as we reach a
rented Server: through the `ssh` binary, via a `~/.ssh/config` alias, with no
production code changed.

The fakes are good at what they are for and will stay. A `fakeServer` records the
commands it was handed and answers with strings the test author chose, which proves
brama sends what it means to send. What it cannot prove is that anything happens at
the other end. Every interesting claim the Shim install makes is a claim about a
Server: that `uname -sm` answers in the two fields the platform parser expects; that
`mv` over an existing symlink is atomic on a real filesystem rather than an unlink
followed by a create; that a binary streamed through `cat >` and chmodded is
executable afterwards by the unprivileged user who owns the home directory it landed
in; that the version it prints when *run* is the version brama sent. A fake answers
all four by construction — it returns whatever was typed into the test — so the
assertions were tautologies, and the four that mattered were the four not being made.

The Servers run privileged, with the host cgroup hierarchy mounted writable and tmpfs
at `/run`. That is the opposite of what a container should do, and it is the cost of
the one thing the rig exists for: systemd as PID 1. A Server, as CONTEXT.md defines
it, is a registered SSH host, and the Servers Brama is pointed at boot their
database and their web server as units. A container running `sshd` under a shell
wrapper would be reachable but would not be that; the Shim would find no `mysqldump`
under a service manager, and the ordering problems a real boot has would be invisible.
MariaDB is inside each Server for the same reason — the Shim finds it locally over a
unix socket, as on a rented Server, rather than over TCP to a sibling container. The
Docker engine here reports cgroup v1, so the modern unified-hierarchy recipe does not
apply and the privileged invocation is the one that was tested to boot.

The rig is not in CI. It is minutes of `apt` and `composer` per build, it needs a
privileged container, and it needs a developer to paste an `Include` line into a file
CI does not have. Running it on every push would buy less than it costs while the
operations it prepares for — Sync, Anonymization, Refusal — do not exist yet. The
trade-off is real and is accepted: nothing stops a change breaking the rig silently,
and it will be found the next time someone runs it rather than on the commit that
did it.

Two Servers rather than one, and staging seeded smaller than production, because a
Pull needs a target that already holds rows — overwriting an empty database would
prove nothing about a Recovery point. The seed plants an Unclassified column
deliberately, because Presets will eventually cover every column WordPress and
WooCommerce ship, and a rig built only from those could never produce the Refusal
path that Classifications exist to trigger.

## Consequences

- A developer can bring up two Servers, register them with `brama server add`, watch
  the Shim install itself over a genuine SSH connection, and then `ssh` in by hand to
  see what it did. Inspecting the result is the point, not a side effect.
- The rig's tests are behind a `testenv` build tag and a `make testenv-test` target.
  `make test` and `make check` compile none of it and need no container, so the fast
  path stays fast and CI stays unchanged.
- No credential is stored. `make testenv-ssh-setup` generates a gitignored keypair and
  a separate ssh config fragment using `IdentityFile` with `IdentitiesOnly yes`, and
  **prints** the `Include` line rather than writing to `~/.ssh/config` — that file
  grants production access, and a script that edits it is a script that can break it.
- The Servers are amd64 only. The arm64 Shim stays validated as an ELF and unexecuted,
  as it was before.
- The `local` Environment and the direct Executor are not covered. Reach is derived
  from whether an Environment names a Server, so a third container would be
  SSH-reached and would test the direct path zero times. That waits for `db pull`,
  when the receiving side's needs are known.
- No new term enters CONTEXT.md. A container running `sshd` already *is* a Server
  under the existing definition — which is precisely why the rig is worth building —
  and the glossary stays free of test infrastructure.
