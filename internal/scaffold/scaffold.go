// Package scaffold writes the starting brama.yaml.
//
// The skeleton is generated as text rather than marshalled from a struct, because
// most of its value is in the comments: the commented-out production and servers
// blocks show the shape of the file without a docs lookup, and an unresolved path is
// written as a comment naming the candidates rather than as a guess.
//
// Later writes — `brama server add`, `brama anonymize init` — edit the YAML tree so
// this commentary survives. See docs/adr/0005-one-committed-config-file.md.
package scaffold

import (
	"fmt"
	"strings"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/config"
)

// Skeleton renders the starting brama.yaml for a detected project.
//
// Anything detection could not determine is written commented out, with the reason
// and any candidates, so the file is something to edit rather than something to
// retype. Nothing is guessed.
func Skeleton(result adapter.Detection) []byte {
	var b strings.Builder

	b.WriteString("# brama.yaml — this project's desired state, committed to the repo.\n")
	b.WriteString("#\n")
	b.WriteString("# It declares what this project is and where its environments live. It never\n")
	b.WriteString("# holds credentials: authentication is OpenSSH's, and environment secrets belong\n")
	b.WriteString("# to the server.\n")
	b.WriteString("\n")
	fmt.Fprintf(&b, "version: %d\n", config.SchemaVersion)
	b.WriteString("\n")

	b.WriteString("app:\n")
	fmt.Fprintf(&b, "  adapter: %s\n", result.Adapter)
	b.WriteString("\n")
	b.WriteString("  # Paths are relative to each environment's root. An environment may override\n")
	b.WriteString("  # any of them key by key.\n")
	b.WriteString("  paths:\n")
	writePath(&b, result, "config", result.Paths.Config,
		"holds the database credentials")
	writePath(&b, result, "uploads", result.Paths.Uploads,
		"rebuilt out of placeholders by files pull")
	b.WriteString("\n")

	b.WriteString("environments:\n")
	b.WriteString("  local:\n")
	if url, ok := unresolvedFor(result, "environments.local.url"); ok {
		writeUnresolved(&b, "    ", "url", url)
	} else {
		fmt.Fprintf(&b, "    url: %s\n", result.LocalURL)
	}
	b.WriteString("\n")
	b.WriteString("  # An environment that names a server is reached over SSH, and needs its root\n")
	b.WriteString("  # path there. One that names no server is this machine, rooted at this file.\n")
	b.WriteString("  # Add remote environments with: brama server add\n")
	b.WriteString("  #\n")
	b.WriteString("  # production:\n")
	b.WriteString("  #   server: prod\n")
	b.WriteString("  #   path: /var/www/app\n")
	b.WriteString("  #   url: https://example.com\n")
	b.WriteString("\n")

	b.WriteString("# servers:\n")
	b.WriteString("#   prod:\n")
	b.WriteString("#     host: prod.example.com   # a hostname, an IP, or a ~/.ssh/config alias\n")
	b.WriteString("#     user: deploy             # optional — without it, OpenSSH decides\n")
	b.WriteString("\n")

	b.WriteString("# No anonymize block yet, which means every column is unclassified and the first\n")
	b.WriteString("# pull will refuse. Classify them with: brama anonymize init\n")

	return []byte(b.String())
}

// pathCommentColumn is where the trailing explanations line up. Wider values simply
// push their own comment out; the file stays valid either way.
const pathCommentColumn = 34

// writePath emits one path key, or a commented explanation of why it is missing.
func writePath(b *strings.Builder, result adapter.Detection, key, value, purpose string) {
	if value != "" {
		entry := fmt.Sprintf("    %s: %s", key, value)
		fmt.Fprintf(b, "%s  # %s\n", padTo(entry, pathCommentColumn), purpose)
		return
	}
	u, ok := unresolvedFor(result, "app.paths."+key)
	if !ok {
		u = adapter.Unresolved{Key: key, Why: "not determined"}
	}
	writeUnresolved(b, "    ", key, u)
}

// writeUnresolved renders a key brama could not fill in: commented out, with the
// reason and whatever candidates were found. The file is left invalid on purpose —
// init exits non-zero, so nothing downstream runs against a value nobody chose.
func writeUnresolved(b *strings.Builder, indent, key string, u adapter.Unresolved) {
	fmt.Fprintf(b, "%s# %s: NOT DETERMINED — %s\n", indent, key, u.Why)
	if len(u.Candidates) == 0 {
		fmt.Fprintf(b, "%s# Fill this in by hand, then rerun.\n", indent)
		return
	}
	fmt.Fprintf(b, "%s# Candidates found — uncomment one:\n", indent)
	for _, c := range u.Candidates {
		fmt.Fprintf(b, "%s# %s: %s\n", indent, key, c)
	}
}

func unresolvedFor(result adapter.Detection, key string) (adapter.Unresolved, bool) {
	for _, u := range result.Unresolved {
		if u.Key == key {
			return u, true
		}
	}
	return adapter.Unresolved{}, false
}

func padTo(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
