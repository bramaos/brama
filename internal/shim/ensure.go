package shim

import (
	"context"
	"fmt"

	"github.com/bramaos/brama/internal/version"
)

// Change is what the version check decided about the Shim on a Server.
//
// All four are resolved the same way — the build this brama carries is the one that
// ends up installed — because the Shim and the brama driving it must come from one
// build (docs/adr/0001-anonymize-on-the-server.md). The distinction is what the user
// is told, and telling them is the point: the version check is a visible step of its
// own, taken before an operation rather than inside one.
type Change string

const (
	// ChangeNone is a Server already running this brama's build.
	ChangeNone Change = "none"
	// ChangeInstall is a Server where no Shim answered — none is installed, or the
	// one that is cannot run. Both are cured the same way, and brama cannot tell them
	// apart without trusting a filesystem it has already decided not to trust.
	ChangeInstall Change = "install"
	// ChangeUpgrade is a Shim this brama can say it is newer than.
	ChangeUpgrade Change = "upgrade"
	// ChangeReplace is a Shim that differs but is not older — a Server reached by a
	// newer brama first, or either side built outside a tagged tree. brama installs
	// its own build rather than stopping, because a Shim it cannot drive protects
	// nothing, and the swap is a rename that keeps the previous build.
	// See docs/adr/0007-replace-a-shim-brama-cannot-call-older.md.
	ChangeReplace Change = "replace"
)

// Step is the Shim's version check: what answered on the Server, what this brama
// carries, and what became of the difference.
type Step struct {
	// Platform is what the Server answered to `uname -sm`.
	Platform Platform
	// From is the version that answered on the Server. Empty when no Shim did.
	From string
	// To is the version this brama carries.
	To string
	// Change is what the difference amounts to.
	Change Change
	// Pending marks a Change that was reported rather than made — what `--dry-run`
	// produces. The Server is still running From.
	Pending bool
}

// Changed reports whether the Shim on the Server is not this brama's build.
func (s Step) Changed() bool { return s.Change != ChangeNone }

// Uploaded reports whether a binary crossed the connection. It is the difference
// between "brama put this here" and "it was already here", which is what a caller
// registering the same Server from a second project needs to see.
func (s Step) Uploaded() bool { return s.Changed() && !s.Pending }

// Running is the version the Shim on the Server reports now that the step is over.
// For a pending Change that is still the old one — which is the whole distinction
// --dry-run exists to make.
func (s Step) Running() string {
	if s.Pending {
		return s.From
	}
	return s.To
}

// Plan reports what Ensure would do, and does none of it.
//
// It is what `--dry-run` runs. The Server is read — its platform, and the version its
// Shim answers with — but nothing is written, and the build this brama would send is
// looked up before the change is promised: a plan that says "will upgrade" followed
// by a run that cannot is worse than no plan at all.
func Plan(ctx context.Context, r Remote, cliVersion string, src Source) (Step, error) {
	step, _, err := decide(ctx, r, cliVersion, src)
	step.Pending = step.Changed()
	return step, err
}

// Ensure makes the Shim on a Server the one this brama carries, and proves it runs.
//
// A matching version is left alone: the transfer is skipped, but the verification is
// not, because running it is what proves the install rather than what the filesystem
// claims about it.
func Ensure(ctx context.Context, r Remote, cliVersion string, src Source) (Step, error) {
	step, data, err := decide(ctx, r, cliVersion, src)
	if err != nil || !step.Changed() {
		return step, err
	}

	staged := stagedPath(cliVersion)
	if err := r.Send(ctx, "mkdir -p "+Home+" && cat > "+staged, data); err != nil {
		return step, fmt.Errorf("uploading the shim: %w", err)
	}
	if _, err := r.Run(ctx, installScript(cliVersion)); err != nil {
		return step, fmt.Errorf("installing the shim: %w", err)
	}

	installed, err := installedVersion(ctx, r)
	if err != nil {
		return step, fmt.Errorf("the shim was installed but does not run: %w", err)
	}
	if installed != cliVersion {
		return step, fmt.Errorf(
			"installed shim reports version %q, but %q was sent", installed, cliVersion)
	}
	return step, nil
}

// decide runs the version check and, when there is something to do, fetches the build
// that would do it. The bytes come back with the Step so Ensure does not read them a
// second time, and so Plan fails on a Server brama has no build for.
func decide(ctx context.Context, r Remote, cliVersion string, src Source) (Step, []byte, error) {
	platform, err := detectPlatform(ctx, r)
	if err != nil {
		return Step{}, nil, err
	}

	step := Step{Platform: platform, To: cliVersion}

	// An absent or unrunnable Shim is not an error here: installing one is the
	// answer either way, and the version it failed to print is not a version.
	if installed, err := installedVersion(ctx, r); err == nil {
		step.From = installed
	}
	step.Change = classify(step.From, cliVersion)

	if !step.Changed() {
		return step, nil, nil
	}

	data, err := src(platform)
	if err != nil {
		// Named here rather than by the caller: "unsupported platform" without the
		// platform leaves the user nothing to report.
		return step, nil, fmt.Errorf("%w (server is %s)", err, platform)
	}
	return step, data, nil
}

// classify names the difference between what is on the Server and what this brama
// carries.
func classify(installed, cliVersion string) Change {
	if installed == "" {
		return ChangeInstall
	}
	switch version.Compare(cliVersion, installed) {
	case version.Same:
		return ChangeNone
	case version.Newer:
		return ChangeUpgrade
	default:
		return ChangeReplace
	}
}
