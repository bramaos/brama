package wordpress_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/adapter/wordpress"
)

// tree builds a directory layout. A path ending in / is a directory; anything else
// is a file with the given contents.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if strings.HasSuffix(path, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDetectVanilla(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php":        "<?php\ndefine('WP_HOME', 'https://acme.local.test');\n",
		"wp-content/uploads/":  "",
		"wp-content/themes/x/": "",
	})

	result, ok := wordpress.Detector{}.Detect(root)
	if !ok {
		t.Fatal("Detect() = false, want a vanilla WordPress install recognised")
	}
	if !result.Complete() {
		t.Errorf("Unresolved = %+v, want none", result.Unresolved)
	}
	if result.Paths.Config != "wp-config.php" {
		t.Errorf("Paths.Config = %q, want wp-config.php", result.Paths.Config)
	}
	if result.Paths.Uploads != "wp-content/uploads" {
		t.Errorf("Paths.Uploads = %q, want wp-content/uploads", result.Paths.Uploads)
	}
	if result.LocalURL != "https://acme.local.test" {
		t.Errorf("LocalURL = %q, want https://acme.local.test", result.LocalURL)
	}
}

func TestDetectBedrock(t *testing.T) {
	root := tree(t, map[string]string{
		"config/application.php": "<?php",
		"web/app/uploads/":       "",
		"web/wp/":                "",
		".env":                   "DB_NAME=acme\nWP_HOME='https://acme.local.test'\nWP_SITEURL=${WP_HOME}/wp\n",
	})

	result, ok := wordpress.Detector{}.Detect(root)
	if !ok {
		t.Fatal("Detect() = false, want a Bedrock install recognised")
	}
	if !result.Complete() {
		t.Errorf("Unresolved = %+v, want none", result.Unresolved)
	}
	if result.Paths.Config != ".env" {
		t.Errorf("Paths.Config = %q, want .env", result.Paths.Config)
	}
	if result.Paths.Uploads != "web/app/uploads" {
		t.Errorf("Paths.Uploads = %q, want web/app/uploads", result.Paths.Uploads)
	}
	if result.LocalURL != "https://acme.local.test" {
		t.Errorf("LocalURL = %q, want https://acme.local.test", result.LocalURL)
	}
	if len(result.Notes) == 0 {
		t.Error("Notes is empty, want the Bedrock observation recorded as a note")
	}
}

// The layout label is an observation, never configuration. Detection must not emit
// anything that would be written to app.adapter other than the framework name.
func TestBedrockIsANoteNotAnAdapterName(t *testing.T) {
	root := tree(t, map[string]string{
		"config/application.php": "<?php",
		"web/app/uploads/":       "",
		".env":                   "WP_HOME=https://acme.local.test\n",
	})

	result, _ := wordpress.Detector{}.Detect(root)
	if result.Adapter != "wordpress" {
		t.Errorf("Adapter = %q, want plain wordpress", result.Adapter)
	}
}

// A moved uploads directory is reported, not assumed. Guessing here means files pull
// rebuilds the wrong tree.
func TestMovedUploadsIsUnresolved(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php": "<?php\ndefine('WP_HOME', 'https://acme.local.test');\n",
		"wp-content/":   "",
		"assets/media/": "",
		"uploads/":      "",
	})

	result, ok := wordpress.Detector{}.Detect(root)
	if !ok {
		t.Fatal("Detect() = false, want WordPress still recognised")
	}
	if result.Complete() {
		t.Fatal("Complete() = true, want the moved uploads directory reported")
	}

	var found *adapter.Unresolved
	for i := range result.Unresolved {
		if result.Unresolved[i].Key == "app.paths.uploads" {
			found = &result.Unresolved[i]
		}
	}
	if found == nil {
		t.Fatalf("Unresolved = %+v, want an entry for app.paths.uploads", result.Unresolved)
	}
	if len(found.Candidates) == 0 {
		t.Error("Candidates is empty, want the directories that plausibly hold uploads")
	}
	if result.Paths.Uploads != "" {
		t.Errorf("Paths.Uploads = %q, want empty rather than a guess", result.Paths.Uploads)
	}
}

func TestUndetectableURLIsUnresolved(t *testing.T) {
	root := tree(t, map[string]string{
		// WP_HOME built from an expression, not a literal.
		"wp-config.php":       "<?php\ndefine('WP_HOME', getenv('SITE_URL'));\n",
		"wp-content/uploads/": "",
	})

	result, ok := wordpress.Detector{}.Detect(root)
	if !ok {
		t.Fatal("Detect() = false, want WordPress recognised")
	}
	if result.LocalURL != "" {
		t.Errorf("LocalURL = %q, want empty — the value is a PHP expression, not a literal", result.LocalURL)
	}
	if result.Complete() {
		t.Error("Complete() = true, want the undetermined URL reported")
	}
}

func TestDetectIgnoresUnrelatedProjects(t *testing.T) {
	root := tree(t, map[string]string{
		"package.json": "{}",
		"src/":         "",
	})

	if _, ok := (wordpress.Detector{}).Detect(root); ok {
		t.Error("Detect() = true for a non-WordPress project, want false")
	}
}

func TestDetectReportsUnrecognised(t *testing.T) {
	root := tree(t, map[string]string{"package.json": "{}"})

	_, err := adapter.Detect(root, []adapter.Detector{wordpress.Detector{}})
	if !errors.Is(err, adapter.ErrUnrecognised) {
		t.Errorf("Detect() = %v, want ErrUnrecognised", err)
	}
}

func TestDetectAsForcesTheAdapter(t *testing.T) {
	root := tree(t, map[string]string{"package.json": "{}"})

	result, err := adapter.DetectAs(root, "wordpress", []adapter.Detector{wordpress.Detector{}})
	if err != nil {
		t.Fatalf("DetectAs() = %v, want the forced adapter honoured", err)
	}
	if result.Adapter != "wordpress" {
		t.Errorf("Adapter = %q, want wordpress", result.Adapter)
	}
	if result.Complete() {
		t.Error("Complete() = true, want the unreadable layout reported as unresolved")
	}
}

func TestDetectAsReportsUnknownAdapter(t *testing.T) {
	_, err := adapter.DetectAs(t.TempDir(), "drupal", []adapter.Detector{wordpress.Detector{}})
	if err == nil {
		t.Fatal("DetectAs() = nil, want an error naming the known adapters")
	}
	if !strings.Contains(err.Error(), "wordpress") {
		t.Errorf("error = %q, want it to list wordpress as known", err)
	}
}

// wp-content can be relocated wholesale by WP_CONTENT_DIR. Such a site is still
// WordPress, and must be recognised — otherwise init refuses to write anything
// instead of writing a skeleton naming what it could not find.
func TestDetectVanillaWithContentRelocated(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php": "<?php\ndefine('WP_HOME', 'https://acme.local.test');\n" +
			"define('WP_CONTENT_DIR', dirname(__FILE__) . '/assets');\n",
		"assets/plugins/": "",
		"assets/uploads/": "",
		"wp-admin/":       "",
	})

	result, ok := wordpress.Detector{}.Detect(root)
	if !ok {
		t.Fatal("Detect() = false, want a relocated-content WordPress install still recognised")
	}
	if result.Adapter != "wordpress" {
		t.Errorf("Adapter = %q, want wordpress", result.Adapter)
	}
	if result.LocalURL != "https://acme.local.test" {
		t.Errorf("LocalURL = %q, want the url still read", result.LocalURL)
	}
	if result.Complete() {
		t.Error("Complete() = true, want the relocated uploads directory reported as unresolved")
	}
	if result.Paths.Uploads != "" {
		t.Errorf("Paths.Uploads = %q, want empty rather than a guess at the old location", result.Paths.Uploads)
	}
}
