package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// Filename is the name brama looks for, at or above the working directory.
const Filename = "brama.yaml"

// ErrNotFound reports that no brama.yaml exists at or above the starting directory.
var ErrNotFound = errors.New("no " + Filename + " found")

// Find walks up from startDir looking for brama.yaml, returning the path of the
// nearest one. It searches to the filesystem root: brama is often run from deep
// inside a project — wp-content/themes/acme — and requiring the project root on
// every invocation is a papercut on every command.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", startDir, err)
	}
	for {
		candidate := filepath.Join(dir, Filename)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load finds the nearest brama.yaml, parses it, and validates it. It returns the
// config and the path it was read from — the directory holding that file is the root
// a local Environment's Paths resolve against.
func Load(startDir string) (*Config, string, error) {
	path, err := Find(startDir)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("reading %s: %w", path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, path, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, path, nil
}

// versionProbe reads only the schema version, ignoring everything else. Parsing it
// separately is what lets a file from a future brama fail with "brama is too old"
// rather than a wall of unknown-key errors about keys that were renamed.
type versionProbe struct {
	Version int `yaml:"version"`
}

// Parse decodes and validates brama.yaml.
//
// Decoding is strict: an unknown key is an error, not a shrug. A typo'd
// `enviroments:` must never silently mean "no environments", and a misspelled table
// under `anonymize:` must never silently mean "unclassified, pass it through".
func Parse(data []byte) (*Config, error) {
	var probe versionProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", Filename, err)
	}
	switch {
	case probe.Version == 0:
		return nil, fmt.Errorf("version is required — add `version: %d` at the top of %s", SchemaVersion, Filename)
	case probe.Version > SchemaVersion:
		return nil, fmt.Errorf(
			"%s declares version %d, but this brama understands version %d — upgrade brama",
			Filename, probe.Version, SchemaVersion)
	case probe.Version < SchemaVersion:
		return nil, fmt.Errorf(
			"%s declares version %d, which this brama no longer understands (it reads version %d)",
			Filename, probe.Version, SchemaVersion)
	}

	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return nil, errors.New(yaml.FormatError(err, false, true))
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ValidationError collects every problem in a file, so one run reports all of them
// rather than making the user fix them one at a time.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 1 {
		return e.Problems[0]
	}
	return fmt.Sprintf("%d problems:\n  - %s", len(e.Problems), strings.Join(e.Problems, "\n  - "))
}

// Validate reports every way the config contradicts itself or the domain rules.
func (c *Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if c.App.Adapter == "" {
		add("app.adapter is required")
	}
	// Both v0.1 paths are required. An empty uploads path would let `files pull`
	// rebuild the wrong tree, and an empty config path would send the Shim at the
	// wrong file — so a skeleton with either left unresolved must not validate.
	if c.App.Paths.Config == "" {
		add("app.paths.config is required — the file holding database credentials")
	}
	if c.App.Paths.Uploads == "" {
		add("app.paths.uploads is required — the directory files pull rebuilds")
	}
	if len(c.Environments) == 0 {
		add("environments: at least one environment is required")
	}

	for _, name := range sortedKeys(c.Environments) {
		env := c.Environments[name]
		if env.URL == "" {
			add("environments.%s.url is required", name)
		}
		switch env.Reach() {
		case ReachRemote:
			if env.Path == "" {
				add("environments.%s.path is required, because it names a server", name)
			}
			if _, ok := c.Servers[env.Server]; !ok {
				add("environments.%s.server is %q, which is not declared under servers", name, env.Server)
			}
			if name == LocalEnvironment {
				add("environments.%s may not name a server: %q is reserved for the machine brama runs on",
					name, LocalEnvironment)
			}
		case ReachLocal:
			if env.Path != "" {
				add("environments.%s.path is set but no server is named — a local environment's root is the directory holding %s",
					name, Filename)
			}
		}
	}

	for _, name := range sortedKeys(c.Servers) {
		if c.Servers[name].Host == "" {
			add("servers.%s.host is required", name)
		}
	}

	if len(problems) > 0 {
		return &ValidationError{Problems: problems}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
