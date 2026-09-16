package renderer_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/refusal"
	"github.com/bramaos/brama/internal/renderer"
)

type fakeResult struct {
	status renderer.Status
	notes  []string
	fields []renderer.Field
}

func (f *fakeResult) Action() string           { return "db_pull" }
func (f *fakeResult) Status() renderer.Status  { return f.status }
func (f *fakeResult) Headline() string         { return "pulled in 1m 12s" }
func (f *fakeResult) Fields() []renderer.Field { return f.fields }
func (f *fakeResult) Notes() []string          { return f.notes }

func result() *fakeResult {
	return &fakeResult{
		status: renderer.StatusSuccess,
		fields: renderer.Fields{}.
			Add("tables", "Tables", 63).
			Add("unclassified", "Unclassified", 0).
			Add("local_url", "Local URL", ""),
	}
}

func TestJSONCarriesActionAndStatus(t *testing.T) {
	var buf bytes.Buffer
	if err := renderer.NewJSON(&buf).Result(result()); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if got["action"] != "db_pull" {
		t.Errorf("action = %v, want db_pull", got["action"])
	}
	if got["status"] != "success" {
		t.Errorf("status = %v, want success", got["status"])
	}
	if got["tables"] != float64(63) {
		t.Errorf("tables = %v, want 63", got["tables"])
	}
}

// A partial result must never report success. This is the whole reason Status lives
// on the Result rather than being chosen by whichever renderer happens to run.
func TestJSONReportsPartial(t *testing.T) {
	r := result()
	r.status = renderer.StatusPartial

	var buf bytes.Buffer
	if err := renderer.NewJSON(&buf).Result(r); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "partial" {
		t.Errorf("status = %v, want partial", got["status"])
	}
}

// An undetermined value is null, not "". Empty string reads as "the answer is blank".
func TestJSONRendersUndeterminedAsNull(t *testing.T) {
	var buf bytes.Buffer
	if err := renderer.NewJSON(&buf).Result(result()); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	value, present := got["local_url"]
	if !present {
		t.Fatal("local_url is absent, want it present and null")
	}
	if value != nil {
		t.Errorf("local_url = %#v, want null", value)
	}
}

// Human prose must not appear in the machine contract.
func TestJSONCarriesNoHumanProse(t *testing.T) {
	r := result()
	r.notes = []string{"looks like Bedrock"}

	var buf bytes.Buffer
	if err := renderer.NewJSON(&buf).Result(r); err != nil {
		t.Fatal(err)
	}

	for _, prose := range []string{"pulled in", "not determined", "Bedrock"} {
		if strings.Contains(buf.String(), prose) {
			t.Errorf("JSON contains human prose %q:\n%s", prose, buf.String())
		}
	}
}

// A Refusal is structurally distinct from an error, so an agent can tell "I am not
// allowed" from "something broke" without reading the message.
func TestJSONRefusalIsNotAnError(t *testing.T) {
	var buf bytes.Buffer
	r := refusal.New(refusal.Unclassified,
		"wp_usermeta.meta_key='stripe_customer_id' has no classification",
		"brama anonymize init")

	if err := renderer.NewJSON(&buf).Refused("db_pull", r); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "refused" {
		t.Errorf("status = %v, want refused", got["status"])
	}
	if got["reason"] != "unclassified" {
		t.Errorf("reason = %v, want unclassified", got["reason"])
	}
	if got["fix"] != "brama anonymize init" {
		t.Errorf("fix = %v, want the command that clears it", got["fix"])
	}
	if _, isError := got["error"]; isError {
		t.Error("refusal carries an `error` key, want it distinct from a failure")
	}
}

func TestJSONError(t *testing.T) {
	var buf bytes.Buffer
	if err := renderer.NewJSON(&buf).Error("db_pull", errors.New("connection reset")); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "error" {
		t.Errorf("status = %v, want error", got["status"])
	}
}

// Piped output must be plain text. An agent reading stdout must never receive ANSI.
func TestHumanStripsANSIWhenNotATerminal(t *testing.T) {
	var out, errOut bytes.Buffer
	r := result()
	r.notes = []string{"looks like Bedrock"}

	if err := renderer.NewHuman(&out, &errOut).Result(r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Errorf("human output to a non-terminal contains ANSI escapes:\n%q", out.String())
	}
}

func TestHumanRendersFieldsAndNotes(t *testing.T) {
	var out, errOut bytes.Buffer
	r := result()
	r.notes = []string{"looks like Bedrock"}

	if err := renderer.NewHuman(&out, &errOut).Result(r); err != nil {
		t.Fatal(err)
	}

	body := out.String()
	for _, want := range []string{"Tables", "63", "looks like Bedrock", "pulled in 1m 12s", "✓"} {
		if !strings.Contains(body, want) {
			t.Errorf("human output is missing %q:\n%s", want, body)
		}
	}
	// The empty value reads as prose for a person, unlike the contract's null.
	if !strings.Contains(body, "not determined") {
		t.Errorf("human output does not explain the empty value:\n%s", body)
	}
}

// A partial result must not be marked with a success tick.
func TestHumanPartialIsNotTicked(t *testing.T) {
	var out, errOut bytes.Buffer
	r := result()
	r.status = renderer.StatusPartial

	if err := renderer.NewHuman(&out, &errOut).Result(r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "✓") {
		t.Errorf("partial result rendered with a success tick:\n%s", out.String())
	}
}

// A Refusal goes to stderr, says what happened, and says what clears it.
func TestHumanRefusalStatesTheFix(t *testing.T) {
	var out, errOut bytes.Buffer
	r := refusal.New(refusal.Unclassified, "wp_usermeta has no classification", "brama anonymize init")

	if err := renderer.NewHuman(&out, &errOut).Refused("db_pull", r); err != nil {
		t.Fatal(err)
	}

	body := errOut.String()
	if !strings.Contains(body, "refused") {
		t.Errorf("refusal output does not say it refused:\n%s", body)
	}
	if !strings.Contains(body, "brama anonymize init") {
		t.Errorf("refusal output does not state the fix:\n%s", body)
	}
	if out.Len() != 0 {
		t.Errorf("refusal wrote to stdout, want stderr only: %q", out.String())
	}
}

// An empty value does not always mean "brama could not work this out". A server's
// user is empty when nobody set one, which means OpenSSH decides — a deliberate
// state, not a gap. The Result knows the difference; the renderer cannot guess it.
func TestHumanRendersADeliberateAbsenceInItsOwnWords(t *testing.T) {
	var out, errOut bytes.Buffer
	r := &fakeResult{
		status: renderer.StatusSuccess,
		fields: renderer.Fields{}.
			AddOptional("user", "User", "", "from ~/.ssh/config").
			Add("local_url", "Local URL", ""),
	}

	if err := renderer.NewHuman(&out, &errOut).Result(r); err != nil {
		t.Fatal(err)
	}

	body := out.String()
	if !strings.Contains(body, "from ~/.ssh/config") {
		t.Errorf("output does not say what the absence means:\n%s", body)
	}
	// The field with no stated meaning keeps the default.
	if !strings.Contains(body, "not determined") {
		t.Errorf("an ordinary empty value lost its wording:\n%s", body)
	}
}

// The wording is for a person. The contract still says null, because "there is no
// answer" is the fact — not the sentence brama uses to explain it.
func TestJSONIgnoresTheAbsenceWording(t *testing.T) {
	var out bytes.Buffer
	r := &fakeResult{
		status: renderer.StatusSuccess,
		fields: renderer.Fields{}.AddOptional("user", "User", "", "from ~/.ssh/config"),
	}

	if err := renderer.NewJSON(&out).Result(r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "ssh/config") {
		t.Errorf("human prose leaked into the contract:\n%s", out.String())
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if value, ok := got["user"]; !ok || value != nil {
		t.Errorf("user = %v, want null", value)
	}
}
