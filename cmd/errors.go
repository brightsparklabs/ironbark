// Package cmd hosts the command definitions for the Ironbark CLI.
//
// This file defines the error-classification primitives used by the
// top-level error handler in `Execute`. The handler distinguishes
// between two error classes:
//
//  1. User errors (a bad flag value, a missing file at a user-supplied
//     path, an invalid argument): wrap in `UserError` via
//     `NewUserError`. The handler prints a single concise stderr line
//     and exits with status 1 (no stack trace).
//
//  2. Anything else (an unexpected internal failure, a downstream
//     library returning a programmer-visible error): the handler logs
//     via `slog` (which includes structured context useful for
//     triage) and exits with status 2.
//
// The split follows the POSIX convention where 1 signals a general
// error and 2 signals misuse / unexpected condition.
package cmd

import (
	"errors"
	"fmt"
)

// UserError represents an error caused by user input or operator
// action that the CLI can predict and handle gracefully (i.e. without
// surfacing a Go stack trace). Wrap any such error in `UserError` via
// `NewUserError` so the top-level handler in `Execute` formats it for
// human consumption.
type UserError struct {
	// msg is the human-readable error message presented to the user.
	msg string
	// cause is the wrapped underlying error, if any. Preserved so
	// callers can use `errors.Is` / `errors.As` to introspect the
	// original failure.
	cause error
}

// Error returns the user-facing error message.
func (e *UserError) Error() string {
	return e.msg
}

// Unwrap returns the wrapped cause, supporting `errors.Is` and
// `errors.As`.
func (e *UserError) Unwrap() error {
	return e.cause
}

// NewUserError constructs a `UserError` with a printf-style message.
// Use this when the caller has classified an error as
// user-actionable.
func NewUserError(format string, args ...any) error {
	return &UserError{msg: fmt.Sprintf(format, args...)}
}

// WrapUserError wraps an existing error as a `UserError` with an
// additional human-readable prefix. The wrapped cause remains
// accessible via `errors.Unwrap`/`errors.Is`/`errors.As`.
func WrapUserError(cause error, format string, args ...any) error {
	prefix := fmt.Sprintf(format, args...)
	return &UserError{
		msg:   fmt.Sprintf("%s: %s", prefix, cause.Error()),
		cause: cause,
	}
}

// IsUserError reports whether the supplied error (or any error in
// its `Unwrap` chain) is a `UserError`.
func IsUserError(err error) bool {
	var ue *UserError
	return errors.As(err, &ue)
}
