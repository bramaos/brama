// Package adapter holds the framework-specific knowledge brama has about a project.
//
// An Adapter's job at init time is detection: work out which framework this is, and
// where its parts live. It never guesses. A layout it cannot read is reported as
// unresolved, because a wrong uploads path means `files pull` silently rebuilds the
// wrong tree, and a wrong config path sends the Shim at the wrong file.
package adapter

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bramaos/brama/internal/config"
)

// ErrUnrecognised reports that no Adapter recognised the project.
var ErrUnrecognised = errors.New("no adapter recognised this project")

// Detector recognises one framework.
type Detector interface {
	// Name is the value written to app.adapter.
	Name() string
	// Detect reports what it found under root. It returns false when the project is
	// not this framework at all, which is different from recognising the framework
	// but failing to locate its parts — that is a Detection with Unresolved entries.
	Detect(root string) (Detection, bool)
}

// Prefixer is an Adapter that can say what the project's database tables are named
// with, by reading the project's own config.
//
// It is separate from Detector because it answers a different question at a different
// time: detection runs once, at `brama init`, and the prefix is read every time a Preset
// is resolved, out of files that may have changed since. Nothing writes it into
// brama.yaml — a prefix recorded there is a second copy of the truth, and the copy that
// goes stale.
type Prefixer interface {
	Detector
	// TablePrefix reports the prefix, or an error naming the files it read. declared is
	// `app.paths.config`, the project's own config file as detection recorded it, and is
	// empty where nothing recorded one.
	//
	// It never falls back to the framework's default: a Preset applied against the wrong
	// prefix classifies whatever table sorted into place as the accounts table.
	TablePrefix(root, declared string) (string, error)
}

// TablePrefix asks the named Adapter what the project under root prefixes its tables
// with.
//
// The name is `app.adapter` rather than a fresh detection: the project already settled
// which framework it is, and asking again here could answer differently from the file
// brama is in the middle of reading.
func TablePrefix(root, name, declared string, detectors []Detector) (string, error) {
	for _, d := range detectors {
		if d.Name() != name {
			continue
		}
		prefixer, ok := d.(Prefixer)
		if !ok {
			return "", fmt.Errorf("the %s adapter cannot read a table prefix out of a project", name)
		}
		return prefixer.TablePrefix(root, declared)
	}

	return "", fmt.Errorf("unknown adapter %q — known adapters: %s", name, strings.Join(Names(detectors), ", "))
}

// Detection is what detection produced.
type Detection struct {
	Adapter string
	Paths   config.Paths
	// LocalURL is the URL the site answers on locally, read from the project's own
	// env config so brama works with Docker, Valet, Herd and Lando without knowing
	// about any of them.
	LocalURL string
	// Notes are observations worth printing but not worth recording — "looks like
	// Bedrock". The layout itself is written out as explicit paths, so the label
	// never becomes configuration.
	Notes []string
	// Unresolved lists what could not be determined. A non-empty Unresolved means
	// init writes the skeleton and stops rather than writing a guess.
	Unresolved []Unresolved
}

// Unresolved is one thing detection could not determine.
type Unresolved struct {
	// Key is the config key that has no value — "app.paths.uploads".
	Key string
	// Why explains what was looked for.
	Why string
	// Candidates are the plausible values found, if any. Written into the skeleton
	// as commented alternatives so the user picks rather than retypes.
	Candidates []string
}

// Complete reports whether everything init needs was determined.
func (r Detection) Complete() bool { return len(r.Unresolved) == 0 }

// AmbiguousError reports that more than one Adapter claimed the project. Guessing
// between them risks pointing the Shim at the wrong directory, so detection stops.
type AmbiguousError struct {
	Adapters []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("this project looks like more than one framework (%s) — rerun with --adapter to choose",
		strings.Join(e.Adapters, ", "))
}

// Detect runs every detector against root and returns the single Detection, if exactly
// one detector claims the project.
func Detect(root string, detectors []Detector) (Detection, error) {
	var (
		results []Detection
		names   []string
	)
	for _, d := range detectors {
		if result, ok := d.Detect(root); ok {
			results = append(results, result)
			names = append(names, d.Name())
		}
	}

	switch len(results) {
	case 0:
		return Detection{}, ErrUnrecognised
	case 1:
		return results[0], nil
	default:
		sort.Strings(names)
		return Detection{}, &AmbiguousError{Adapters: names}
	}
}

// DetectAs runs the single named detector, skipping recognition. This is the escape
// hatch behind `brama init --adapter`, for a layout detection cannot read.
func DetectAs(root, name string, detectors []Detector) (Detection, error) {
	for _, d := range detectors {
		if d.Name() != name {
			continue
		}
		result, ok := d.Detect(root)
		if !ok {
			// The user asserted the framework, so honour it and report what is
			// missing rather than refusing outright.
			result.Adapter = name
			result.Unresolved = append(result.Unresolved, Unresolved{
				Key: "app.paths",
				Why: fmt.Sprintf("forced --adapter %s, but this does not look like a %s project", name, name),
			})
		}
		return result, nil
	}

	return Detection{}, fmt.Errorf("unknown adapter %q — known adapters: %s",
		name, strings.Join(Names(detectors), ", "))
}

// Names lists the adapters brama knows, for help text and error messages.
func Names(detectors []Detector) []string {
	names := make([]string, 0, len(detectors))
	for _, d := range detectors {
		names = append(names, d.Name())
	}
	sort.Strings(names)
	return names
}
