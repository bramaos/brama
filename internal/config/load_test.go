package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/config"
)

// valid is the smallest config that passes validation, used as a base that tests
// mutate one field at a time.
const valid = `
version: 1

app:
  adapter: wordpress
  paths:
    config: web/app/.env
    uploads: web/app/uploads

environments:
  local:
    url: https://example.local.test
  production:
    server: prod
    path: /www/htdocs/w01/production
    url: https://example.com

servers:
  prod:
    host: example-prod
    user: deploy
`

func TestParseValid(t *testing.T) {
	cfg, err := config.Parse([]byte(valid))
	if err != nil {
		t.Fatalf("Parse() = %v, want no error", err)
	}

	if cfg.App.Adapter != "wordpress" {
		t.Errorf("App.Adapter = %q, want wordpress", cfg.App.Adapter)
	}
	if got := cfg.App.Paths.Uploads; got != "web/app/uploads" {
		t.Errorf("App.Paths.Uploads = %q, want web/app/uploads", got)
	}
	if got := cfg.Servers["prod"].Host; got != "example-prod" {
		t.Errorf("Servers[prod].Host = %q, want example-prod", got)
	}
}

func TestReachIsDerivedNotDeclared(t *testing.T) {
	cfg, err := config.Parse([]byte(valid))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	if got := cfg.Environments["local"].Reach(); got != config.ReachLocal {
		t.Errorf("local Reach = %v, want local", got)
	}
	if got := cfg.Environments["production"].Reach(); got != config.ReachRemote {
		t.Errorf("production Reach = %v, want remote", got)
	}
}

// Reach must not be settable from the file: anything that can edit brama.yaml must
// not be able to change what an Environment is.
func TestReachIsNotADeclarableKey(t *testing.T) {
	for _, key := range []string{"reach: local", "class: local", "locality: local"} {
		withKey := strings.Replace(valid,
			"  production:\n", "  production:\n    "+key+"\n", 1)

		if _, err := config.Parse([]byte(withKey)); err == nil {
			t.Errorf("Parse() accepted %q, want it rejected as an unknown key", key)
		}
	}
}

func TestLocalEnvironmentsExcludesRemote(t *testing.T) {
	cfg, err := config.Parse([]byte(valid))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	got := cfg.LocalEnvironments()
	if len(got) != 1 || got[0] != "local" {
		t.Errorf("LocalEnvironments() = %v, want [local]", got)
	}
}

func TestParseRejectsUnknownKeys(t *testing.T) {
	// The typo that must never silently mean "no environments".
	typo := strings.Replace(valid, "environments:", "enviroments:", 1)

	_, err := config.Parse([]byte(typo))
	if err == nil {
		t.Fatal("Parse() accepted a misspelled top-level key, want an error")
	}
	if !strings.Contains(err.Error(), "enviroments") {
		t.Errorf("error %q does not name the offending key", err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr string
	}{
		{"missing", "", "version is required"},
		// 0 is indistinguishable from absent, so it reports as missing.
		{"zero", "version: 0", "version is required"},
		{"too new", "version: 2", "upgrade brama"},
		{"nonsense", "version: -1", "no longer understands"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Replace(valid, "version: 1", tt.version, 1)

			_, err := config.Parse([]byte(body))
			if err == nil {
				t.Fatalf("Parse() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name:    "environment without a url",
			mutate:  func(s string) string { return strings.Replace(s, "    url: https://example.local.test\n", "", 1) },
			wantErr: "environments.local.url is required",
		},
		{
			name:    "remote environment without a path",
			mutate:  func(s string) string { return strings.Replace(s, "    path: /www/htdocs/w01/production\n", "", 1) },
			wantErr: "environments.production.path is required",
		},
		{
			name:    "environment naming an undeclared server",
			mutate:  func(s string) string { return strings.Replace(s, "server: prod", "server: staging", 1) },
			wantErr: `is "staging", which is not declared under servers`,
		},
		{
			name:    "server without a host",
			mutate:  func(s string) string { return strings.Replace(s, "    host: example-prod\n", "", 1) },
			wantErr: "servers.prod.host is required",
		},
		{
			name: "local naming a server",
			mutate: func(s string) string {
				return strings.Replace(s, "  local:\n", "  local:\n    server: prod\n    path: /srv/x\n", 1)
			},
			wantErr: "reserved for the machine brama runs on",
		},
		{
			name:    "local environment with a path",
			mutate:  func(s string) string { return strings.Replace(s, "  local:\n", "  local:\n    path: /srv/x\n", 1) },
			wantErr: "path is set but no server is named",
		},
		{
			name:    "no adapter",
			mutate:  func(s string) string { return strings.Replace(s, "  adapter: wordpress\n", "", 1) },
			wantErr: "app.adapter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Parse([]byte(tt.mutate(valid)))
			if err == nil {
				t.Fatalf("Parse() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// Every problem should be reported in one run, not one per attempt.
func TestValidateReportsEveryProblem(t *testing.T) {
	broken := `
version: 1
app:
  adapter: wordpress
environments:
  production:
    server: nope
  staging:
    server: alsonope
`
	_, err := config.Parse([]byte(broken))
	if err == nil {
		t.Fatal("Parse() = nil, want an error")
	}

	for _, want := range []string{
		"environments.production.url is required",
		"environments.production.path is required",
		"environments.staging.url is required",
		`is "nope", which is not declared`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%s", want, err)
		}
	}
}

func TestResolvedPathsMergesKeyByKey(t *testing.T) {
	body := strings.Replace(valid,
		"    url: https://example.local.test\n",
		"    url: https://example.local.test\n    paths:\n      uploads: storage/uploads\n", 1)

	cfg, err := config.Parse([]byte(body))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	got := cfg.Environments["local"].ResolvedPaths(cfg.App.Paths)
	if got.Uploads != "storage/uploads" {
		t.Errorf("Uploads = %q, want the override storage/uploads", got.Uploads)
	}
	if got.Config != "web/app/.env" {
		t.Errorf("Config = %q, want the app default web/app/.env", got.Config)
	}
}

func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "wp-content", "themes", "acme")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, config.Filename)
	if err := os.WriteFile(want, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := config.Find(deep)
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if got != want {
		t.Errorf("Find() = %q, want %q", got, want)
	}
}

func TestFindPrefersTheNearestFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "sites", "acme")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, config.Filename), []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(nested, config.Filename)
	if err := os.WriteFile(want, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := config.Find(nested)
	if err != nil {
		t.Fatalf("Find() = %v", err)
	}
	if got != want {
		t.Errorf("Find() = %q, want the nearer %q", got, want)
	}
}

func TestFindReportsNotFound(t *testing.T) {
	_, err := config.Find(t.TempDir())
	if !errors.Is(err, config.ErrNotFound) {
		t.Errorf("Find() = %v, want ErrNotFound", err)
	}
}

// Both v0.1 paths are required. Regression: a skeleton with only `uploads` unresolved
// used to parse as valid with Uploads: "", which would have let `files pull` rebuild
// the wrong tree without ever complaining.
func TestValidateRequiresBothPaths(t *testing.T) {
	tests := []struct {
		name    string
		remove  string
		wantErr string
	}{
		{"uploads alone", "    uploads: web/app/uploads\n", "app.paths.uploads is required"},
		{"config alone", "    config: web/app/.env\n", "app.paths.config is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Replace(valid, tt.remove, "", 1)

			_, err := config.Parse([]byte(body))
			if err == nil {
				t.Fatalf("Parse() = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
