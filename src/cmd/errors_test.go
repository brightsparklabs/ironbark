// Tests for the typed-error primitives used by the top-level error
// handler.
package cmd

import (
	"errors"
	"fmt"
	"testing"
)

// TestNewUserError_isClassifiedAsUserError verifies that
// `NewUserError` produces an error that `IsUserError` recognises.
func TestNewUserError_isClassifiedAsUserError(t *testing.T) {
	err := NewUserError("bad flag %q", "--foo")
	if !IsUserError(err) {
		t.Fatal("expected NewUserError to produce a UserError")
	}
	if err.Error() != `bad flag "--foo"` {
		t.Errorf("unexpected message: %q", err.Error())
	}
}

// TestWrapUserError_preservesCause verifies that `WrapUserError`
// keeps the wrapped error accessible via `errors.Is` and
// `errors.Unwrap`.
func TestWrapUserError_preservesCause(t *testing.T) {
	cause := errors.New("disk full")
	err := WrapUserError(cause, "could not write file %q", "config.yaml")

	if !IsUserError(err) {
		t.Fatal("expected WrapUserError to produce a UserError")
	}
	if !errors.Is(err, cause) {
		t.Errorf("expected wrapped cause to be reachable via errors.Is")
	}
	want := `could not write file "config.yaml": disk full`
	if err.Error() != want {
		t.Errorf("unexpected message: got=%q want=%q", err.Error(), want)
	}
}

// TestIsUserError_throughDeepUnwrap verifies that `IsUserError`
// returns true when a `UserError` is buried beneath other wrapping
// (e.g. `fmt.Errorf("...: %w", userErr)`).
func TestIsUserError_throughDeepUnwrap(t *testing.T) {
	ue := NewUserError("bad input")
	wrapped := fmt.Errorf("call site context: %w", ue)
	doubleWrapped := fmt.Errorf("outer: %w", wrapped)

	if !IsUserError(doubleWrapped) {
		t.Fatal("expected IsUserError to find the UserError through fmt.Errorf wrappings")
	}
}

// TestIsUserError_forPlainError_returnsFalse verifies that plain
// errors are NOT classified as user errors.
func TestIsUserError_forPlainError_returnsFalse(t *testing.T) {
	if IsUserError(errors.New("any other error")) {
		t.Fatal("expected plain errors.New to NOT be a UserError")
	}
	if IsUserError(nil) {
		t.Fatal("expected nil to NOT be a UserError")
	}
}
