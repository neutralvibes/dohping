// Package version holds build/version metadata for dohping.
package version

import "runtime/debug"

// Version is the semantic version of dohping. It may be overridden at
// build time:
//
//	go build -ldflags "-X dohping/internal/version.Version=v1.2.3"
//
// The release constant MUST match the next release tag — scripts/release.sh
// refuses to build a release when this and the tag disagree (a drift here
// silently produced a mislabeled v0.1.2 draft on 2026-08-26; the guard is
// the fix).
var Version = "0.1.3"

// Commit is the VCS revision at build time (short form). Overridable via
// ldflags; falls back to module build info when empty.
var Commit = ""

// String returns the full version string shown by --version.
func String() string {
	commit := Commit
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && s.Value != "" {
					commit = s.Value
					if len(commit) > 12 {
						commit = commit[:12]
					}
					break
				}
			}
		}
	}
	if commit == "" {
		return "dohping " + Version
	}
	return "dohping " + Version + " (commit " + commit + ")"
}
