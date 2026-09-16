// Package state reads and writes an Environment's Observed state.
//
// Observed state is what brama recorded about an Environment the last time it acted
// on it, and it lives on the Server, under the Environment it describes — not in the
// repo. brama.yaml is Desired state, owned by developers through git; the two are
// never merged. See docs/product-description.md, "State ownership".
//
// The Shim binary is the one thing on a Server that is not per-Environment, so it is
// the one thing this package records that lives elsewhere: `~/.brama/shim` is
// server-global, and state.json only remembers which build an operation here ran
// against. See docs/adr/0006-server-global-shim-home.md.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/bramaos/brama/internal/shim"
)

// SchemaVersion is the only `version` this build of brama writes, and the highest it
// will read. A file declaring more is rejected rather than parsed optimistically: the
// schema moves before 1.0, and "brama is too old" beats silently discarding a key
// this build does not know about.
const SchemaVersion = 1

// Dir is the Environment's own brama directory, on its Server.
const Dir = ".brama"

// Filename is the Observed state within it.
const Filename = "state.json"

// KeptChanges bounds the Shim history. A Server reached for years is a Server with an
// unbounded list in its state.json, and recording one line must never be the thing
// that fills a disk. Twenty is far more than anyone reads and far less than anyone
// notices.
const KeptChanges = 20

// Remote is the part of a Server this package needs. Declared here, where it is
// consumed, rather than by the transport: internal/ssh stays a concrete package with
// no interface of its own to keep in step.
type Remote interface {
	// Run executes a command and returns its standard output.
	Run(ctx context.Context, command string) (string, error)
	// Send streams data to a command's standard input.
	Send(ctx context.Context, command string, data []byte) error
}

// State is one Environment's Observed state.
type State struct {
	Version int `json:"version"`
	// Shim is absent until brama has run one here.
	Shim *Shim `json:"shim,omitempty"`
}

// Shim is the build an operation on this Environment last ran against, and how it
// came to be that one.
type Shim struct {
	// Version is what the Shim on the Server reports now.
	Version string `json:"version"`
	// Platform is the Server it was built for.
	Platform string `json:"platform,omitempty"`
	// Changes are the version checks that changed something, oldest first. One that
	// changed nothing adds none: an Environment reached daily would otherwise bury
	// the entries that matter under years of entries saying nothing happened.
	Changes []ShimChange `json:"changes,omitempty"`
}

// ShimChange is one visible version-check step that moved the Shim: what the Server
// was running, what replaced it, and when.
type ShimChange struct {
	At time.Time `json:"at"`
	// From is omitted for a first install — there was nothing to come from, and
	// `"from": ""` would read as a version.
	From string `json:"from,omitempty"`
	To   string `json:"to"`
	// Change is what the difference amounted to: install, upgrade, or replace. It is
	// shim.Change rather than a string so that what lands in state.json can only ever
	// be one of the outcomes the version check can produce.
	Change   shim.Change `json:"change"`
	Platform string      `json:"platform,omitempty"`
}

// RecordShim notes the version check an operation took before running.
//
// A Step that was reported rather than taken — what `--dry-run` produces — updates
// nothing and records nothing. Observed state is what brama did, and a dry run did
// not do it; leaving that judgement to each caller is how a `--dry-run` eventually
// writes to a Server.
func (s *State) RecordShim(step shim.Step, at time.Time) {
	if step.Pending {
		return
	}

	if s.Shim == nil {
		s.Shim = &Shim{}
	}
	s.Shim.Version = step.Running()
	if step.Platform.OS != "" {
		s.Shim.Platform = step.Platform.String()
	}

	if !step.Changed() {
		return
	}
	s.Shim.Changes = append(s.Shim.Changes, ShimChange{
		At:       at.UTC(),
		From:     step.From,
		To:       step.To,
		Change:   step.Change,
		Platform: s.Shim.Platform,
	})
	if extra := len(s.Shim.Changes) - KeptChanges; extra > 0 {
		s.Shim.Changes = append([]ShimChange(nil), s.Shim.Changes[extra:]...)
	}
}

// Load reads an Environment's Observed state from its Server.
//
// envPath is the Environment's root there. An Environment brama has never acted on
// has no state.json, which is not an error: it is a fact about the Environment, and
// the caller's next move is to record one. A state.json that is there but unreadable
// is an error, because overwriting a history brama could not parse loses it.
func Load(ctx context.Context, r Remote, envPath string) (*State, error) {
	file := filePath(envPath)

	// `if [ -e ]` rather than a bare cat: absent must be distinguishable from
	// unreadable, and a cat that fails on permissions has to reach the caller.
	out, err := r.Run(ctx, "if [ -e "+quote(file)+" ]; then cat "+quote(file)+"; fi")
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	if strings.TrimSpace(out) == "" {
		return &State{Version: SchemaVersion}, nil
	}

	var s State
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return nil, fmt.Errorf("%s on the server is not readable as %s: %w", file, Filename, err)
	}
	if s.Version > SchemaVersion {
		return nil, fmt.Errorf(
			"%s declares version %d, but this brama understands %d — upgrade brama",
			file, s.Version, SchemaVersion)
	}
	s.Version = SchemaVersion
	return &s, nil
}

// Save writes an Environment's Observed state back to its Server.
//
// The write lands by rename, for the same reason the Shim's does: state.json is the
// Server-side source of truth, and a half-written one is worse than a stale one.
func Save(ctx context.Context, r Remote, envPath string, s *State) error {
	s.Version = SchemaVersion

	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", Filename, err)
	}
	body = append(body, '\n')

	var (
		dir    = path.Join(envPath, Dir)
		file   = filePath(envPath)
		staged = path.Join(dir, "."+Filename+".part")
	)
	command := strings.Join([]string{
		"set -e",
		"mkdir -p " + quote(dir),
		"cat > " + quote(staged),
		"mv -f " + quote(staged) + " " + quote(file),
	}, " && ")

	if err := r.Send(ctx, command, body); err != nil {
		return fmt.Errorf("writing %s: %w", file, err)
	}
	return nil
}

func filePath(envPath string) string { return path.Join(envPath, Dir, Filename) }

// quote makes a value safe inside a single-quoted shell word. An Environment's path
// comes out of brama.yaml, which anything that can write to the repo can edit, and a
// path that reaches a shell unquoted is how a path becomes a command.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
