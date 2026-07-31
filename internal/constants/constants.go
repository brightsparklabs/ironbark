/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

// Package constants holds genuine compile-time constants used across
// the application. Anything that resolves at runtime (e.g. from an
// environment variable) belongs in `internal/settings` instead.
package constants

// -----------------------------------------------------------------------------
// CONSTANTS
// -----------------------------------------------------------------------------

// ArgoCDRepoName is the name of the git repository for the ArgoCD App
// of Apps.
const ArgoCDRepoName string = "ironbark-argocd-app-of-apps"

// ArgoCDNamespace is the Kubernetes namespace where ArgoCD is deployed.
const ArgoCDNamespace string = "bsl-ironbark-argocd"
