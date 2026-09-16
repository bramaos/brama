# A server-global home for the Shim

The Server layout put the Shim at `<environment path>/.brama/shim`, alongside `state.json`.
We moved the binary to `~/.brama/shim` — the home directory of the SSH user — and left
`state.json` where it was.

`brama server add` is what forced the question. It registers a Server and installs the Shim in
the same operation, and at that moment no Environment need name that Server yet: the entry it
writes is what a later `environments.production.server: prod` will point at. A path derived
from an Environment is a path `server add` cannot compute. The alternatives were to make
registration refuse until an Environment exists — an ordering constraint on a file humans
write by hand, in whatever order they like — or to drop the install from registration and
leave the first Pull to discover the Server has no Shim.

The binary is also the one thing on the Server that is not per-Environment. It is the same
build for every Environment on the box, it carries no state, and it is replaced wholesale on
upgrade. Storing one copy per Environment means one copy per Environment to upgrade, and a
Server hosting staging and production would hold two identical binaries that must not drift.

## Consequences

- One Shim serves every Environment on a Server. Two projects registering the same host share
  it, so the second registration finds a current binary and skips the transfer — it still runs
  `shim --version`, because executing it is what proves the install.
- Observed state stays per-Environment. `state.json` remains under the Environment's
  `.brama/`, which is what it describes; only the executable moved.
- The Shim's home is the SSH user's home, so two Identities on one machine get their own. That
  follows from Policy living on the Server rather than in `brama.yaml`, and it means an
  upgrade by one user cannot change what another user's operations execute.
- Installing is no longer coupled to the Environment layout, which is what lets `server add`
  verify a Server end to end — connect, detect, install, run — before writing anything. A
  `servers:` entry therefore always means "reachable, and the Shim runs here".
- Versioned filenames with a symlink, rather than one overwritten file: `shim-0.1.0` beside
  `shim` pointing at it. A shared binary is one an interrupted upload must never corrupt, so
  the swap is a rename, and the version is readable without executing anything. Two builds are
  kept.
