package main

import (
	"fmt"
	"os"
)

// version is overwritten at build time via -ldflags. See the toolchain reference.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("brama", version)
		return
	}

	fmt.Fprintln(os.Stderr, "usage: brama <command>")
	os.Exit(2)
}
