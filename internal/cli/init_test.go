package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/renderer"
)

// wordpressProject builds a vanilla WordPress tree that detection fully resolves.
func wordpressProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "wp-content", "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "<?php\ndefine('WP_HOME', 'https://acme.local.test');\n"
	if err := os.WriteFile(filepath.Join(root, "wp-config.php"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func testEnv() (*console, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return &console{
		Out:      &out,
		Err:      &errOut,
		Renderer: renderer.NewHuman(&out, &errOut),
	}, &out, &errOut
}

func TestInitWritesAConfigBramaCanReadBack(t *testing.T) {
	root := wordpressProject(t)
	env, _, _ := testEnv()

	if err := runInit(env, root, "", false); err != nil {
		t.Fatalf("runInit() = %v, want success", err)
	}

	cfg, path, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load() = %v, want the written file to be valid", err)
	}
	if path != filepath.Join(root, config.Filename) {
		t.Errorf("Load() read %q, want the file init wrote", path)
	}
	if cfg.App.Adapter != "wordpress" {
		t.Errorf("App.Adapter = %q, want wordpress", cfg.App.Adapter)
	}
	if got := cfg.Environments[config.LocalEnvironment].URL; got != "https://acme.local.test" {
		t.Errorf("local url = %q, want the detected one", got)
	}
}

// brama.yaml is desired state owned through git. There is no --force, because a
// clobbered file is reviewed decisions destroyed.
func TestInitWillNotOverwriteAnExistingFile(t *testing.T) {
	root := wordpressProject(t)
	env, _, _ := testEnv()

	if err := runInit(env, root, "", false); err != nil {
		t.Fatalf("first runInit() = %v", err)
	}
	before, err := os.ReadFile(filepath.Join(root, config.Filename))
	if err != nil {
		t.Fatal(err)
	}

	err = runInit(env, root, "", false)
	if err == nil {
		t.Fatal("second runInit() = nil, want it to refuse to overwrite")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to say the file already exists", err)
	}

	after, readErr := os.ReadFile(filepath.Join(root, config.Filename))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Error("the existing file was modified, want it untouched")
	}
}

// A nested project gets its own file. Discovery prefers the nearest brama.yaml, so
// the sub-project's own config wins when brama runs inside it and the parent's wins
// elsewhere — there is nothing ambiguous to protect against.
func TestInitWorksInsideAnotherProject(t *testing.T) {
	parent := wordpressProject(t)
	env, _, _ := testEnv()
	if err := runInit(env, parent, "", false); err != nil {
		t.Fatalf("runInit() on the parent = %v", err)
	}

	nested := filepath.Join(parent, "sites", "child")
	if err := os.MkdirAll(filepath.Join(nested, "wp-content", "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "<?php\ndefine('WP_HOME', 'https://child.local.test');\n"
	if err := os.WriteFile(filepath.Join(nested, "wp-config.php"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runInit(env, nested, "", false); err != nil {
		t.Fatalf("runInit() in a nested project = %v, want it to succeed", err)
	}

	cfg, path, err := config.Load(nested)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(nested, config.Filename) {
		t.Errorf("Load() from the nested project read %q, want its own file", path)
	}
	if got := cfg.Environments[config.LocalEnvironment].URL; got != "https://child.local.test" {
		t.Errorf("local url = %q, want the nested project's own", got)
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	root := wordpressProject(t)
	env, out, _ := testEnv()

	if err := runInit(env, root, "", true); err != nil {
		t.Fatalf("runInit() = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, config.Filename)); !os.IsNotExist(err) {
		t.Error("--dry-run wrote the file, want nothing written")
	}
	// It still shows what it would have written.
	if !strings.Contains(out.String(), "adapter: wordpress") {
		t.Errorf("--dry-run did not preview the file:\n%s", out.String())
	}
}

// An unreadable layout still produces a file to edit, but exits non-zero so nothing
// downstream runs against a value nobody chose.
func TestInitWritesSkeletonAndFailsWhenIncomplete(t *testing.T) {
	root := t.TempDir()
	// WordPress, but with uploads moved and the URL built from an expression.
	if err := os.MkdirAll(filepath.Join(root, "wp-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "<?php\ndefine('WP_HOME', getenv('SITE_URL'));\n"
	if err := os.WriteFile(filepath.Join(root, "wp-config.php"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _, _ := testEnv()
	err := runInit(env, root, "", false)

	if err == nil {
		t.Fatal("runInit() = nil, want a non-zero outcome for an incomplete file")
	}
	if !errors.Is(err, ErrAlreadyReported) {
		t.Errorf("error = %v, want it marked as already reported so it is not printed twice", err)
	}

	written, readErr := os.ReadFile(filepath.Join(root, config.Filename))
	if readErr != nil {
		t.Fatalf("no file written: %v — an incomplete result should still leave something to edit", readErr)
	}
	if !strings.Contains(string(written), "NOT DETERMINED") {
		t.Error("the written file does not flag what could not be determined")
	}
	// The file must not validate: it is deliberately incomplete.
	if _, parseErr := config.Parse(written); parseErr == nil {
		t.Error("the incomplete file parsed as valid, want it to fail until the keys are filled in")
	}
}

func TestInitReportsAnUnrecognisedProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _, _ := testEnv()
	err := runInit(env, root, "", false)

	if err == nil {
		t.Fatal("runInit() = nil, want an error for an unrecognised project")
	}
	if !strings.Contains(err.Error(), "--adapter") {
		t.Errorf("error = %q, want it to offer the --adapter escape hatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, config.Filename)); !os.IsNotExist(statErr) {
		t.Error("a file was written for an unrecognised project, want none")
	}
}

func TestInitHonoursForcedAdapter(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	env, _, _ := testEnv()
	err := runInit(env, root, "wordpress", false)

	// Forced, so it writes a skeleton — and reports that it could not read the layout.
	if !errors.Is(err, ErrAlreadyReported) {
		t.Errorf("error = %v, want an incomplete result", err)
	}
	written, readErr := os.ReadFile(filepath.Join(root, config.Filename))
	if readErr != nil {
		t.Fatalf("no file written: %v", readErr)
	}
	if !strings.Contains(string(written), "adapter: wordpress") {
		t.Error("the forced adapter was not recorded")
	}
}

// brama init never classifies. The absence of the block is what makes the first pull
// refuse, which is the guarantee ADR-0003 exists to keep.
func TestInitWritesNoClassification(t *testing.T) {
	root := wordpressProject(t)
	env, _, _ := testEnv()

	if err := runInit(env, root, "", false); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Anonymize != nil {
		t.Errorf("Anonymize = %+v, want nil — anonymize init owns that block", cfg.Anonymize)
	}
}

func TestInitResultStatus(t *testing.T) {
	complete := &InitResult{Path: "/x/brama.yaml"}
	if got := complete.Status(); got != renderer.StatusSuccess {
		t.Errorf("Status() = %q, want success", got)
	}

	partial := &InitResult{
		Path:       "/x/brama.yaml",
		unresolved: []adapter.Unresolved{{Key: "app.paths.uploads"}},
	}
	if got := partial.Status(); got != renderer.StatusPartial {
		t.Errorf("Status() = %q, want partial", got)
	}
	if !strings.Contains(partial.Headline(), "incomplete") {
		t.Errorf("Headline() = %q, want it to say the file is incomplete", partial.Headline())
	}
}

// The machine contract carries raw values; prose belongs to the human renderer.
func TestInitResultFieldsCarryRawValues(t *testing.T) {
	partial := &InitResult{
		Path:       "/x/brama.yaml",
		Adapter:    "wordpress",
		unresolved: []adapter.Unresolved{{Key: "environments.local.url"}},
	}

	for _, f := range partial.Fields() {
		if s, ok := f.Value.(string); ok && s == "not determined" {
			t.Errorf("field %q carries human prose, want the raw empty value", f.Key)
		}
	}
}
