package wordpress_test

import (
	"strings"
	"testing"

	"github.com/bramaos/brama/internal/adapter/wordpress"
)

// The ordinary case, and the one the whole feature is for: a project that hardened its
// prefix away from the default still gets an answer.
func TestTablePrefixReadsVanillaWpConfig(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php":       "<?php\n$table_prefix = 'acme_';\n",
		"wp-content/uploads/": "",
	})

	prefix, err := (wordpress.Detector{}).TablePrefix(root, "wp-config.php")
	if err != nil {
		t.Fatalf("TablePrefix() = %v, want the prefix wp-config.php declares", err)
	}
	if prefix != "acme_" {
		t.Errorf("TablePrefix() = %q, want acme_", prefix)
	}
}

// Spacing, quoting and a commented-out line above the real one are all wp-config.php as
// people write it, and none of them changes what the prefix is.
func TestTablePrefixReadsHowPeopleWriteIt(t *testing.T) {
	for _, body := range []string{
		"<?php\n$table_prefix='acme_';\n",
		"<?php\n$table_prefix   =   \"acme_\";\n",
		"<?php\n// $table_prefix = 'wp_';\n$table_prefix = 'acme_';\n",
	} {
		root := tree(t, map[string]string{"wp-config.php": body, "wp-content/uploads/": ""})
		prefix, err := (wordpress.Detector{}).TablePrefix(root, "wp-config.php")
		if err != nil {
			t.Fatalf("TablePrefix() = %v, want acme_ out of %q", err, body)
		}
		if prefix != "acme_" {
			t.Errorf("TablePrefix() = %q, want acme_ out of %q", prefix, body)
		}
	}
}

// Bedrock keeps the value in .env, where the rest of its credentials live.
func TestTablePrefixReadsBedrockEnv(t *testing.T) {
	root := tree(t, map[string]string{
		"web/app/uploads/":       "",
		"config/application.php": "<?php\n$table_prefix = env('DB_PREFIX') ?: 'wp_';\n",
		".env":                   "DB_NAME=acme\nDB_PREFIX=acme_\n",
	})

	prefix, err := (wordpress.Detector{}).TablePrefix(root, ".env")
	if err != nil {
		t.Fatalf("TablePrefix() = %v, want the prefix .env declares", err)
	}
	if prefix != "acme_" {
		t.Errorf("TablePrefix() = %q, want acme_", prefix)
	}
}

// A Bedrock project that never set DB_PREFIX is undetermined, and the `?: 'wp_'` in
// application.php does not settle it: `env()` reads the real environment as well as
// .env, so brama on a laptop cannot see what the container sets. Taking the literal
// would be a guess that looks like a reading.
func TestTablePrefixRefusesBedrocksUnsetDefault(t *testing.T) {
	root := tree(t, map[string]string{
		"web/app/uploads/":       "",
		"config/application.php": "<?php\n$table_prefix = env('DB_PREFIX') ?: 'wp_';\n",
		".env":                   "DB_NAME=acme\n",
	})

	if _, err := (wordpress.Detector{}).TablePrefix(root, ".env"); err == nil {
		t.Fatal("TablePrefix() = nil, want a refusal where only the environment knows the prefix")
	}
}

// A literal in application.php is PHP's last word on the value — `env()` never runs — so
// it wins over a DB_PREFIX in .env that nothing reads.
func TestTablePrefixPrefersBedrocksOwnLiteral(t *testing.T) {
	root := tree(t, map[string]string{
		"web/app/uploads/":       "",
		"config/application.php": "<?php\n$table_prefix = 'bd_';\n",
		".env":                   "DB_NAME=acme\nDB_PREFIX=stale_\n",
	})

	prefix, err := (wordpress.Detector{}).TablePrefix(root, ".env")
	if err != nil {
		t.Fatalf("TablePrefix() = %v, want the literal application.php assigns", err)
	}
	if prefix != "bd_" {
		t.Errorf("TablePrefix() = %q, want bd_ — the assignment that actually runs", prefix)
	}
}

// PHP takes the last assignment, so brama does. A file that sets the prefix twice means
// the bottom one, and reading the top one is a wrong answer rather than no answer.
func TestTablePrefixTakesTheLastAssignment(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php":       "<?php\n$table_prefix = 'wp_';\n$table_prefix = 'acme_';\n",
		"wp-content/uploads/": "",
	})

	prefix, err := (wordpress.Detector{}).TablePrefix(root, "wp-config.php")
	if err != nil {
		t.Fatalf("TablePrefix() = %v", err)
	}
	if prefix != "acme_" {
		t.Errorf("TablePrefix() = %q, want acme_ — the assignment that stands", prefix)
	}
}

// WordPress lets a project move wp-config.php, and `app.paths.config` is where init
// recorded that it did. A prefix brama could read is not a prefix it refuses.
func TestTablePrefixReadsTheConfigTheProjectRecorded(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php":        "<?php\nrequire __DIR__ . '/config/wp-config.php';\n",
		"config/wp-config.php": "<?php\n$table_prefix = 'acme_';\n",
		"wp-content/uploads/":  "",
	})

	prefix, err := (wordpress.Detector{}).TablePrefix(root, "config/wp-config.php")
	if err != nil {
		t.Fatalf("TablePrefix() = %v, want the config app.paths.config names", err)
	}
	if prefix != "acme_" {
		t.Errorf("TablePrefix() = %q, want acme_", prefix)
	}
}

// A line brama only half understood is a line it did not understand. An unbalanced quote
// is not a prefix.
func TestTablePrefixRefusesAValueItCannotReadWhole(t *testing.T) {
	root := tree(t, map[string]string{
		"web/app/uploads/":       "",
		"config/application.php": "<?php\n// nothing here\n",
		".env":                   "DB_PREFIX='acme_\n",
	})

	if _, err := (wordpress.Detector{}).TablePrefix(root, ".env"); err == nil {
		t.Fatal("TablePrefix() = nil, want a refusal on a value it could not read whole")
	}
}

// A value built from an expression brama cannot evaluate is absent, not half-read. The
// same rule parseURL follows: a prefix guessed wrong applies the account table's
// classification to whatever table sorted into its place.
func TestTablePrefixRefusesAnExpressionItCannotRead(t *testing.T) {
	root := tree(t, map[string]string{
		"wp-config.php":       "<?php\n$table_prefix = getenv('WP_PREFIX');\n",
		"wp-content/uploads/": "",
	})

	_, err := (wordpress.Detector{}).TablePrefix(root, "wp-config.php")
	if err == nil {
		t.Fatal("TablePrefix() = nil, want a refusal rather than a guess at wp_")
	}
	if !strings.Contains(err.Error(), "wp-config.php") {
		t.Errorf("TablePrefix() = %q, want it to name wp-config.php as what it looked in", err)
	}
}

// What it looked in, for a layout where that is two files.
func TestTablePrefixNamesBothBedrockFiles(t *testing.T) {
	root := tree(t, map[string]string{
		"web/app/uploads/":       "",
		"config/application.php": "<?php\n// nothing about the prefix at all\n",
		".env":                   "DB_NAME=acme\n",
	})

	_, err := (wordpress.Detector{}).TablePrefix(root, ".env")
	if err == nil {
		t.Fatal("TablePrefix() = nil, want a refusal rather than a guess at wp_")
	}
	for _, want := range []string{".env", "config/application.php"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("TablePrefix() = %q, want it to name %s as what it looked in", err, want)
		}
	}
}

// A project this adapter does not recognise has no prefix to read, and says so rather
// than reporting an empty one.
func TestTablePrefixRefusesAProjectThatIsNotWordPress(t *testing.T) {
	root := tree(t, map[string]string{"artisan": ""})

	if _, err := (wordpress.Detector{}).TablePrefix(root, ""); err == nil {
		t.Fatal("TablePrefix() = nil, want a refusal on a project that is not WordPress")
	}
}
