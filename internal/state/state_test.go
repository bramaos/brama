package state_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bramaos/brama/internal/shim"
	"github.com/bramaos/brama/internal/state"
)

var linuxAMD64 = shim.Platform{OS: "linux", Arch: "amd64"}

// upgrade is the step a version check produces when it moved the Shim.
func upgrade(from, to string) shim.Step {
	return shim.Step{Platform: linuxAMD64, From: from, To: to, Change: shim.ChangeUpgrade}
}

const envPath = "/var/www/acme"

var at = time.Date(2026, 9, 16, 10, 30, 0, 0, time.UTC)

// server stands in for a Server: a filesystem reached over a connection, with just
// enough shell to answer the two commands this package issues.
type server struct {
	files  map[string]string
	runErr error
	log    []string
}

func newServer() *server { return &server{files: map[string]string{}} }

func (s *server) Run(_ context.Context, command string) (string, error) {
	s.log = append(s.log, command)
	if s.runErr != nil {
		return "", s.runErr
	}
	path, ok := readPath(command)
	if !ok {
		return "", nil
	}
	return s.files[path], nil
}

func (s *server) Send(_ context.Context, command string, data []byte) error {
	s.log = append(s.log, command)
	if s.runErr != nil {
		return s.runErr
	}
	path, ok := writePath(command)
	if !ok {
		return errors.New("nothing to write to in: " + command)
	}
	s.files[path] = string(data)
	return nil
}

// readPath pulls the file out of `if [ -e '<path>' ]; then cat '<path>'; fi`.
func readPath(command string) (string, bool) {
	_, rest, found := strings.Cut(command, "cat '")
	if !found {
		return "", false
	}
	path, _, _ := strings.Cut(rest, "'")
	return path, true
}

// writePath pulls the destination out of the staged write, following the `mv` so the
// stand-in stores the file under the name it ends up with.
func writePath(command string) (string, bool) {
	_, rest, found := strings.Cut(command, "mv -f ")
	if !found {
		return "", false
	}
	_, rest, found = strings.Cut(rest, "' '")
	if !found {
		return "", false
	}
	path, _, _ := strings.Cut(rest, "'")
	return path, true
}

func (s *server) ran(substr string) bool {
	for _, c := range s.log {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func load(t *testing.T, s *server) *state.State {
	t.Helper()
	loaded, err := state.Load(context.Background(), s, envPath)
	if err != nil {
		t.Fatalf("Load() = %v, want success", err)
	}
	return loaded
}

func save(t *testing.T, s *server, st *state.State) {
	t.Helper()
	if err := state.Save(context.Background(), s, envPath, st); err != nil {
		t.Fatalf("Save() = %v, want success", err)
	}
}

// An Environment brama has never acted on has no Observed state. That is a fact about
// the Environment, not a failure to read it.
func TestLoadOfAnEnvironmentNeverActedOn(t *testing.T) {
	st := load(t, newServer())

	if st.Version != state.SchemaVersion {
		t.Errorf("Version = %d, want %d", st.Version, state.SchemaVersion)
	}
	if st.Shim != nil {
		t.Errorf("Shim = %+v, want nil — nothing has been recorded yet", st.Shim)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	s := newServer()

	st := load(t, s)
	st.RecordShim(upgrade("0.1.0", "0.2.0"), at)
	save(t, s, st)

	back := load(t, s)
	if back.Shim == nil {
		t.Fatal("Shim = nil after a save that recorded one")
	}
	if back.Shim.Version != "0.2.0" {
		t.Errorf("Shim.Version = %q, want 0.2.0", back.Shim.Version)
	}
	if back.Shim.Platform != "linux/amd64" {
		t.Errorf("Shim.Platform = %q, want linux/amd64", back.Shim.Platform)
	}
	if len(back.Shim.Changes) != 1 {
		t.Fatalf("Changes = %d, want 1", len(back.Shim.Changes))
	}
	if got := back.Shim.Changes[0]; got.From != "0.1.0" || got.To != "0.2.0" || got.Change != shim.ChangeUpgrade {
		t.Errorf("Changes[0] = %+v, want 0.1.0 → 0.2.0 (upgrade)", got)
	}
	if !back.Shim.Changes[0].At.Equal(at) {
		t.Errorf("At = %v, want %v", back.Shim.Changes[0].At, at)
	}
}

// The Shim version is the current one; the changes are how it got there. Both are
// wanted: "what is running" answers the next operation's question, "how it got here"
// answers the one asked after something breaks.
func TestRecordShimKeepsTheHistoryAndTheCurrentVersion(t *testing.T) {
	st := &state.State{Version: state.SchemaVersion}
	st.RecordShim(shim.Step{Platform: linuxAMD64, To: "0.1.0", Change: shim.ChangeInstall}, at)
	st.RecordShim(upgrade("0.1.0", "0.2.0"), at.Add(time.Hour))

	if st.Shim.Version != "0.2.0" {
		t.Errorf("Shim.Version = %q, want the latest", st.Shim.Version)
	}
	if len(st.Shim.Changes) != 2 {
		t.Fatalf("Changes = %d, want 2", len(st.Shim.Changes))
	}
	if st.Shim.Changes[1].To != "0.2.0" {
		t.Error("the newest change is not last, want them in the order they happened")
	}
	if st.Shim.Changes[0].Change != shim.ChangeInstall {
		t.Errorf("Changes[0].Change = %q, want install — a first install is not an upgrade",
			st.Shim.Changes[0].Change)
	}
}

// A Server reached for years is a Server with an unbounded list in its state.json.
// The history is bounded so recording one line can never be what fills a disk.
func TestRecordShimBoundsTheHistory(t *testing.T) {
	st := &state.State{Version: state.SchemaVersion}
	for i := 0; i < state.KeptChanges+5; i++ {
		st.RecordShim(upgrade("0.1.0", "0.2.0"), at.Add(time.Duration(i)*time.Hour))
	}

	if len(st.Shim.Changes) != state.KeptChanges {
		t.Errorf("Changes = %d, want %d", len(st.Shim.Changes), state.KeptChanges)
	}
	if !st.Shim.Changes[0].At.Equal(at.Add(5 * time.Hour)) {
		t.Errorf("oldest kept = %v, want the sixth — the earliest five are dropped",
			st.Shim.Changes[0].At)
	}
}

// An operation that found the Shim already current still records what it ran
// against — but it is not an event, and filling the history with entries saying
// nothing happened would bury the ones that did.
func TestRecordShimRecordsWhatRanWithoutAnEvent(t *testing.T) {
	st := &state.State{Version: state.SchemaVersion}
	st.RecordShim(shim.Step{
		Platform: shim.Platform{OS: "linux", Arch: "arm64"},
		From:     "0.2.0", To: "0.2.0", Change: shim.ChangeNone,
	}, at)

	if st.Shim.Version != "0.2.0" {
		t.Errorf("Shim.Version = %q, want 0.2.0 — it is what ran, change or not", st.Shim.Version)
	}
	if st.Shim.Platform != "linux/arm64" {
		t.Errorf("Shim.Platform = %q, want linux/arm64", st.Shim.Platform)
	}
	if len(st.Shim.Changes) != 0 {
		t.Errorf("Changes = %d, want none recorded for a step that changed nothing", len(st.Shim.Changes))
	}
}

// Observed state is what brama did. A --dry-run did not do it, and the only way a
// dry run can never write to a Server is if the recording refuses it rather than
// every caller remembering to.
func TestRecordShimIgnoresAPendingStep(t *testing.T) {
	st := &state.State{Version: state.SchemaVersion}
	pending := upgrade("0.1.0", "0.2.0")
	pending.Pending = true

	st.RecordShim(pending, at)

	if st.Shim != nil {
		t.Errorf("Shim = %+v, want nil — a dry run records nothing", st.Shim)
	}
}

// state.json is the Server-side source of truth. A half-written one is worse than a
// stale one, so the write lands by rename.
func TestSaveWritesByRename(t *testing.T) {
	s := newServer()
	save(t, s, &state.State{Version: state.SchemaVersion})

	if !s.ran("mv -f ") {
		t.Error("the write did not land by rename")
	}
	if !s.ran("mkdir -p '" + envPath + "/" + state.Dir + "'") {
		t.Errorf("the state directory was not created; commands were %v", s.log)
	}
}

// An Environment path comes out of brama.yaml, which anything that can write to the
// repo can edit. It reaches a shell, so it is quoted rather than trusted.
func TestPathsAreQuoted(t *testing.T) {
	s := newServer()
	hostile := "/var/www/a b'; rm -rf /"

	if _, err := state.Load(context.Background(), s, hostile); err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if s.ran("b'; rm -rf /") {
		t.Errorf("the quote in the path closed the shell's: %v", s.log)
	}
	if !s.ran(`'\''`) {
		t.Errorf("the quote in the path was not escaped: %v", s.log)
	}
}

// A state.json brama cannot parse is not an Environment with no history — it is an
// Environment whose history brama must not overwrite silently.
func TestLoadRefusesUnreadableState(t *testing.T) {
	s := newServer()
	s.files[envPath+"/"+state.Dir+"/"+state.Filename] = "{not json"

	_, err := state.Load(context.Background(), s, envPath)
	if err == nil {
		t.Fatal("Load() = nil, want a parse failure")
	}
	if !strings.Contains(err.Error(), state.Filename) {
		t.Errorf("error = %q, want it to name the file", err)
	}
}

// The schema moves before 1.0. A file from a newer brama is rejected rather than
// parsed optimistically, for the same reason brama.yaml's version: is.
func TestLoadRejectsANewerSchema(t *testing.T) {
	s := newServer()
	s.files[envPath+"/"+state.Dir+"/"+state.Filename] = `{"version": 99}`

	_, err := state.Load(context.Background(), s, envPath)
	if err == nil {
		t.Fatal("Load() = nil, want a newer schema to be rejected")
	}
	if !strings.Contains(err.Error(), "brama") {
		t.Errorf("error = %q, want it to say which side is out of date", err)
	}
}

func TestLoadPropagatesAConnectionFailure(t *testing.T) {
	s := newServer()
	s.runErr = errors.New("connection refused")

	if _, err := state.Load(context.Background(), s, envPath); err == nil {
		t.Fatal("Load() = nil, want the failure to propagate")
	}
}

// state.json is read by people and by other tooling on the Server, so its keys are a
// contract in the same way --json's are.
func TestStateJSONContract(t *testing.T) {
	s := newServer()
	st := &state.State{Version: state.SchemaVersion}
	st.RecordShim(upgrade("0.1.0", "0.2.0"), at)
	save(t, s, st)

	var got map[string]any
	body := s.files[envPath+"/"+state.Dir+"/"+state.Filename]
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("state.json is not JSON: %v\n%s", err, body)
	}

	recorded, ok := got["shim"].(map[string]any)
	if !ok {
		t.Fatalf("missing `shim` in %v", got)
	}
	if got["version"] != float64(state.SchemaVersion) {
		t.Errorf("version = %v, want %d", got["version"], state.SchemaVersion)
	}
	for key, want := range map[string]any{"version": "0.2.0", "platform": "linux/amd64"} {
		if recorded[key] != want {
			t.Errorf("shim.%s = %v, want %v", key, recorded[key], want)
		}
	}

	changes, ok := recorded["changes"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("shim.changes = %v, want one entry", recorded["changes"])
	}
	for key, want := range map[string]any{
		"at":     "2026-09-16T10:30:00Z",
		"from":   "0.1.0",
		"to":     "0.2.0",
		"change": "upgrade",
	} {
		if changes[0].(map[string]any)[key] != want {
			t.Errorf("shim.changes[0].%s = %v, want %v", key, changes[0].(map[string]any)[key], want)
		}
	}

	if !strings.HasSuffix(body, "\n") {
		t.Error("state.json does not end in a newline")
	}
}

// A first install has nothing to come from. The key is omitted rather than written
// empty, so "from": "" never reads as a version.
func TestFirstInstallHasNoFrom(t *testing.T) {
	s := newServer()
	st := &state.State{Version: state.SchemaVersion}
	st.RecordShim(shim.Step{Platform: linuxAMD64, To: "0.1.0", Change: shim.ChangeInstall}, at)
	save(t, s, st)

	if strings.Contains(s.files[envPath+"/"+state.Dir+"/"+state.Filename], `"from"`) {
		t.Error("state.json carries a `from` key for a first install")
	}
}
