package shim

import (
	"context"
	"fmt"
	"strings"
)

// Home is where the Shim lives on a Server.
//
// It is server-global, not per-Environment: one Server may host several
// Environments, and the Shim is the same binary for all of them. Deriving its path
// from an Environment would mean `brama server add` could not install it at all —
// at registration time no Environment need name the Server yet.
// See docs/adr/0006-server-global-shim-home.md.
const Home = "~/.brama"

// Path is the symlink every operation executes. It points at the versioned file that
// is actually installed.
const Path = Home + "/shim"

// keptVersions is how many builds survive on the Server. Two: the one in use, and
// the one to fall back to.
const keptVersions = 2

// Runner is the part of a Server connection this package needs. It is declared here,
// where it is consumed, rather than by the transport — there is one real
// implementation, and this is the only place that benefits from substituting it.
type Runner interface {
	// Run executes a command and returns its standard output.
	Run(ctx context.Context, command string) (string, error)
	// Send streams data to a command's standard input.
	Send(ctx context.Context, command string, data []byte) error
}

// Source supplies the build for a Platform. Install takes one rather than reaching
// for the embedded binaries directly, so that what it does — the install protocol —
// stays separable from where the bytes came from. Production passes Binary.
type Source func(Platform) ([]byte, error)

// Report is what an install did.
type Report struct {
	// Platform is what the Server answered to `uname -sm`.
	Platform Platform
	// Version is the Shim that answered on the Server once the install finished.
	Version string
	// Uploaded distinguishes an install from a no-op. A caller registering a fleet
	// needs to tell "I put this here" from "it was already here" — two projects
	// sharing one Server is the ordinary case, not the exotic one.
	Uploaded bool
}

// Install puts the Shim on a Server and proves it runs there.
//
// A matching version already installed is left alone: the transfer is skipped, but
// the verification is not, because running it is what proves the install rather than
// what the filesystem claims about it.
func Install(ctx context.Context, r Runner, version string, src Source) (Report, error) {
	platform, err := detectPlatform(ctx, r)
	if err != nil {
		return Report{}, err
	}

	if installed, err := installedVersion(ctx, r); err == nil && installed == version {
		return Report{Platform: platform, Version: installed, Uploaded: false}, nil
	}

	data, err := src(platform)
	if err != nil {
		return Report{Platform: platform}, err
	}

	staged := stagedPath(version)
	if err := r.Send(ctx, "mkdir -p "+Home+" && cat > "+staged, data); err != nil {
		return Report{Platform: platform}, fmt.Errorf("uploading the shim: %w", err)
	}
	if _, err := r.Run(ctx, installScript(version)); err != nil {
		return Report{Platform: platform}, fmt.Errorf("installing the shim: %w", err)
	}

	installed, err := installedVersion(ctx, r)
	if err != nil {
		return Report{Platform: platform}, fmt.Errorf("the shim was installed but does not run: %w", err)
	}
	if installed != version {
		return Report{Platform: platform}, fmt.Errorf(
			"installed shim reports version %q, but %q was sent", installed, version)
	}
	return Report{Platform: platform, Version: installed, Uploaded: true}, nil
}

func detectPlatform(ctx context.Context, r Runner) (Platform, error) {
	out, err := r.Run(ctx, "uname -sm")
	if err != nil {
		return Platform{}, fmt.Errorf("asking the server what it is: %w", err)
	}
	return ParsePlatform(out)
}

// installedVersion asks the Shim on the Server what it is. An absent or unrunnable
// Shim is an error here, not an empty string: the caller's next move is to install
// one either way, and a blank version must never be mistaken for a match.
func installedVersion(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, Path+" --version")
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(out)
	if version == "" {
		return "", fmt.Errorf("%s printed no version", Path)
	}
	return version, nil
}

// installScript makes the staged upload live.
//
// The swap is atomic in both steps that matter: the binary is moved into place under
// its versioned name only after chmod, and the symlink is replaced by renaming over
// it rather than by unlinking first. An interrupted install therefore leaves the
// previous Shim intact instead of a half-written one at the path every operation
// executes.
func installScript(version string) string {
	var (
		staged   = stagedPath(version)
		final    = versionedPath(version)
		tmpLink  = Home + "/.shim.new"
		pruneOld = fmt.Sprintf(
			"ls -1t %s/shim-* 2>/dev/null | tail -n +%d | xargs -r rm -f",
			Home, keptVersions+1)
	)

	return strings.Join([]string{
		"set -e",
		"chmod 0755 " + staged,
		"mv -f " + staged + " " + final,
		"ln -sfn " + fileName(version) + " " + tmpLink,
		"mv -T " + tmpLink + " " + Path,
		pruneOld,
	}, " && ")
}

func stagedPath(version string) string    { return Home + "/." + fileName(version) + ".part" }
func versionedPath(version string) string { return Home + "/" + fileName(version) }
func fileName(version string) string      { return "shim-" + safeVersion(version) }

// safeVersion keeps a version string usable as a filename in an unquoted shell
// command. `git describe` can produce a slash or worse, and a path assembled from an
// unchecked string is how a version becomes a command.
func safeVersion(version string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-', r == '+':
			return r
		default:
			return '-'
		}
	}, version)
	if safe == "" {
		return "unknown"
	}
	return safe
}
