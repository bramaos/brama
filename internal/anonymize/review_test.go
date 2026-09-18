package anonymize_test

import (
	"testing"

	"github.com/bramaos/brama/internal/anonymize"
	"github.com/bramaos/brama/internal/config"
	"github.com/bramaos/brama/internal/preset"
)

// The two lanes are what the amendment carries and what it does not. A tightening and a
// column a migration added are written; a loosening and an unapproved keep are not.
func TestAmendmentsCarryTheAutomaticLaneOnly(t *testing.T) {
	review := anonymize.Review{
		Tightenings: preset.Drifts{{
			Name: "wp_users.user_email", Table: "wp_users", Column: "user_email",
			Recorded: config.Keep, Shipped: "fake.email", Correlate: "wp_email", Applied: true,
		}},
		New: []config.TableClassification{{Name: "orders", Columns: []config.ColumnClassification{
			{Name: "billing_email", Action: "fake.email", Declared: "varchar(200)"},
		}}},
		Held: preset.Drifts{{
			Name: "wp_posts.post_content", Table: "wp_posts", Column: "post_content",
			Recorded: config.Drop, Shipped: config.Keep,
		}},
		Unapproved: anonymize.Fallbacks{{Environment: "local", Column: "wp_users.user_status"}},
	}

	got := review.Amendments()

	want := []config.Amendment{
		{Table: "wp_users", Column: "user_email", Action: "fake.email", Correlate: "wp_email"},
		{Table: "orders", Column: "billing_email", Action: "fake.email", Declared: "varchar(200)"},
	}
	if len(got) != len(want) {
		t.Fatalf("Amendments() = %+v, want only the automatic lane: %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Amendments()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if review.Automatic() != 2 {
		t.Errorf("Automatic() = %d, want 2", review.Automatic())
	}
}

// Pending is the exit condition, and the only question a CI runner has to ask.
func TestPendingIsTheDecisionsNobodyButAHumanCanMake(t *testing.T) {
	held := preset.Drifts{{Name: "wp_posts.post_content"}}
	keeps := anonymize.Fallbacks{{Environment: "local", Column: "wp_users.user_status"}}
	claimed := []config.TableClassification{{Name: "orders", Columns: []config.ColumnClassification{
		{Name: "billing_email", Action: "fake.email"},
	}}}

	for name, tc := range map[string]struct {
		review anonymize.Review
		want   bool
	}{
		"a held loosening":   {anonymize.Review{Held: held}, true},
		"an unapproved keep": {anonymize.Review{Unapproved: keeps}, true},
		// Applied already, and written down. Nothing is waiting on anybody.
		"an applied tightening":      {anonymize.Review{Tightenings: preset.Drifts{{Applied: true}}}, false},
		"a column a migration added": {anonymize.Review{New: claimed}, false},
		// A pull refuses on one either way, and it is reported — but answering it is the
		// interactive review's job, not this one's.
		"a column nothing claims": {anonymize.Review{Unclassified: []anonymize.Uncovered{{Table: "users"}}}, false},
		"nothing at all":          {anonymize.Review{}, false},
	} {
		if got := tc.review.Pending(); got != tc.want {
			t.Errorf("%s: Pending() = %v, want %v", name, got, tc.want)
		}
	}
}
