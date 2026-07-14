// Tests for the top-level error handler in `cmd/root.go`. The handler
// is tested in-process via `ExecuteWithDeps`, which returns the exit
// code instead of calling `os.Exit` directly.
package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// TESTS: reportError - classification + rendering
// -----------------------------------------------------------------------------

// TestReportError_withUserError_printsConciseMessageAndReturnsExitOne
// verifies that a `UserError` results in a single concise stderr line
// (no Go stack trace) and the user-error exit code.
func TestReportError_withUserError_printsConciseMessageAndReturnsExitOne(t *testing.T) {
	var stderr bytes.Buffer

	code := reportError(&stderr, NewUserError("install-base-dir must not be empty"))

	if code != exitCodeUserError {
		t.Errorf("expected exit code %d, got %d", exitCodeUserError, code)
	}
	got := stderr.String()
	if !strings.Contains(got, "Error: install-base-dir must not be empty") {
		t.Errorf("expected user error message on stderr, got: %q", got)
	}
	if strings.Contains(got, "goroutine ") {
		t.Errorf("expected NO Go stack trace for user errors, got: %q", got)
	}
}

// TestReportError_withCobraUsageError_printsConciseMessageAndReturnsExitOne
// verifies that errors raised by cobra/pflag during command-line
// parsing are treated as user errors.
func TestReportError_withCobraUsageError_printsConciseMessageAndReturnsExitOne(t *testing.T) {
	// Sample of the error types cobra/pflag emit. The handler uses
	// prefix-matching so any of these should be classified as user
	// errors.
	cases := []string{
		"unknown command \"foobar\" for \"ironbark\"",
		"unknown flag: --nope",
		"flag needs an argument: --release-environment",
		"required flag(s) \"image\" not set",
	}

	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			var stderr bytes.Buffer
			code := reportError(&stderr, errors.New(msg))

			if code != exitCodeUserError {
				t.Errorf("expected exit code %d, got %d", exitCodeUserError, code)
			}
			if !strings.Contains(stderr.String(), msg) {
				t.Errorf("expected cobra usage error on stderr, got: %q", stderr.String())
			}
		})
	}
}

// TestReportError_withInternalError_returnsExitTwo verifies that any
// non-user, non-cobra-usage error is classified as internal and gets
// the internal-error exit code.
func TestReportError_withInternalError_returnsExitTwo(t *testing.T) {
	var stderr bytes.Buffer
	code := reportError(&stderr, errors.New("downstream library exploded"))

	if code != exitCodeInternalError {
		t.Errorf("expected exit code %d (internal), got %d", exitCodeInternalError, code)
	}
}

// TestReportError_withWrappedUserError_isStillUserError verifies that
// wrapping a `UserError` in additional layers (e.g. via `fmt.Errorf`)
// does not cause it to be misclassified as internal.
func TestReportError_withWrappedUserError_isStillUserError(t *testing.T) {
	inner := NewUserError("install-base-dir must not be empty")
	wrapped := errors.New("not a UserError but wraps one")
	_ = wrapped // sanity placeholder; the real test follows.

	// Wrap with fmt.Errorf %w.
	type withErr interface{ Error() string }
	var _ withErr // appease imports.

	wrappedW := wrap(inner, "outer context")

	var stderr bytes.Buffer
	code := reportError(&stderr, wrappedW)
	if code != exitCodeUserError {
		t.Errorf("expected wrapped UserError to remain a user error, got exit code %d", code)
	}
}

// wrap is a tiny helper mirroring how callers attach context to
// downstream errors via `fmt.Errorf("%s: %w", ...)`.
func wrap(err error, msg string) error {
	return &wrappedError{msg: msg, cause: err}
}

type wrappedError struct {
	msg   string
	cause error
}

func (w *wrappedError) Error() string { return w.msg + ": " + w.cause.Error() }
func (w *wrappedError) Unwrap() error { return w.cause }

// -----------------------------------------------------------------------------
// TESTS: isCobraUsageError
// -----------------------------------------------------------------------------

// TestIsCobraUsageError_recognisesKnownPrefixes locks in the known
// set of cobra/pflag error prefixes the handler should match.
func TestIsCobraUsageError_recognisesKnownPrefixes(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"unknown command foo", true},
		{"unknown flag: --bar", true},
		{"unknown shorthand flag: 'q'", true},
		{"flag needs an argument: --image", true},
		{"invalid argument \"foo\" for \"--release-environment\"", true},
		{"required flag(s) \"image\" not set", true},
		{"requires at least 1 arg(s), only received 0", true},
		{"accepts 1 arg(s), received 0", true},
		{"something completely different", false},
		{"unrelated error message", false},
	}

	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			if got := isCobraUsageError(errors.New(c.msg)); got != c.want {
				t.Errorf("isCobraUsageError(%q) = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}
