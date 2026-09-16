package config

import "fmt"

// Classification is the recorded decision of what happens to a column's values during
// Anonymization. There are exactly three, and there is no fourth meaning "not sure" —
// a column with no Classification is Unclassified, and Unclassified refuses the Pull.
type Classification string

const (
	// Fake replaces the value with a fabricated one of the same shape.
	Fake Classification = "fake"
	// Keep transfers the value as-is.
	Keep Classification = "keep"
	// Drop transfers the value as empty or null.
	Drop Classification = "drop"
)

// Valid reports whether c is one of the three classifications. Anything else is a
// typo in the config, and an unclassified column is what `anonymize check` refuses on.
func (c Classification) Valid() bool {
	return c == Fake || c == Keep || c == Drop
}

// Anonymize is the Classification for this project. It is written by
// `brama anonymize init` and reviewed in the diff like any other committed decision.
type Anonymize struct {
	// Preset names the Classification an Adapter ships for tables it already knows.
	Preset string `yaml:"preset,omitempty"`
	// Tables holds what the Preset does not cover — what is specific to this project.
	Tables map[string]Table `yaml:"tables,omitempty"`
}

// Table is the Classification for one table, in one of two shapes.
//
// Most tables classify per column:
//
//	wp_users:
//	  user_email: fake
//	  ID: keep
//
// Key/value tables instead name a Discriminator — the column whose value selects
// which Classification applies to the row — and classify per key:
//
//	wp_usermeta:
//	  discriminator: meta_key
//	  keys:
//	    billing_phone: fake
//	    _edit_lock: keep
type Table struct {
	Discriminator string                    `yaml:"discriminator,omitempty"`
	Keys          map[string]Classification `yaml:"keys,omitempty"`
	Columns       map[string]Classification `yaml:"-"`
}

// UnmarshalYAML accepts both Table shapes. Any key that is not `discriminator` or
// `keys` is read as a column name.
//
// Those two keys configure a Table rather than classify a column, so a column
// genuinely named "discriminator" or "keys" would collide. That is accepted, and
// `anonymize check` is where such a collision would surface.
func (t *Table) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}

	for key, value := range raw {
		switch key {
		case "discriminator":
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("discriminator must be a column name, got %T", value)
			}
			t.Discriminator = s
		case "keys":
			entries, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("keys must be a mapping of value to classification, got %T", value)
			}
			t.Keys = make(map[string]Classification, len(entries))
			for name, raw := range entries {
				c, err := classification(name, raw)
				if err != nil {
					return err
				}
				t.Keys[name] = c
			}
		default:
			c, err := classification(key, value)
			if err != nil {
				return err
			}
			if t.Columns == nil {
				t.Columns = map[string]Classification{}
			}
			t.Columns[key] = c
		}
	}
	return nil
}

// MarshalYAML writes the shape back out, flattening Columns to top-level keys.
func (t Table) MarshalYAML() (any, error) {
	out := map[string]any{}
	for column, c := range t.Columns {
		out[column] = string(c)
	}
	if t.Discriminator != "" {
		out["discriminator"] = t.Discriminator
	}
	if len(t.Keys) > 0 {
		keys := map[string]any{}
		for name, c := range t.Keys {
			keys[name] = string(c)
		}
		out["keys"] = keys
	}
	return out, nil
}

func classification(name string, value any) (Classification, error) {
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s: classification must be fake, keep, or drop, got %T", name, value)
	}
	c := Classification(s)
	if !c.Valid() {
		return "", fmt.Errorf("%s: %q is not a classification — must be fake, keep, or drop", name, s)
	}
	return c, nil
}
