// Package arch holds tests that enforce architectural boundaries rather than
// behaviour. They exist because these rules are invisible at a call site: nothing
// stops a package importing lipgloss except this test failing.
package arch_test

import (
	"go/build"
	"path"
	"slices"
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
		"internal/anonymize",
		"internal/config",
		"internal/adapter",
		"internal/adapter/wordpress",
		"internal/scaffold",
		"internal/generator",
		"internal/preset",
		"internal/refusal",
		"internal/schema",
		"internal/schema/catalog",
		"internal/schema/mysql",
		"internal/schema/postgres",
		"internal/shim",
		"internal/ssh",
		"internal/state",
		"internal/version",
	}

	for _, pkg := range corePackages {
		t.Run(pkg, func(t *testing.T) {
			for _, imported := range transitiveImports(t, module+"/"+pkg) {
				for _, bad := range cliOnly() {
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

// The Shim runs on a Server and links only the core (docs/agents/rules/architecture.md).
// TestCoreDoesNotImportARenderer checks the packages listed in corePackages. This
// test starts at the Shim's main package instead, so it also catches a package that
// the Shim imports and that nobody added to that list.
func TestShimLinksOnlyTheCore(t *testing.T) {
	const shim = "cmd/brama-shim"
	var linked []string
	for _, imported := range transitiveImports(t, module+"/"+shim) {
		for _, bad := range cliOnly() {
			if strings.HasPrefix(imported, bad) {
				linked = append(linked, imported)
			}
		}
	}
	if len(linked) == 0 {
		return
	}
	slices.Sort(linked)
	t.Errorf("%s links packages that only the CLI may use:\n\t%s\n\n"+
		"Why: the Shim runs on a Server and links only the core. It is uploaded over SSH\n"+
		"and version-checked on every operation, and it has no terminal to render to.\n"+
		"Fix: remove the import that brings these in. Move the code the Shim needs into a\n"+
		"core package that imports no renderer, CLI or terminal package, and add that\n"+
		"package to corePackages in internal/arch/arch_test.go.",
		shim, strings.Join(linked, "\n\t"))
}

// cliOnly returns the import path prefixes that only the CLI may use: the commands,
// the renderer, the keyboard prompts, Cobra, and every Charm package.
func cliOnly() []string {
	return []string{
		module + "/internal/cli",
		module + "/internal/renderer",
		module + "/internal/prompt",
		"charm.land/",
		"github.com/charmbracelet/",
		"github.com/spf13/cobra",
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
