// Package config reads and validates brama.yaml, the project's Desired state.
//
// brama.yaml is committed to the repo and owned by developers through git. It never
// holds credentials: authentication is OpenSSH's, and per-Environment secrets belong
// to the Server. See docs/adr/0005-one-committed-config-file.md.
package config

// SchemaVersion is the only `version:` this build of brama understands. A file
// declaring a higher version is rejected rather than parsed optimistically — the
// schema moves before 1.0, and a clear "brama is too old" beats a parse error
// pointing at a key that changed meaning.
const SchemaVersion = 1

// LocalEnvironment is the conventional name for the Environment on the machine brama
// is running on. The name is reserved — an Environment called `local` may not name a
// Server — but it carries no behaviour of its own: Reach is derived instead, so that
// nothing which can edit brama.yaml can change what an Environment is.
const LocalEnvironment = "local"

// Config is a parsed brama.yaml.
type Config struct {
	Version      int                    `yaml:"version"`
	App          App                    `yaml:"app"`
	Environments map[string]Environment `yaml:"environments"`
	Servers      map[string]Server      `yaml:"servers,omitempty"`

	// Anonymize is written by `brama anonymize init`, never by `brama init`. Its
	// absence is meaningful: with no Classification, every column is Unclassified
	// and the first Pull refuses.
	Anonymize *Anonymize `yaml:"anonymize,omitempty"`
}

// App describes the project itself — the same in every Environment, because every
// Environment runs the same repo.
type App struct {
	Adapter string `yaml:"adapter"`
	Paths   Paths  `yaml:"paths"`
}

// Paths locate the parts of an installation brama needs to reach. They are relative
// to an Environment's root, which is what lets one layout describe a Server at
// /www/htdocs/w01/production and a laptop at ~/sites/acme.
//
// v0.1 needs exactly two. Deployment adds more in v0.2; adding an optional key then
// is not a breaking change.
type Paths struct {
	// Config is the file holding database credentials — wp-config.php, or .env
	// under Bedrock. brama reads it on the Server; it never copies it.
	Config string `yaml:"config,omitempty"`
	// Uploads is the tree `files pull` rebuilds out of Placeholders.
	Uploads string `yaml:"uploads,omitempty"`
}

// Environment is a named target brama can act on.
type Environment struct {
	// Server names the Server hosting this Environment. Its presence is what makes
	// the Environment remote; see Reach.
	Server string `yaml:"server,omitempty"`
	// Path is the Environment's root on its Server. Required when Server is set,
	// meaningless without it — a local Environment's root is the directory holding
	// brama.yaml.
	Path string `yaml:"path,omitempty"`
	URL  string `yaml:"url"`
	// Paths overrides App.Paths key by key. Usually absent.
	Paths *Paths `yaml:"paths,omitempty"`
}

// Server is an SSH host. Host is passed to OpenSSH untouched — it may be a hostname,
// an IP, or a ~/.ssh/config alias, and brama neither parses nor resolves it. User is
// optional: absent, OpenSSH decides, exactly as `ssh <host>` would.
//
// Nothing else belongs here. No password, no private_key, no port that ssh_config
// could carry instead.
type Server struct {
	Host string `yaml:"host"`
	User string `yaml:"user,omitempty"`
}

// Reach is how brama gets to an Environment. It is derived, never declared: anything
// that can edit brama.yaml must not be able to change what an Environment is.
type Reach int

const (
	// ReachLocal is an Environment on the machine brama is running on.
	ReachLocal Reach = iota
	// ReachRemote is an Environment reached across SSH to a Server.
	ReachRemote
)

func (r Reach) String() string {
	if r == ReachRemote {
		return "remote"
	}
	return "local"
}

// Reach reports how brama gets to this Environment. An Environment that names a
// Server is reached across SSH; one that names none is reached directly.
func (e Environment) Reach() Reach {
	if e.Server == "" {
		return ReachLocal
	}
	return ReachRemote
}

// ResolvedPaths merges the Environment's overrides over the App layout, key by key.
// An Environment that overrides only Uploads keeps the App's Config.
func (e Environment) ResolvedPaths(app Paths) Paths {
	out := app
	if e.Paths == nil {
		return out
	}
	if e.Paths.Config != "" {
		out.Config = e.Paths.Config
	}
	if e.Paths.Uploads != "" {
		out.Uploads = e.Paths.Uploads
	}
	return out
}

// LocalEnvironments returns the names of every Environment brama can reach directly.
// v0.1 permits a Pull to target one of these and nothing else, which is what makes
// "data flows downward only" true without the schema encoding any ordering.
func (c *Config) LocalEnvironments() []string {
	var names []string
	for name, env := range c.Environments {
		if env.Reach() == ReachLocal {
			names = append(names, name)
		}
	}
	return names
}
