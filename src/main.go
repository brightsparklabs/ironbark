/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package main

import (
	"os"
	"strings"

	"brightsparklabs.com/ironbark/cmd"

	"log/slog"
)

func getLogLevelFromEnv() slog.Level {
	levelStr := os.Getenv("IRONBARK_LOG_LEVEL")
	switch strings.ToLower(levelStr) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	jsonHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: getLogLevelFromEnv(),
	})
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)

	cmd.Execute()
}
