// Command brama-shim is the component that runs on a Server.
//
// It is not a daemon. It is uploaded over SSH, runs for the duration of one
// operation, and is the only thing that ever reads raw production data.
//
// This build answers `--version` and nothing else. That is enough to prove the
// install path end to end — cross-build, transfer, chmod, atomic swap, remote exec —
// which is the part `brama server add` depends on. The operations it will carry
// arrive with the commands that need them.
//
// It deliberately links no terminal UI: a Shim crossing the wire on every operation
// should not carry a renderer it can never use. See the rendering rule in
// docs/product-description.md.
package main

import (
	"fmt"
	"os"
)

// version is stamped at link time, from the same tag as the brama that embeds this
// binary. brama compares what it shipped against what answers on the Server, so the
// two must come from one build.
var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(version)
		return
	}

	fmt.Fprintf(os.Stderr, "brama-shim %s\n", version)
	fmt.Fprintln(os.Stderr, "usage: brama-shim --version")
	os.Exit(1)
}
