/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package api

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"

	"brightsparklabs.com/ironbark/internal/settings"
	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

const (
	// uploadedKubeconfigPath is the location where uploaded kubeconfigs
	// are stored within the container.
	uploadedKubeconfigPath = "/tmp/ironbark/uploaded-kubeconfig"
)

// -----------------------------------------------------------------------------
// PACKAGE VARIABLES
// -----------------------------------------------------------------------------

// kubeconfigMutex protects concurrent access to the uploaded kubeconfig file.
var kubeconfigMutex sync.RWMutex

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// SaveUploadedKubeconfig saves the provided kubeconfig content to the
// uploaded kubeconfig location. This uploaded kubeconfig takes precedence
// over any mounted kubeconfig.
//
// The content is validated to ensure it is valid UTF-8 text and valid YAML
// before saving.
//
// Returns the path where the kubeconfig was saved.
func SaveUploadedKubeconfig(content []byte) (string, error) {
	kubeconfigMutex.Lock()
	defer kubeconfigMutex.Unlock()

	// Validate that the content is valid UTF-8 text.
	if err := validateTextContent(content); err != nil {
		return "", fmt.Errorf("invalid kubeconfig content: %w", err)
	}

	// Validate that the content is valid YAML.
	if err := validateYAML(content); err != nil {
		return "", fmt.Errorf("invalid kubeconfig YAML: %w", err)
	}

	// Ensure parent directory exists.
	dir := filepath.Dir(uploadedKubeconfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for uploaded kubeconfig: %w", err)
	}

	// Write the kubeconfig content.
	if err := os.WriteFile(uploadedKubeconfigPath, content, 0600); err != nil {
		return "", fmt.Errorf("failed to write uploaded kubeconfig: %w", err)
	}

	slog.Info("Uploaded kubeconfig saved", "path", uploadedKubeconfigPath)
	return uploadedKubeconfigPath, nil
}

// GetActiveKubeconfig returns the content and source of the currently active
// kubeconfig. The resolution order is:
//  1. Uploaded kubeconfig (if present)
//  2. Mounted kubeconfig (fallback)
//
// Returns the kubeconfig content, the source description, and any error.
func GetActiveKubeconfig() (content []byte, source string, err error) {
	kubeconfigMutex.RLock()
	defer kubeconfigMutex.RUnlock()

	// Try uploaded kubeconfig first.
	if content, err := tryReadKubeconfig(uploadedKubeconfigPath); err == nil {
		return content, "uploaded", nil
	}

	// Fall back to mounted kubeconfig.
	// Note: The actual kubeconfig path used by kubectl/client-go is resolved
	// via KUBECONFIG env var or default ~/.kube/config. For now, we just
	// check if a mounted kubeconfig exists at the expected location.
	mountedPath := getMountedKubeconfigPath()
	if content, err := tryReadKubeconfig(mountedPath); err == nil {
		return content, fmt.Sprintf("mounted (%s)", mountedPath), nil
	}

	return nil, "", fmt.Errorf("no kubeconfig available (neither uploaded nor mounted)")
}

// GetActiveKubeconfigPath returns the filesystem path of the currently active
// kubeconfig. The resolution order is:
//  1. Uploaded kubeconfig (if present)
//  2. Mounted kubeconfig (fallback)
//
// Returns the path and source description, or an error if no kubeconfig exists.
func GetActiveKubeconfigPath() (path string, source string, err error) {
	kubeconfigMutex.RLock()
	defer kubeconfigMutex.RUnlock()

	// Try uploaded kubeconfig first.
	if fileExists(uploadedKubeconfigPath) {
		return uploadedKubeconfigPath, "uploaded", nil
	}

	// Fall back to mounted kubeconfig.
	mountedPath := getMountedKubeconfigPath()
	if fileExists(mountedPath) {
		return mountedPath, fmt.Sprintf("mounted (%s)", mountedPath), nil
	}

	return "", "", fmt.Errorf("no kubeconfig available (neither uploaded nor mounted)")
}

// DeleteUploadedKubeconfig removes any uploaded kubeconfig, reverting to
// the mounted kubeconfig if present.
func DeleteUploadedKubeconfig() error {
	kubeconfigMutex.Lock()
	defer kubeconfigMutex.Unlock()

	if !fileExists(uploadedKubeconfigPath) {
		return fmt.Errorf("no uploaded kubeconfig to delete")
	}

	if err := os.Remove(uploadedKubeconfigPath); err != nil {
		return fmt.Errorf("failed to delete uploaded kubeconfig: %w", err)
	}

	slog.Info("Uploaded kubeconfig deleted", "path", uploadedKubeconfigPath)
	return nil
}

// HasUploadedKubeconfig returns true if an uploaded kubeconfig exists.
func HasUploadedKubeconfig() bool {
	kubeconfigMutex.RLock()
	defer kubeconfigMutex.RUnlock()
	return fileExists(uploadedKubeconfigPath)
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// tryReadKubeconfig attempts to read a kubeconfig file from the specified path.
// Returns the content or an error if the file doesn't exist or can't be read.
func tryReadKubeconfig(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	return content, nil
}

// fileExists returns true if a file exists at the specified path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// getMountedKubeconfigPath returns the expected path for a mounted kubeconfig.
// This follows the convention used in the Dockerfile and launcher scripts.
func getMountedKubeconfigPath() string {
	// Check if we're in a container and if a mounted kubeconfig path is
	// available via environment variable.
	if settings.InContainer() {
		// Convention: mounted configs go to /mnt/conf/kubeconfig.
		return "/mnt/conf/kubeconfig"
	}

	// When running outside a container (development), fall back to
	// the standard kubeconfig location.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".kube", "config")
}

// validateTextContent checks that the provided content is valid UTF-8 text
// and does not contain binary data.
func validateTextContent(content []byte) error {
	if len(content) == 0 {
		return fmt.Errorf("content is empty")
	}

	// Check if content is valid UTF-8.
	// This covers ASCII as ASCII is a subset of UTF-8.
	if !isValidUTF8(content) {
		return fmt.Errorf("content is not valid UTF-8 text")
	}

	// Check for null bytes which indicate binary data.
	for i, b := range content {
		if b == 0 {
			return fmt.Errorf("content contains null byte at position %d (binary data not allowed)", i)
		}
	}

	return nil
}

// isValidUTF8 checks if the content is valid UTF-8.
func isValidUTF8(content []byte) bool {
	// Check the entire content is valid UTF-8.
	i := 0
	for i < len(content) {
		r, size := decodeRune(content[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 sequence.
			return false
		}
		i += size
	}
	return true
}

// decodeRune decodes a single UTF-8 rune from the byte slice.
// Returns the rune and its size in bytes.
func decodeRune(b []byte) (rune, int) {
	if len(b) == 0 {
		return utf8.RuneError, 0
	}

	// Single-byte ASCII character.
	if b[0] < 0x80 {
		return rune(b[0]), 1
	}

	// Multi-byte UTF-8 sequence - use standard library.
	r, size := utf8.DecodeRune(b)
	return r, size
}

// validateYAML checks that the provided content is valid YAML.
func validateYAML(content []byte) error {
	// Attempt to unmarshal as YAML into a generic map.
	// Kubernetes kubeconfigs are always structured data.
	var data map[string]interface{}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("content is not valid YAML: %w", err)
	}

	// Basic sanity check - kubeconfig should have specific top-level keys.
	// At minimum, it should have 'apiVersion' or 'clusters' or 'contexts'.
	hasValidStructure := false
	for _, key := range []string{"apiVersion", "clusters", "contexts", "current-context", "kind", "users"} {
		if _, exists := data[key]; exists {
			hasValidStructure = true
			break
		}
	}

	if !hasValidStructure {
		return fmt.Errorf("content does not appear to be a valid kubeconfig (missing expected keys)")
	}

	return nil
}

// SetKubeconfigEnvFromPath sets the KUBECONFIG environment variable globally
// to the provided path.
func SetKubeconfigEnvFromPath(path string) error {
	slog.Info("Setting KUBECONFIG environment variable globally", "path", path)
	if err := os.Setenv("KUBECONFIG", path); err != nil {
		return fmt.Errorf("failed to set KUBECONFIG environment variable: %w", err)
	}
	return nil
}

// ClearKubeconfigEnv unsets the KUBECONFIG environment variable.
func ClearKubeconfigEnv() {
	slog.Info("Clearing KUBECONFIG environment variable")
	os.Unsetenv("KUBECONFIG")
}

// InitializeKubeconfigEnv attempts to set the KUBECONFIG environment variable
// at startup if a kubeconfig is available (uploaded or mounted).
// Returns an error if no kubeconfig is found, but this is not fatal - the
// kubeconfig can be uploaded later via the API.
func InitializeKubeconfigEnv() error {
	kubeconfigPath, source, err := GetActiveKubeconfigPath()
	if err != nil {
		return err
	}

	slog.Info("Found kubeconfig at startup", "source", source, "path", kubeconfigPath)
	return SetKubeconfigEnvFromPath(kubeconfigPath)
}
