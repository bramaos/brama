package main

import (
	"os"

	"github.com/bramaos/brama/internal/cli"
)

// version is overwritten at build time via -ldflags. See the toolchain reference.
var version = "dev"

// main does nothing but choose the exit code, so that every command can return an
// error normally and let its deferred cleanup run.
func main() {
	os.Exit(cli.Main(version))
}
