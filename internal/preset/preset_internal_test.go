package preset

import (
	"testing"

	"github.com/bramaos/brama/internal/config"
)

// keyed is a preset whose only placeholder is in a Discriminator key.
//
// No shipped preset is shaped like this — wordpress carries the mark in its table names
// too — so it is written here and named from inside the package, which is what the rest
// of these tests are in preset_test.go for.
var keyed = Preset{
	Name: "keyed",
	Tables: map[string]config.Table{
		"usermeta": {
			Discriminator: "meta_key",
			Value:         "meta_value",
			Keys: map[string]config.Column{
				"{prefix}capabilities": keep,
				"first_name":           drop,
			},
		},
	},
}

// A key that carries the prefix is as much a name the project decides as a table is, so
// a preset holding one is waiting on a prefix even where every table it names is
// spelled out.
func TestNeedsPrefixCountsAKeyThatCarriesIt(t *testing.T) {
	if !keyed.needsPrefix() {
		t.Error("needsPrefix() = false, want a preset whose key carries the mark to say it is waiting on a prefix")
	}
	if _, err := keyed.named(""); err == nil {
		t.Error("named(\"\") = nil, want a refusal rather than a key named after nothing")
	}
}

// The placeholder is never a key either. `{prefix}capabilities` matches no row, and a
// Discriminator value nothing matches is Unclassified, which refuses the Pull over a key
// the preset was carrying an answer for all along.
func TestNamedResolvesTheKeysThatCarryThePrefix(t *testing.T) {
	p, err := keyed.named("acme_")
	if err != nil {
		t.Fatalf("named(acme_): %v", err)
	}

	keys := p.Tables["usermeta"].Keys
	if got, known := keys["acme_capabilities"]; !known || got != keep {
		t.Errorf("keys[acme_capabilities] = %v, %v, want the shipped classification under the project's name", got, known)
	}
	if _, stale := keys["{prefix}capabilities"]; stale {
		t.Error("the placeholder survived as a key, and it matches no row")
	}
	if got := keys["first_name"]; got != drop {
		t.Errorf("keys[first_name] = %v, want a key carrying no prefix left as it is", got)
	}
	if _, aliased := keyed.Tables["usermeta"].Keys["acme_capabilities"]; aliased {
		t.Error("naming a preset wrote back into the set brama ships")
	}
}
