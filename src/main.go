/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package main

import (
	"os"

	"brightsparklabs.com/ironbark/cmd"
	"brightsparklabs.com/ironbark/internal/settings"

	"log/slog"
)

// main is the Ironbark binary entry point. It freezes the runtime
// settings snapshot, configures structured logging, and dispatches to
// cobra. Settings resolution and validation deliberately happen
// BEFORE any subcommand can run.
func main() {
	// Resolve the settings snapshot once and validate the launcher
	// environment. Logging is not yet configured so we use a temporary
	// stderr handler to surface a validation failure.
	if err := settings.Init(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).
			Error("Launcher environment is invalid", "error", err.Error())
		os.Exit(1)
	}

	jsonHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: settings.LogLevel(),
	})
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)

	cmd.Execute()
}
