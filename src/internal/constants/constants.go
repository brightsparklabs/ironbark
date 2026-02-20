/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package constants provides constant values used throughout the applicaton.
package constants

import (
	"os"
	"path/filepath"
)

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// ArgoCDRepoName is the name of the git repository for the ArgoCD App of Apps.
const ArgoCDRepoName string = "ironbark-argocd-app-of-apps"

// -----------------------------------------------------------------------------
// PRIVATE CONSTANTS
// -----------------------------------------------------------------------------

// envDataDir is the name of the environment variable which defines the directory to store application data in.
const envDataDir = "IRONBARK_DATA_DIR"

// defaultDataDir is the default directory to store application data in.
const defaultDataDir = "/tmp/ironbark/data"

// envDataDir is the name of the environment variable which defines the directory to store application data in.
const envInternalDataDir = "IRONBARK_INTERNAL_DATA_DIR"

// defaultDataDir is the default directory to store application data in.
const defaultInternalDataDir = "/tmp/ironbark/internal/data"

// -----------------------------------------------------------------------------
// PUBLIC FUNCTIONS
// -----------------------------------------------------------------------------

// GetDataDir returns the directory to store application data in.
func GetDataDir() string {
	return getEnvVar(envDataDir, defaultDataDir)
}

// GetInternalDataDir returns the directory to store application data in.
func GetInternalDataDir() string {
	return getEnvVar(envInternalDataDir, defaultInternalDataDir)
}

// GetArgoCDRepoDir returns the path to the local ArgoCD app of apps repo directory.
func GetArgoCDRepoDir() string {
	dataDir := GetDataDir()
	reposDir := filepath.Join(dataDir, "repos")
	argoCDRepoDir := filepath.Join(reposDir, ArgoCDRepoName)
	return argoCDRepoDir
}

// -----------------------------------------------------------------------------
// PRIVATE FUNCTIONS
// -----------------------------------------------------------------------------

// getEnvVar returns the value of the specfied environment variable, or the specfied default if it is blank/undefined.
func getEnvVar(envVarName, defaultValue string) string {
	value := os.Getenv(envVarName)
	if value == "" {
		value = defaultValue
	}
	return value
}
