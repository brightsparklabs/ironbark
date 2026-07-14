// Tests for the `ironbark docs` command.
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// TestExecDocs_emitsNonEmptyOutput verifies the command produces
// output regardless of whether the binary was built with the real
// README embedded or `go build` was invoked standalone (in which case
// the fallback message is returned by `docs.ReadMe`).
func TestExecDocs_emitsNonEmptyOutput(t *testing.T) {
	var buf bytes.Buffer
	if err := execDocs(&buf); err != nil {
		t.Fatalf("execDocs returned unexpected error: %v", err)
	}

	got := buf.String()
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected docs output to be non-empty (real README or fallback)")
	}
	// Every README/fallback variant starts with the AsciiDoc title marker.
	if !strings.HasPrefix(strings.TrimSpace(got), "=") {
		t.Errorf("expected output to start with an AsciiDoc heading, got: %.80q", got)
	}
}
