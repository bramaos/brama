// Package testenv is a rig of two containerised Servers, and the tests that use them.
//
// Every other test in this repository reaches a Server through a fake: a struct that
// records the commands it was given and answers with strings a test author chose.
// That proves brama sends what it means to send. It cannot prove the Server does
// anything with it — that `uname -sm` answers what the platform parser expects, that
// a symlink rename is atomic on a real filesystem, or that a Shim written by one
// user is executable by that user. The Servers in this directory answer those.
//
// A Server here is an Ubuntu container running systemd and a real sshd. brama
// reaches it through the `ssh` binary, via a `~/.ssh/config` alias, with no
// production code changed and no credential stored anywhere — the same path, byte
// for byte, that reaches a rented Server.
//
// The tests are behind the `testenv` build tag, so `make test` and `make check`
// never compile them and need no container:
//
//	make testenv-up      # build the image, start both Servers
//	make testenv-seed    # load the deterministic WordPress data
//	make testenv-test    # run the tests in this package
//	make testenv-down    # stop and remove them
//
// See README.md in this directory for the one manual step — an `Include` line in
// ~/.ssh/config — and docs/adr/0008-test-against-containerised-servers.md for why
// the rig runs privileged and why it is not in CI.
package testenv
