package schema_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bramaos/brama/internal/schema"
)

// fakeIntrospector answers with a fixed Schema, and with fixed values for the
// Discriminators it knows, recording which it was asked about.
type fakeIntrospector struct {
	schema schema.Schema
	values map[string][]string
	err    error
	asked  []string
}

func (f *fakeIntrospector) Introspect(context.Context) (schema.Schema, error) {
	return f.schema, nil
}

func (f *fakeIntrospector) DiscriminatorValues(_ context.Context, table, column string) ([]string, error) {
	f.asked = append(f.asked, table+"."+column)
	if f.err != nil {
		return nil, f.err
	}
	return f.values[table+"."+column], nil
}

// wordpress is three tables of a WordPress schema: the two key/value tables and one
// that keys nothing.
func wordpress() schema.Schema {
	return schema.Schema{Database: "wordpress", Tables: []schema.Table{
		{Name: "wp_options", Columns: []schema.Column{{Name: "option_name"}, {Name: "option_value"}}},
		{Name: "wp_usermeta", Columns: []schema.Column{{Name: "meta_key"}, {Name: "meta_value"}}},
		{Name: "wp_users", Columns: []schema.Column{{Name: "user_email"}}},
	}}
}

func TestReadReadsTheValuesOfEachDiscriminatorIntoItsTable(t *testing.T) {
	f := &fakeIntrospector{schema: wordpress(), values: map[string][]string{
		"wp_usermeta.meta_key": {"", "billing_email", "nickname"},
	}}

	got, err := schema.Read(t.Context(), f, map[string]string{"wp_usermeta": "meta_key"})
	if err != nil {
		t.Fatalf("Read() = %v, want no error", err)
	}

	table, _ := got.Table("wp_usermeta")
	want := schema.Discriminator{Column: "meta_key", Values: []string{"", "billing_email", "nickname"}}
	if !reflect.DeepEqual(table.Discriminator, want) {
		t.Errorf("wp_usermeta Discriminator = %+v, want %+v", table.Discriminator, want)
	}
	// Nobody named wp_options, so its rows stay unread.
	if options, _ := got.Table("wp_options"); options.Discriminator.Column != "" {
		t.Errorf("wp_options Discriminator = %+v, want it left unread", options.Discriminator)
	}
	if !reflect.DeepEqual(f.asked, []string{"wp_usermeta.meta_key"}) {
		t.Errorf("asked = %v, want only the Discriminator named", f.asked)
	}
}

// A Discriminator the database does not have is nothing to read, and asking would be an
// error about a table this Environment simply lacks.
func TestReadSkipsADiscriminatorTheDatabaseDoesNotHave(t *testing.T) {
	f := &fakeIntrospector{schema: wordpress()}

	_, err := schema.Read(t.Context(), f, map[string]string{
		"wp_termmeta": "meta_key",
		"wp_usermeta": "umeta_key",
	})
	if err != nil {
		t.Fatalf("Read() = %v, want no error", err)
	}
	if len(f.asked) != 0 {
		t.Errorf("asked = %v, want nothing asked of a table or column that is absent", f.asked)
	}
}

func TestReadFailsWhenAValueCannotBeRead(t *testing.T) {
	broken := errors.New("connection reset")
	f := &fakeIntrospector{schema: wordpress(), err: broken}

	_, err := schema.Read(t.Context(), f, map[string]string{"wp_usermeta": "meta_key"})
	if !errors.Is(err, broken) {
		t.Errorf("Read() = %v, want %v", err, broken)
	}
}
