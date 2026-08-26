package version

import (
	"strings"
	"testing"
)

// Version string is part of the -V contract (help.go, app_test.go asserts
// the full --version output). These pin the String() shape directly.

func TestStringNoCommit(t *testing.T) {
	oldV, oldC := Version, Commit
	defer func() { Version, Commit = oldV, oldC }()
	Version = "1.2.3"
	Commit = ""
	if got := String(); got != "dohping 1.2.3" {
		t.Errorf("String() = %q, want %q", got, "dohping 1.2.3")
	}
}

func TestStringWithCommit(t *testing.T) {
	oldV, oldC := Version, Commit
	defer func() { Version, Commit = oldV, oldC }()
	Version = "1.2.3"
	Commit = "0123456789abcdef"
	if got := String(); got != "dohping 1.2.3 (commit 0123456789abcdef)" {
		t.Errorf("String() = %q, want the full commit form", got)
	}
}

func TestStringTruncatesLongCommitFromBuildInfo(t *testing.T) {
	// Truncation to 12 chars happens on the debug.ReadBuildInfo() path
	// (a real binary built with VCS info). With an empty Commit, String()
	// falls through to ReadBuildInfo — which in a go test run carries the
	// module's own vcs.revision. Pin the shape: 12-char commit or a clean
	// no-commit form, never a >12 raw value straight from Commit.
	oldV, oldC := Version, Commit
	defer func() { Version, Commit = oldV, oldC }()
	Version = "0.1.1"
	Commit = "" // force the build-info path
	got := String()
	if len(got) > 12+len("dohping 0.1.1 (commit )") && strings.Contains(got, "0123456789abcdef0123456789abcdef") {
		t.Errorf("String() = %q, want the 12-char truncated commit", got)
	}
}
