package config

import (
	"errors"
	"strings"
	"testing"
)

// head is everything above the environments block, which this file never touches and
// every fixture needs in order to parse.
const head = `version: 1
app:
  adapter: wordpress
  paths:
    config: wp-config.php
    uploads: wp-content/uploads
`

const project = head + `environments:
  local:
    url: https://acme.local.test
  staging:
    # Somebody's note about staging.
    url: https://staging.acme.test
    anonymize:
      approved:
        - wp_users.user_email
`

func approved(t *testing.T, doc []byte, environment string) []ColumnRef {
	t.Helper()
	cfg, err := Parse(doc)
	if err != nil {
		t.Fatalf("the written file does not parse: %v\n%s", err, doc)
	}
	env, ok := cfg.Environments[environment]
	if !ok {
		t.Fatalf("the written file declares no %s:\n%s", environment, doc)
	}
	if env.Anonymize == nil {
		return nil
	}
	return env.Anonymize.Approved
}

// The whole job: an approval lands under the destination it is for, and brama can read
// back the file it wrote.
func TestApproveRecordsTheReference(t *testing.T) {
	out, err := Approve([]byte(project), "local", []ColumnRef{{Table: "users", Column: "display_name"}})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	refs := approved(t, out, "local")
	if len(refs) != 1 || refs[0].String() != "users.display_name" {
		t.Errorf("local approves %v, want users.display_name", refs)
	}
}

// An approval counts for one destination and no other. That is the whole of what
// approval is, and a write that leaked into a second environment would grant an exposure
// nobody chose.
func TestApproveLeavesEveryOtherEnvironmentAlone(t *testing.T) {
	out, err := Approve([]byte(project), "local", []ColumnRef{{Table: "users", Column: "display_name"}})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	refs := approved(t, out, "staging")
	if len(refs) != 1 || refs[0].String() != "wp_users.user_email" {
		t.Errorf("staging approves %v, want only what it already approved", refs)
	}
}

// An environment that already has a list gets one more line in it, not a second list.
func TestApproveAppendsToAnExistingList(t *testing.T) {
	out, err := Approve([]byte(project), "staging", []ColumnRef{{Table: "wp_users", Column: "display_name"}})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	refs := approved(t, out, "staging")
	if len(refs) != 2 {
		t.Fatalf("staging approves %v, want both", refs)
	}
	if refs[0].String() != "wp_users.user_email" || refs[1].String() != "wp_users.display_name" {
		t.Errorf("staging approves %v, want the new reference appended to the old", refs)
	}
}

// The same decision written twice says nothing the first one did not, and puts it in the
// diff a second time.
func TestApproveRecordsAReferenceOnce(t *testing.T) {
	out, err := Approve([]byte(project), "staging", []ColumnRef{{Table: "wp_users", Column: "user_email"}})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	if refs := approved(t, out, "staging"); len(refs) != 1 {
		t.Errorf("staging approves %v, want the reference it already had, once", refs)
	}
	if string(out) != project {
		t.Errorf("a file with nothing to add changed:\n%s", out)
	}
}

// Several references in one call, which is what one answered question produces.
func TestApproveRecordsEveryReference(t *testing.T) {
	out, err := Approve([]byte(project), "local", []ColumnRef{
		{Table: "wp_users", Column: "user_login"},
		{Table: "wp_users", Column: "user_url"},
	})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	if refs := approved(t, out, "local"); len(refs) != 2 {
		t.Errorf("local approves %v, want both", refs)
	}
}

// brama.yaml is a review surface a human also writes in. Everything outside the inserted
// lines comes through byte for byte.
func TestApprovePreservesCommentsAndKeyOrder(t *testing.T) {
	out, err := Approve([]byte(project), "staging", []ColumnRef{{Table: "wp_users", Column: "display_name"}})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	written := string(out)
	if !strings.Contains(written, "# Somebody's note about staging.") {
		t.Errorf("a comment was lost:\n%s", written)
	}
	head, _, _ := strings.Cut(project, "environments:")
	if !strings.HasPrefix(written, head) {
		t.Errorf("the file above the block changed:\n%s", written)
	}
	if strings.Index(written, "local:") > strings.Index(written, "staging:") {
		t.Errorf("the environments were reordered:\n%s", written)
	}
}

// An approval names a destination, so a name the file does not declare is an error and
// never a block this creates: it would record an exposure against a name nothing reads.
func TestApproveRefusesAnEnvironmentTheFileDoesNotDeclare(t *testing.T) {
	_, err := Approve([]byte(project), "production", []ColumnRef{{Table: "users", Column: "email"}})
	if err == nil {
		t.Fatal("Approve() = nil, want it to refuse a destination that does not exist")
	}
	if !strings.Contains(err.Error(), "production") {
		t.Errorf("error = %v, want it to name the environment", err)
	}
}

func TestApproveRefusesAFileWithNoEnvironments(t *testing.T) {
	_, err := Approve([]byte("version: 1\n"), "local", []ColumnRef{{Table: "users", Column: "email"}})
	if !errors.Is(err, ErrNoEnvironmentsBlock) {
		t.Errorf("error = %v, want ErrNoEnvironmentsBlock", err)
	}
}

// brama edits lines, so a block it cannot find lines for is one it must not guess at.
func TestApproveRefusesAnInlineEnvironmentsBlock(t *testing.T) {
	_, err := Approve([]byte("environments: {local: {}}\n"), "local", []ColumnRef{{Table: "users", Column: "email"}})
	if err == nil {
		t.Fatal("Approve() = nil, want it to refuse a block written inline")
	}
}

// A list written with its dashes at the key's own indentation is the same list, and one
// more line goes into it rather than beside it.
func TestApproveReadsADedentedList(t *testing.T) {
	doc := head + `environments:
  local:
    url: https://acme.local.test
    anonymize:
      approved:
      - users.email
`
	out, err := Approve([]byte(doc), "local", []ColumnRef{{Table: "users", Column: "display_name"}})
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}

	refs := approved(t, out, "local")
	if len(refs) != 2 {
		t.Errorf("local approves %v, want both — the existing list was not found", refs)
	}
}

// Nothing to record writes nothing. A run where nobody approved anything must not dirty
// a checkout.
func TestApproveWithNothingToRecordChangesNothing(t *testing.T) {
	out, err := Approve([]byte(project), "local", nil)
	if err != nil {
		t.Fatalf("Approve() = %v", err)
	}
	if string(out) != project {
		t.Errorf("the file changed with nothing to record:\n%s", out)
	}
}
