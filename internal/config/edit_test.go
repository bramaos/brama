package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/bramaos/brama/internal/config"
)

// AddServer edits brama.yaml as a tree rather than re-marshalling it, because the
// file is a review surface a human also writes in. These tests are about what
// survives the edit as much as what the edit adds.
// See docs/adr/0005-one-committed-config-file.md.

func TestAddServerAppendsToAnExistingServersBlock(t *testing.T) {
	doc := `version: 1

servers:
  prod:
    host: prod.example.com
`

	out, err := config.AddServer([]byte(doc), "staging", config.Server{Host: "staging.example.com"})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	servers := parseServers(t, out)
	if got := servers["prod"].Host; got != "prod.example.com" {
		t.Errorf("prod.host = %q, want the original entry to survive", got)
	}
	if got := servers["staging"].Host; got != "staging.example.com" {
		t.Errorf("staging.host = %q, want staging.example.com", got)
	}
}

// The skeleton `brama init` writes carries a commented servers example. It is
// brama's own scaffolding, and its job ends the moment a real Server exists —
// leaving it would put a dead example above live config in every reviewed file.
func TestAddServerReplacesTheCommentedSkeletonBlock(t *testing.T) {
	doc := `version: 1

app:
  paths:
    config: wp-config.php            # holds the database credentials

# servers:
#   prod:
#     host: prod.example.com   # a hostname, an IP, or a ~/.ssh/config alias
#     user: deploy             # optional — without it, OpenSSH decides

# No anonymize block yet.
`

	out, err := config.AddServer([]byte(doc), "prod", config.Server{Host: "hetzner-prod"})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	if got := parseServers(t, out)["prod"].Host; got != "hetzner-prod" {
		t.Errorf("prod.host = %q, want hetzner-prod", got)
	}
	mustNotContain(t, out, "# servers:")
	mustNotContain(t, out, "#     host: prod.example.com")

	// Comments brama did not write are not brama's to reflow.
	mustContain(t, out, "    config: wp-config.php            # holds the database credentials")
	mustContain(t, out, "# No anonymize block yet.")
}

func TestAddServerRefusesANameAlreadyRegistered(t *testing.T) {
	doc := "version: 1\n\nservers:\n  prod:\n    host: hetzner-prod\n"

	_, err := config.AddServer([]byte(doc), "prod", config.Server{Host: "somewhere-else"})
	if !errors.Is(err, config.ErrServerExists) {
		t.Fatalf("err = %v, want ErrServerExists", err)
	}
}

// An absent user means OpenSSH decides, which is not the same as an empty one.
func TestAddServerWritesUserOnlyWhenSet(t *testing.T) {
	doc := "version: 1\n"

	withUser, err := config.AddServer([]byte(doc), "prod", config.Server{Host: "h", User: "deploy"})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	mustContain(t, withUser, "user: deploy")

	without, err := config.AddServer([]byte(doc), "prod", config.Server{Host: "h"})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	mustNotContain(t, without, "user:")
}

// The edited file is what every later command reads, so it has to survive a full
// parse — not just a probe for the block that changed.
func TestAddServerLeavesTheFileValid(t *testing.T) {
	doc := `version: 1

app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads

environments:
  local:
    url: https://acme.local.test
  production:
    server: prod
    path: /var/www/app
    url: https://acme.com

# servers:
#   prod:
#     host: prod.example.com
`

	out, err := config.AddServer([]byte(doc), "prod", config.Server{Host: "hetzner-prod"})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}

	cfg, err := config.Parse(out)
	if err != nil {
		t.Fatalf("edited file does not validate: %v\n\n%s", err, out)
	}
	if cfg.Environments["production"].Reach() != config.ReachRemote {
		t.Error("production should be remote once its server is declared")
	}
}

// parseServers reads back only the servers block, so a test asserts on the meaning
// of the edited file rather than on its exact bytes.
func parseServers(t *testing.T, doc []byte) map[string]config.Server {
	t.Helper()

	var probe struct {
		Servers map[string]config.Server `yaml:"servers"`
	}
	if err := yaml.Unmarshal(doc, &probe); err != nil {
		t.Fatalf("edited file does not parse: %v\n\n%s", err, doc)
	}
	return probe.Servers
}

func mustContain(t *testing.T, doc []byte, want string) {
	t.Helper()
	if !strings.Contains(string(doc), want) {
		t.Errorf("edited file is missing %q:\n\n%s", want, doc)
	}
}

func mustNotContain(t *testing.T, doc []byte, unwanted string) {
	t.Helper()
	if strings.Contains(string(doc), unwanted) {
		t.Errorf("edited file still contains %q:\n\n%s", unwanted, doc)
	}
}
