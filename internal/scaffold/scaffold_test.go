package scaffold_test

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/scaffold"
)

func complete() adapter.Detection {
	return adapter.Detection{
		Adapter:  "wordpress",
		Paths:    config.Paths{Config: "web/app/.env", Uploads: "web/app/uploads"},
		LocalURL: "https://acme.local.test",
	}
}

// The skeleton init writes must be a file brama can read back. This is the round trip
// that stops the generator and the parser drifting apart.
func TestSkeletonParsesBack(t *testing.T) {
	cfg, err := config.Parse(scaffold.Skeleton(complete()))
	if err != nil {
		t.Fatalf("Parse(Skeleton()) = %v, want the generated file to be valid", err)
	}

	if cfg.App.Adapter != "wordpress" {
		t.Errorf("App.Adapter = %q, want wordpress", cfg.App.Adapter)
	}
	if cfg.App.Paths.Uploads != "web/app/uploads" {
		t.Errorf("App.Paths.Uploads = %q, want web/app/uploads", cfg.App.Paths.Uploads)
	}
	if got := cfg.Environments[config.LocalEnvironment].URL; got != "https://acme.local.test" {
		t.Errorf("local url = %q, want https://acme.local.test", got)
	}
}

// local is reached directly. The skeleton must not give it a server.
func TestSkeletonLocalIsLocalReach(t *testing.T) {
	cfg, err := config.Parse(scaffold.Skeleton(complete()))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	if got := cfg.Environments[config.LocalEnvironment].Reach(); got != config.ReachLocal {
		t.Errorf("local Reach = %v, want local", got)
	}
}

// anonymize init owns that block. Its absence is what makes the first pull refuse.
func TestSkeletonWritesNoAnonymizeBlock(t *testing.T) {
	body := scaffold.Skeleton(complete())

	cfg, err := config.Parse(body)
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if cfg.Anonymize != nil {
		t.Errorf("Anonymize = %+v, want nil — brama init does not classify", cfg.Anonymize)
	}
	if !strings.Contains(string(body), "brama anonymize init") {
		t.Error("skeleton does not point at `brama anonymize init`, so the refusal has no stated fix")
	}
}

// brama holds no credentials. No credential-shaped key may appear, not even commented
// out as an example — a commented `# password:` is an invitation.
func TestSkeletonContainsNoCredentialKeys(t *testing.T) {
	body := strings.ToLower(string(scaffold.Skeleton(complete())))

	for _, forbidden := range []string{
		"password", "private_key", "passphrase", "secret", "token", "api_key", "identityfile",
	} {
		if strings.Contains(body, forbidden+":") {
			t.Errorf("skeleton declares a %q key, but brama stores no secrets", forbidden)
		}
	}
}

func TestSkeletonCommentsTheRemoteShape(t *testing.T) {
	body := string(scaffold.Skeleton(complete()))

	for _, want := range []string{"# production:", "#   server: prod", "# servers:", "#     host:"} {
		if !strings.Contains(body, want) {
			t.Errorf("skeleton is missing the commented line %q", want)
		}
	}
}

// An unresolved path is written as a comment naming the candidates. It is never
// written as a guess, and the resulting file must not validate — init exits non-zero,
// and nothing downstream should run against a value nobody chose.
func TestSkeletonCommentsUnresolvedPaths(t *testing.T) {
	result := complete()
	result.Paths.Uploads = ""
	result.Unresolved = []adapter.Unresolved{{
		Key:        "app.paths.uploads",
		Why:        "the uploads directory has been moved",
		Candidates: []string{"assets/media", "uploads"},
	}}

	body := string(scaffold.Skeleton(result))

	if !strings.Contains(body, "NOT DETERMINED") {
		t.Error("skeleton does not flag the unresolved path")
	}
	if !strings.Contains(body, "the uploads directory has been moved") {
		t.Error("skeleton does not carry the reason")
	}
	for _, candidate := range []string{"# uploads: assets/media", "# uploads: uploads"} {
		if !strings.Contains(body, candidate) {
			t.Errorf("skeleton is missing candidate line %q", candidate)
		}
	}
	if strings.Contains(body, "\n    uploads:") {
		t.Error("skeleton wrote an uncommented uploads value, want no guess at all")
	}
}

func TestSkeletonCommentsUnresolvedURL(t *testing.T) {
	result := complete()
	result.LocalURL = ""
	result.Unresolved = []adapter.Unresolved{{
		Key: "environments.local.url",
		Why: "no WP_HOME or WP_SITEURL found in wp-config.php",
	}}

	body := string(scaffold.Skeleton(result))

	if !strings.Contains(body, "# url: NOT DETERMINED") {
		t.Errorf("skeleton does not comment the undetermined url:\n%s", body)
	}
	if !strings.Contains(body, "no WP_HOME or WP_SITEURL") {
		t.Error("skeleton does not carry the reason the url is missing")
	}
}
