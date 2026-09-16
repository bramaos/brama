// Package arch holds tests that enforce architectural boundaries rather than
// behaviour. They exist because these rules are invisible at a call site: nothing
// stops a package importing lipgloss except this test failing.
package arch_test

import (
	"go/build"
	"path"
	"strings"
	"testing"
)

const module = "github.com/bramaos/brama"

// The core never formats. Two things depend on this holding:
//
// --json cannot be forgotten, because a command that returns a Result never had the
// option of printing prose instead; and cmd/brama-shim can link the core without
// linking a terminal UI it could never use — which matters because the Shim is
// uploaded over SSH and version-checked on every operation.
//
// See docs/product-description.md, "The rendering rule".
func TestCoreDoesNotImportARenderer(t *testing.T) {
	corePackages := []string{
		"internal/config",
		"internal/adapter",
		"internal/adapter/wordpress",
		"internal/scaffold",
		"internal/refusal",
		"internal/shim",
		"internal/ssh",
	}

	forbidden := []string{
		module + "/internal/renderer",
		module + "/internal/cli",
		"charm.land/",
		"github.com/charmbracelet/",
		"github.com/spf13/cobra",
	}

	for _, pkg := range corePackages {
		t.Run(pkg, func(t *testing.T) {
			for _, imported := range transitiveImports(t, module+"/"+pkg) {
				for _, bad := range forbidden {
					if strings.HasPrefix(imported, bad) {
						t.Errorf("%s imports %s\n\n"+
							"Nothing under core/, schema/, config/ or executor/ may import a\n"+
							"renderer, the CLI, or a terminal package. See the rendering rule in\n"+
							"docs/product-description.md.", pkg, imported)
					}
				}
			}
		})
	}
}

// transitiveImports returns every package reachable from pkg, excluding the standard
// library.
func transitiveImports(t *testing.T, pkg string) []string {
	t.Helper()

	seen := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] || isStdlib(name) {
			return
		}
		seen[name] = true

		p, err := build.Import(name, "", 0)
		if err != nil {
			// Reported, never swallowed. A package this test cannot read is a
			// package it cannot vouch for, and a guard that passes when it fails
			// to look is worse than no guard.
			t.Errorf("cannot resolve %s: %v", name, err)
			return
		}
		for _, next := range p.Imports {
			walk(next)
		}
	}
	walk(pkg)

	out := make([]string, 0, len(seen))
	for name := range seen {
		if name != pkg {
			out = append(out, name)
		}
	}
	return out
}

// isStdlib reports whether an import path belongs to the standard library. Stdlib
// paths have no dot in their first segment: "net/http" versus "charm.land/fang/v2".
func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(path.Clean(importPath), "/")
	return !strings.Contains(first, ".")
}
