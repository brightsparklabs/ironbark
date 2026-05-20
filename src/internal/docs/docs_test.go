// Tests for the embedded README accessor.
package docs

import (
	"strings"
	"testing"
)

// TestReadMe_returnsNonEmptyContent verifies the accessor always
// returns something usable - either the real embedded README (when
// the binary was built via `make build`) or the hard-coded fallback
// string (when `go build` was invoked standalone).
//
// This is the same union-coverage assertion the `cmd` package's
// `TestExecDocs_emitsNonEmptyOutput` makes; it is repeated here so
// the package can be tested in isolation.
func TestReadMe_returnsNonEmptyContent(t *testing.T) {
	got := ReadMe()
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected ReadMe() to return a non-empty string in all cases")
	}
	if !strings.HasPrefix(strings.TrimSpace(got), "=") {
		t.Errorf("expected output to start with an AsciiDoc heading marker, got: %.80q", got)
	}
}

// TestReadMeFallback_isNonEmpty verifies the hard-coded fallback
// string compiled into the binary is itself non-empty - i.e. there
// is always *something* to fall back to when the README is missing
// from the embedded resources.
func TestReadMeFallback_isNonEmpty(t *testing.T) {
	if strings.TrimSpace(readMeFallback) == "" {
		t.Fatal("readMeFallback must not be empty - it is the safety net when the embedded README is absent")
	}
}

// TestReadMeFallback_clearlyStatesNotEmbedded verifies the fallback
// string contains an unambiguous indicator that the running binary
// was built without the README embedded, so end-users immediately
// understand why they are seeing the fallback instead of the real
// README.
func TestReadMeFallback_clearlyStatesNotEmbedded(t *testing.T) {
	const want = "not embedded in this build"
	if !strings.Contains(readMeFallback, want) {
		t.Errorf("readMeFallback should clearly state %q for end-user clarity; got:\n%s", want, readMeFallback)
	}
}
