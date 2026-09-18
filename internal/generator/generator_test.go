package generator_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/generator"
	"github.com/bramaos/brama/internal/schema"
)

// Brama owns the Generator vocabulary, so a name either resolves to something this
// build knows how to fabricate or it does not resolve at all. There is no third
// outcome, and in particular no "unrecognised, so produce a random string" — that is
// the silent degradation halfway through a production dump that ADR 0013 exists to
// prevent.
func TestLookupRefusesANameNoGeneratorHas(t *testing.T) {
	if _, err := generator.Lookup("email"); err != nil {
		t.Fatalf("Lookup(email) = %v, want no error", err)
	}

	_, err := generator.Lookup("e_mail")
	if err == nil {
		t.Fatal("Lookup(e_mail) = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "e_mail") {
		t.Errorf("error = %q, want it to quote the name that failed", err)
	}
	if !strings.Contains(err.Error(), "email") {
		t.Errorf("error = %q, want it to list the generators brama knows", err)
	}
}

// A claim is a declared pattern matching or not matching. There is no score, no
// threshold and no nearest match: "why did brama choose fake.email for this column?"
// has to be answerable with "because fake.email declares it matches `user_email`",
// in an incident review, months later.
func TestClaimNameIsAPatternMatchAndNothingSofter(t *testing.T) {
	claimed := []struct {
		column string
		want   string
	}{
		{"email", "email"},
		{"user_email", "email"},
		{"billing_email", "email"},
	}
	for _, tt := range claimed {
		t.Run(tt.column, func(t *testing.T) {
			g, ok := generator.ClaimName(tt.column)
			if !ok {
				t.Fatalf("ClaimName(%q) claimed nothing, want %s", tt.column, tt.want)
			}
			if g.Name != tt.want {
				t.Errorf("ClaimName(%q) = %s, want %s", tt.column, g.Name, tt.want)
			}
		})
	}

	// Near misses. Every one of these resembles a column fake.email fills, and
	// resemblance is exactly what a Generator is forbidden to fabricate on: an
	// unclaimed column is left Unclassified, which is a review item, not a guess.
	for _, column := range []string{"e_mail", "emails", "emailer", "email_verified_at", "Email"} {
		t.Run(column, func(t *testing.T) {
			if g, ok := generator.ClaimName(column); ok {
				t.Errorf("ClaimName(%q) = %s, want no claim", column, g.Name)
			}
		})
	}
}

// A Generator declares column types as well as names, and a claim needs both. A
// column called `email` holding a bigint is a foreign key to an addresses table under
// a misleading name, and fabricating an address into it breaks the join it was the
// whole point of.
func TestClaimNeedsTheTypeAsWellAsTheName(t *testing.T) {
	varchar := schema.Column{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100}
	if g, ok := generator.Claim(varchar); !ok || g.Name != "email" {
		t.Errorf("Claim(varchar user_email) = %s/%v, want email/true", g.Name, ok)
	}

	// The name matches. The type does not, and no amount of name matching makes an
	// email address fit an integer.
	bigint := schema.Column{Name: "user_email", Type: "bigint", Declared: "bigint(20)"}
	if g, ok := generator.Claim(bigint); ok {
		t.Errorf("Claim(bigint user_email) = %s, want no claim", g.Name)
	}

	// And a type it can fill under a name it does not claim is not a claim either.
	body := schema.Column{Name: "post_content", Type: "longtext"}
	if g, ok := generator.Claim(body); ok {
		t.Errorf("Claim(longtext post_content) = %s, want no claim", g.Name)
	}
}

// `check` refuses a Generator whose output cannot fit a column's declared type or
// length, in the editor rather than mid-dump on the Server. Fits is where that answer
// comes from, and it is asked of a Generator the file already named — so it explains
// itself rather than returning a bare false.
func TestFitsRefusesTheTypeAndTheLengthSeparately(t *testing.T) {
	email, err := generator.Lookup("email")
	if err != nil {
		t.Fatalf("Lookup(email) = %v, want no error", err)
	}

	if err := email.Fits(schema.Column{Name: "user_email", Type: "varchar", Declared: "varchar(100)", Length: 100}); err != nil {
		t.Errorf("Fits(varchar(100)) = %v, want no error", err)
	}

	err = email.Fits(schema.Column{Name: "user_email", Type: "bigint", Declared: "bigint(20)"})
	if err == nil {
		t.Fatal("Fits(bigint) = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "bigint(20)") {
		t.Errorf("error = %q, want it to quote the declared type", err)
	}

	err = email.Fits(schema.Column{Name: "user_email", Type: "varchar", Declared: "varchar(8)", Length: 8})
	if err == nil {
		t.Fatal("Fits(varchar(8)) = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "8") {
		t.Errorf("error = %q, want it to name the length that is too short", err)
	}
}

// corpus is the column names each Generator must claim, and — read the other way —
// the names every other Generator must leave alone. It lives in the test rather than
// on the Generator because it is the evidence for a rule, not part of the vocabulary
// brama ships.
//
// Disjointness over every possible column name is settled structurally, below. What
// this adds is the other half: that the patterns actually reach the columns real
// schemas have. The awkward neighbours are deliberate — `email_address`,
// `ip_address` and `street_address` are three different Generators' columns and one
// careless `.+_address$` away from being the same one.
var corpus = map[string][]string{
	"email":      {"email", "email_address", "user_email", "billing_email", "customer_email"},
	"full_name":  {"full_name", "display_name", "billing_full_name"},
	"first_name": {"first_name", "given_name", "billing_first_name"},
	"last_name":  {"last_name", "surname", "family_name", "billing_last_name"},
	"username":   {"username", "login", "user_login", "user_nicename", "admin_username"},
	"phone":      {"phone", "phone_number", "telephone", "billing_phone"},
	"url":        {"url", "website", "user_url", "website_url"},
	"ip":         {"ip", "ip_address", "user_ip", "last_login_ip"},
	"password":   {"password", "user_pass", "password_hash", "admin_password"},
	"street_address": {
		"address", "street_address",
		"address_1", "address_2", "address_line_1", "address_line_2",
		"billing_address_1", "shipping_address_2", "billing_address_line_2",
	},
	"city": {"city", "town", "billing_city", "shipping_city"},
	"postcode": {
		"postcode", "postal_code", "zip", "zip_code",
		"billing_postcode", "billing_postal_code", "shipping_zip", "billing_zip_code",
	},
	"company": {"company", "company_name", "billing_company"},
}

// Every Generator claims the columns it is for, and no other Generator claims them.
func TestEachGeneratorClaimsItsOwnColumnsAndNoOthers(t *testing.T) {
	for _, g := range generator.All() {
		columns, ok := corpus[g.Name]
		if !ok {
			t.Errorf("fake.%s has no columns in the corpus — a generator nothing is "+
				"asserted about is a generator whose claims nothing polices", g.Name)
			continue
		}

		for _, column := range columns {
			t.Run(g.Name+"/"+column, func(t *testing.T) {
				var claimants []string
				for _, other := range generator.All() {
					if other.ClaimsName(column) {
						claimants = append(claimants, "fake."+other.Name)
					}
				}
				switch {
				case len(claimants) == 0:
					t.Errorf("no generator claims %q, want fake.%s", column, g.Name)
				case len(claimants) > 1:
					t.Errorf("%q is claimed by %s — claims must be disjoint",
						column, strings.Join(claimants, " and "))
				case claimants[0] != "fake."+g.Name:
					t.Errorf("%q is claimed by %s, want fake.%s", column, claimants[0], g.Name)
				}
			})
		}
	}

	for name := range corpus {
		if _, err := generator.Lookup(name); err != nil {
			t.Errorf("the corpus names %q, which is not a generator: %v", name, err)
		}
	}
}

// simple is the only shape of pattern the vocabulary is allowed to use: an anchored
// literal, `^email$`, or an anchored suffix, `^.+_email$`.
//
// Confining the patterns to this family is what makes disjointness checkable rather
// than merely tested. Two patterns of this shape overlap only if one's witness — the
// shortest column name it claims — is claimed by the other, so witnessing every
// pattern against every Generator decides the question for all column names, not just
// the ones somebody thought to write down. An alternation or an unanchored pattern
// would put that argument out of reach, so it is refused here.
var simple = regexp.MustCompile(`^\^(\.\+)?[a-z0-9_]+\$$`)

// No two Generators may claim the same column. This is the test ADR 0013 promises,
// and it makes the promise the ADR makes: for every column name, not only the ones
// somebody put in the corpus. An overlap is caught here, before release, rather than
// becoming a column brama silently picks a Generator for on a production Server.
func TestNoTwoGeneratorsClaimTheSameColumn(t *testing.T) {
	for _, g := range generator.All() {
		for _, p := range g.Patterns {
			expr := p.String()
			if !simple.MatchString(expr) {
				t.Errorf("fake.%s declares %s — a pattern must be an anchored literal "+
					"or an anchored suffix, so that disjointness stays decidable", g.Name, expr)
				continue
			}

			// The witness: `^.+_email$` is witnessed by `x_email`, and `^email$` by
			// itself. Any column either pattern claims ends the same way this does.
			witness := strings.TrimSuffix(strings.TrimPrefix(expr, "^"), "$")
			witness = strings.Replace(witness, ".+", "x", 1)

			t.Run(g.Name+"/"+expr, func(t *testing.T) {
				var claimants []string
				for _, other := range generator.All() {
					if other.ClaimsName(witness) {
						claimants = append(claimants, "fake."+other.Name)
					}
				}
				if len(claimants) != 1 || claimants[0] != "fake."+g.Name {
					t.Errorf("%s is witnessed by %q, which is claimed by %s — want fake.%s alone",
						expr, witness, strings.Join(claimants, " and "), g.Name)
				}
			})
		}
	}
}

// hasher stands in for an Adapter that knows what a password hash looks like here.
type hasher struct{}

func (hasher) Hash() (string, error) {
	return "$2y$10$abcdefghijklmnopqrstuv0123456789012345678901234567890123", nil
}

// `fake.password` is ADR 0013's one exception: the name is brama's and validates like
// any other, but the bytes come from the Adapter. A project with no Adapter has no
// implementation of it at all, and that is a refusal — not a hash-shaped random
// string that leaves staging with no way to log in, and not a quiet fallback to
// `keep`, which leaks real hashes.
func TestResolvingPasswordWithoutAnAdapterFailsRatherThanFallingBack(t *testing.T) {
	// The name exists on its own. `anonymize check` validates a committed file on a
	// CI runner with no Adapter loaded, so `fake.password` has to be a name brama
	// recognises there.
	password, err := generator.Lookup("password")
	if err != nil {
		t.Fatalf("Lookup(password) = %v, want no error", err)
	}
	if !password.AdapterSupplied {
		t.Error("password.AdapterSupplied = false, want true")
	}

	_, err = generator.Resolve("password", nil)
	if err == nil {
		t.Fatal("Resolve(password, no adapter) = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "adapter") {
		t.Errorf("error = %q, want it to say the adapter is what is missing", err)
	}

	if _, err := generator.Resolve("password", hasher{}); err != nil {
		t.Errorf("Resolve(password, adapter) = %v, want no error", err)
	}

	// Every other Generator is brama's own all the way down, so a project with no
	// Adapter resolves it the same as one with.
	if _, err := generator.Resolve("email", nil); err != nil {
		t.Errorf("Resolve(email, no adapter) = %v, want no error", err)
	}

	// And an unknown name fails here too, for the same reason it fails in Lookup.
	if _, err := generator.Resolve("e_mail", hasher{}); err == nil {
		t.Error("Resolve(e_mail) = nil error, want a refusal")
	}
}
