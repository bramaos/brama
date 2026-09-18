package wordpress

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// TablePrefix reports what this project's tables are named with.
//
// Every core table carries `$table_prefix` in front of its name, and the value is the
// project's own — `wp_` is what the installer writes, `acme_` is what a hardening guide
// tells people to write instead, and both are ordinary. The Preset is written against
// the prefix rather than against `wp_`, so this is what decides whether it matches
// anything at all.
//
// It is read out of the project's config and never declared in brama.yaml: a prefix
// written down twice is a prefix that can disagree with itself, and the copy in
// brama.yaml would be the one nobody updates when the install moves.
//
// declared is `app.paths.config` — the config file detection found, which WordPress lets
// a project move. It is read alongside the files this layout usually holds the prefix in,
// so a relocated wp-config.php is still the project's config rather than a file brama
// cannot find.
//
// A prefix this cannot determine is an error naming the files it read, and never a
// fallback to `wp_`. Classifying `acme_users` because it sorts where `wp_users` would
// have is the resemblance-matching ADR 0013 keeps out of the Generators, and it would
// apply the account table's Classification to whatever table happened to be there.
// See docs/adr/0014-a-preset-is-named-for-the-projects-table-prefix.md.
func (d Detector) TablePrefix(root, declared string) (string, error) {
	looked := prefixFiles(root, declared)
	if len(looked) == 0 {
		return "", fmt.Errorf("this does not look like a WordPress project, so there is no $table_prefix to read — "+
			"brama looked for wp-config.php and web/app under %s", root)
	}

	for _, from := range looked {
		if prefix := parsePrefix(read(root, from)); prefix != "" {
			return prefix, nil
		}
	}
	return "", fmt.Errorf("no literal $table_prefix found in %s — brama will not assume wp_, because a wrong "+
		"prefix classifies whatever table sorted into place as the accounts table",
		strings.Join(looked, " or "))
}

// prefixFiles is where this project writes the prefix down, in the order PHP would
// settle it.
//
// On Bedrock that is config/application.php before .env: the file assigns
// `$table_prefix` last and wins outright where it names a literal, and only where it
// leaves the value to `env()` does .env decide. Whatever `app.paths.config` names is read
// after both, because a layout brama recognises says more about where the prefix is than
// a path recorded once at init does.
func prefixFiles(root, declared string) []string {
	var looked []string
	switch {
	case isBedrock(root):
		looked = []string{"config/application.php", ".env"}
	case isVanilla(root):
		looked = []string{"wp-config.php"}
	}
	if declared != "" && !slices.Contains(looked, declared) {
		looked = append(looked, declared)
	}
	return looked
}

func read(root, from string) string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(from)))
	if err != nil {
		return ""
	}
	return string(data)
}

var (
	// $table_prefix = 'acme_'; in wp-config.php or in Bedrock's application.php. A value
	// built from an expression — env(), getenv(), a concatenation — does not match, and
	// reads as absent rather than as half-understood.
	phpPrefix = regexp.MustCompile(`(?m)^[^\S\n]*\$table_prefix[^\S\n]*=[^\S\n]*['"](\w+)['"]`)
	// DB_PREFIX=acme_ in a .env, quoted or bare.
	envPrefix = regexp.MustCompile(`(?m)^[^\S\n]*DB_PREFIX[^\S\n]*=[^\S\n]*(\S+)`)
)

// parsePrefix pulls a literal prefix out of one config file.
//
// The last assignment wins, because that is what PHP does with two of them: a file that
// sets the prefix under a condition and a file that carries a superseded line above the
// real one both mean the bottom one, and taking the top one would be a wrong answer
// rather than no answer.
func parsePrefix(body string) string {
	if body == "" {
		return ""
	}
	for _, re := range []*regexp.Regexp{phpPrefix, envPrefix} {
		if m := re.FindAllStringSubmatch(body, -1); m != nil {
			if prefix := literal(m[len(m)-1][1]); prefix != "" {
				return prefix
			}
		}
	}
	return ""
}

// literal is the value with its quotes taken off, and "" for anything that is not a
// prefix a table could be named with — an unbalanced quote, a trailing comment, an
// expression. Half a value read out of a line brama did not understand is the guess this
// whole file exists to refuse.
func literal(value string) string {
	if len(value) >= 2 {
		if quote := value[0]; (quote == '\'' || quote == '"') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
		}
	}
	if value == "" || !word.MatchString(value) {
		return ""
	}
	return value
}

var word = regexp.MustCompile(`^\w+$`)
