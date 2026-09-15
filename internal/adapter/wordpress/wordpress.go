// Package wordpress detects WordPress installations and where their parts live.
//
// WordPress is the harder case on purpose: core, wp-content and uploads can each be
// relocated independently, so there is no fixed pair of layouts to choose between.
// Detection seeds explicit paths; it never records a layout label, because a label
// stops meaning anything the moment someone moves half of one.
package wordpress

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bramaos/brama/internal/adapter"
	"github.com/bramaos/brama/internal/config"
)

// Detector recognises WordPress, in both the vanilla and Bedrock arrangements.
type Detector struct{}

func (Detector) Name() string { return "wordpress" }

// Detect reports a WordPress project under root.
func (d Detector) Detect(root string) (adapter.Detection, bool) {
	switch {
	case isBedrock(root):
		return d.bedrock(root), true
	case isVanilla(root):
		return d.vanilla(root), true
	default:
		return adapter.Detection{}, false
	}
}

// Bedrock keeps core under web/wp and content under web/app, and holds credentials
// in a .env at the project root rather than in wp-config.php.
func isBedrock(root string) bool {
	return dirExists(filepath.Join(root, "web", "app")) &&
		(fileExists(filepath.Join(root, "config", "application.php")) ||
			fileExists(filepath.Join(root, "web", "wp-config.php")))
}

// A vanilla install has wp-config.php at its root. That file alone is the marker,
// deliberately: wp-content can be relocated by WP_CONTENT_DIR and uploads by UPLOADS,
// so requiring them here would make a legitimately-rearranged site unrecognisable —
// and then init would refuse to write anything rather than writing a skeleton naming
// what it could not find.
func isVanilla(root string) bool {
	return fileExists(filepath.Join(root, "wp-config.php"))
}

func (d Detector) bedrock(root string) adapter.Detection {
	result := adapter.Detection{
		Adapter: d.Name(),
		Notes:   []string{"looks like Bedrock — core under web/wp, content under web/app"},
		Paths:   config.Paths{Config: ".env"},
	}

	result.Paths.Uploads = d.findUploads(root, &result, "web/app/uploads")
	result.LocalURL = d.findURL(root, &result, ".env")
	return result
}

func (d Detector) vanilla(root string) adapter.Detection {
	result := adapter.Detection{
		Adapter: d.Name(),
		Paths:   config.Paths{Config: "wp-config.php"},
	}

	result.Paths.Uploads = d.findUploads(root, &result, "wp-content/uploads")
	result.LocalURL = d.findURL(root, &result, "wp-config.php")
	return result
}

// findUploads confirms the conventional uploads directory exists. It can be moved by
// WP_CONTENT_DIR or the UPLOADS constant, so its absence is reported rather than
// assumed away — a wrong uploads path means files pull rebuilds the wrong tree.
func (d Detector) findUploads(root string, result *adapter.Detection, conventional string) string {
	if dirExists(filepath.Join(root, filepath.FromSlash(conventional))) {
		return conventional
	}

	result.Unresolved = append(result.Unresolved, adapter.Unresolved{
		Key:        "app.paths.uploads",
		Why:        "the uploads directory is not where this layout usually puts it — it may have been moved by WP_CONTENT_DIR or UPLOADS",
		Candidates: uploadCandidates(root),
	})
	return ""
}

// findURL reads the site's own configuration for the URL it answers on locally.
func (d Detector) findURL(root string, result *adapter.Detection, from string) string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(from)))
	if err == nil {
		if url := parseURL(string(data)); url != "" {
			return url
		}
	}

	result.Unresolved = append(result.Unresolved, adapter.Unresolved{
		Key: "environments.local.url",
		Why: "no WP_HOME or WP_SITEURL found in " + from,
	})
	return ""
}

var (
	// WP_HOME='https://example.test' in a .env, with or without quotes.
	envAssignment = regexp.MustCompile(`(?m)^\s*(?:WP_HOME|WP_SITEURL)\s*=\s*['"]?([^'"\s#]+)`)
	// define('WP_HOME', 'https://example.test'); in wp-config.php.
	phpDefine = regexp.MustCompile(`(?i)define\s*\(\s*['"](?:WP_HOME|WP_SITEURL)['"]\s*,\s*['"]([^'"]+)['"]`)
)

// parseURL pulls a literal WP_HOME or WP_SITEURL out of a config file. A value built
// from a PHP expression — getenv, string concatenation — reads as absent rather than
// being half-understood.
func parseURL(body string) string {
	for _, re := range []*regexp.Regexp{envAssignment, phpDefine} {
		if m := re.FindStringSubmatch(body); m != nil {
			if url := strings.TrimSpace(m[1]); strings.HasPrefix(url, "http") {
				return url
			}
		}
	}
	return ""
}

// uploadCandidates looks for directories that plausibly hold uploads, so an
// unresolved path comes with something to choose from rather than a blank line.
func uploadCandidates(root string) []string {
	var found []string
	for _, dir := range []string{
		"wp-content/uploads",
		"web/app/uploads",
		"content/uploads",
		"app/uploads",
		"uploads",
		"web/wp-content/uploads",
	} {
		if dirExists(filepath.Join(root, filepath.FromSlash(dir))) {
			found = append(found, dir)
		}
	}
	return found
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
