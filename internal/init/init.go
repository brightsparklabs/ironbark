/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/

package init

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"brightsparklabs.com/ironbark/internal/constants"
	ironbarkGit "brightsparklabs.com/ironbark/internal/git"
	"brightsparklabs.com/ironbark/internal/settings"
	"brightsparklabs.com/ironbark/internal/zarf"
	"brightsparklabs.com/ironbark/resources"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

// -----------------------------------------------------------------------------
// VALIDATION FUNCTIONS
// -----------------------------------------------------------------------------

// requireZarfInitialized checks if Zarf is initialized and returns an error if not.
// This helper eliminates repetitive initialization checks throughout the codebase.
func requireZarfInitialized(ctx context.Context) error {
	initialized, err := zarf.IsInitialized(ctx)
	if err != nil {
		return fmt.Errorf("failed to check Zarf initialization status: %w", err)
	}
	if !initialized {
		return fmt.Errorf("Zarf not initialized. Run InitZarf() first")
	}
	return nil
}

// CanInitZarf checks if prerequisites for Zarf initialization are met.
// Returns nil if Zarf init can proceed, error otherwise.
func CanInitZarf() error {
	// Check 1: zarf binary must be in PATH.
	if _, err := exec.LookPath("zarf"); err != nil {
		return fmt.Errorf("zarf binary not found in PATH")
	}

	// Check 2: zarf-init package must exist.
	zarfInitDir := "/app/resources/zarf/init"
	files, err := os.ReadDir(zarfInitDir)
	if err != nil {
		return fmt.Errorf("zarf-init directory not found: %w", err)
	}

	found := false
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "zarf-init-") && strings.HasSuffix(file.Name(), ".tar.zst") {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("no zarf-init package found in %s", zarfInitDir)
	}

	return nil
}

// -----------------------------------------------------------------------------
// INITIALIZATION FUNCTIONS
// -----------------------------------------------------------------------------

// InitZarf initializes Zarf on the cluster with the git-server component.
// This is idempotent - it will skip if Zarf is already initialized.
func InitZarf(ctx context.Context) error {
	slog.Info("Initializing Zarf...")

	// Check if already initialized.
	initialized, err := zarf.IsInitialized(ctx)
	if err != nil {
		return fmt.Errorf("failed to check Zarf initialization status: %w", err)
	}
	if initialized {
		slog.Info("Zarf already initialized, skipping")
		return nil
	}

	// Validate prerequisites.
	if err := CanInitZarf(); err != nil {
		return fmt.Errorf("cannot initialize Zarf: %w", err)
	}

	// Find the zarf-init package path.
	zarfInitDir := "/app/resources/zarf/init"
	files, err := os.ReadDir(zarfInitDir)
	if err != nil {
		return fmt.Errorf("could not read zarf-init directory: %w", err)
	}

	var zarfInitPackage string
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "zarf-init-") && strings.HasSuffix(file.Name(), ".tar.zst") {
			zarfInitPackage = filepath.Join(zarfInitDir, file.Name())
			break
		}
	}

	if zarfInitPackage == "" {
		return fmt.Errorf("zarf-init package not found in %s", zarfInitDir)
	}

	// Get kubeconfig path (uploaded or mounted).
	kubeconfigPath, err := getKubeconfigPath()
	if err != nil {
		return fmt.Errorf("no kubeconfig available: %w", err)
	}

	// Run zarf init with git-server component.
	slog.Info("Running zarf init with git-server component...", "package", zarfInitPackage, "kubeconfig", kubeconfigPath)
	cmd := exec.Command("zarf", "init", "--components=git-server", "--confirm", zarfInitPackage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set KUBECONFIG environment variable so zarf can connect to the cluster.
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("zarf init failed: %w", err)
	}

	slog.Info("Zarf initialization completed successfully")
	return nil
}

// InitPackages deploys and mirrors Ironbark packages.
// Requires Zarf to be initialized first.
func InitPackages(ctx context.Context) error {
	slog.Info("Initializing packages...")

	// Check prerequisite: Zarf must be initialized.
	if err := requireZarfInitialized(ctx); err != nil {
		return err
	}

	// Get packages directory from settings.
	packagesDir := settings.InternalPackagesDir()

	// Deploy packages.
	slog.Info("Deploying packages...", "dir", packagesDir)
	deployDir := filepath.Join(packagesDir, "deploy")
	if err := zarf.DeployPackages(deployDir); err != nil {
		return fmt.Errorf("failed to deploy packages: %w", err)
	}

	// Mirror packages (images and charts).
	slog.Info("Mirroring packages (images and charts)...", "dir", packagesDir)
	mirrorDir := filepath.Join(packagesDir, "mirror")
	if err := zarf.MirrorPackagesWithCharts(ctx, mirrorDir); err != nil {
		return fmt.Errorf("failed to mirror packages: %w", err)
	}

	slog.Info("Package initialization completed successfully")
	return nil
}

// InitArgoCDRepoSecrets creates ArgoCD repository secrets for accessing
// the internal Zarf Git and OCI registry.
// Requires Zarf to be initialized first.
//
// This creates three secrets using cluster-internal service DNS names:
//  1. repository-zarf-helm-oci-http - Internal registry (HTTP)
//  2. repository-zarf-helm-oci-https - TLS proxy registry (HTTPS)
//  3. repository-zarf-git-http - Git server
//
// All secrets include "zarf.dev/agent": "ignore" to bypass Zarf agent webhook validation.
func InitArgoCDRepoSecrets(ctx context.Context) error {
	slog.Info("Adding ArgoCD repository secrets...")

	// Check prerequisite: Zarf must be initialized.
	if err := requireZarfInitialized(ctx); err != nil {
		return err
	}

	zarfCluster, err := zarf.GetCluster(ctx)
	if err != nil {
		return fmt.Errorf("could not load zarf cluster: %w", err)
	}

	registryInfo, err := zarf.GetRegistryInfo(ctx)
	if err != nil {
		return fmt.Errorf("could not load zarf registry info: %w", err)
	}

	gitInfo, err := zarf.GetGitServerInfo(ctx)
	if err != nil {
		return fmt.Errorf("could not load zarf git server info: %w", err)
	}

	// Secret 1: Helm OCI HTTP (internal cluster registry).
	slog.Info("Adding Helm OCI HTTP", "url", "zarf-docker-registry.zarf.svc.cluster.local:5000")
	helmSecret := v1ac.Secret("repository-zarf-helm-oci-http", constants.ArgoCDNamespace).
		WithLabels(map[string]string{
			"argocd.argoproj.io/secret-type": "repository",
			"zarf.dev/agent":                 "ignore",
		}).
		WithData(map[string][]byte{
			"url":       []byte("zarf-docker-registry.zarf.svc.cluster.local:5000"),
			"username":  []byte(registryInfo.PullUsername),
			"password":  []byte(registryInfo.PullPassword),
			"type":      []byte("helm"),
			"enableOCI": []byte("true"),
			"insecure":  []byte("true"),
			// TODO: Does not seem to do anything.
			"insecureOCIForceHttp": []byte("true"),
		})
	_, err = zarfCluster.Clientset.CoreV1().Secrets(*helmSecret.Namespace).Apply(
		ctx, helmSecret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	if err != nil {
		return fmt.Errorf("could not create ArgoCD zarf registry secret: %w", err)
	}

	// Secret 2: Helm OCI HTTPS (TLS proxy).
	slog.Info("Adding Helm OCI HTTPS", "url", "internal-tls-proxy.bsl-ironbark-internal-tls-proxy.svc.cluster.local")
	helmTlsSecret := v1ac.Secret("repository-zarf-helm-oci-https", constants.ArgoCDNamespace).
		WithLabels(map[string]string{
			"argocd.argoproj.io/secret-type": "repository",
			"zarf.dev/agent":                 "ignore",
		}).
		WithData(map[string][]byte{
			"url":       []byte("internal-tls-proxy.bsl-ironbark-internal-tls-proxy.svc.cluster.local"),
			"username":  []byte(registryInfo.PullUsername),
			"password":  []byte(registryInfo.PullPassword),
			"type":      []byte("helm"),
			"enableOCI": []byte("true"),
			"insecure":  []byte("true"),
		})
	_, err = zarfCluster.Clientset.CoreV1().Secrets(*helmTlsSecret.Namespace).Apply(
		ctx, helmTlsSecret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	if err != nil {
		return fmt.Errorf("could not create ArgoCD zarf registry TLS secret: %w", err)
	}

	// Secret 3: Git HTTP.
	slog.Info("Adding Git HTTP", "url", "http://zarf-gitea-http.zarf.svc.cluster.local:3000/zarf-git-user/ironbark-argocd-app-of-apps")
	gitSecret := v1ac.Secret("repository-zarf-git-http", constants.ArgoCDNamespace).
		WithLabels(map[string]string{
			"argocd.argoproj.io/secret-type": "repository",
			"zarf.dev/agent":                 "ignore",
		}).
		WithData(map[string][]byte{
			"url":      []byte("http://zarf-gitea-http.zarf.svc.cluster.local:3000/zarf-git-user/ironbark-argocd-app-of-apps"),
			"username": []byte(gitInfo.PushUsername),
			"password": []byte(gitInfo.PushPassword),
			"type":     []byte("git"),
		})
	_, err = zarfCluster.Clientset.CoreV1().Secrets(*gitSecret.Namespace).Apply(
		ctx, gitSecret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	if err != nil {
		return fmt.Errorf("could not create ArgoCD zarf git server secret: %w", err)
	}

	slog.Info("Successfully added ArgoCD repository secrets")
	return nil
}

// InitArgoCDAppOfAppsRepo creates and pushes the ArgoCD App of Apps repository.
// Requires Zarf to be initialized first.
func InitArgoCDAppOfAppsRepo(ctx context.Context) error {
	slog.Info("Initialising ArgoCD App of Apps repository ...")

	// Check prerequisite: Zarf must be initialized.
	if err := requireZarfInitialized(ctx); err != nil {
		return err
	}

	// Create local repository.
	localDir := filepath.Join(settings.DataDir(), "repos", constants.ArgoCDRepoName)
	localRepo, err := createLocalArgoCDRepo(localDir)
	if err != nil {
		return fmt.Errorf("could not create local repository: %w", err)
	}

	// Copy resources to repository.
	if err := copyAppOfAppResources(localDir); err != nil {
		return fmt.Errorf("could not copy resources: %w", err)
	}

	// Commit changes.
	worktree, err := localRepo.Worktree()
	if err != nil {
		return fmt.Errorf("could not get worktree: %w", err)
	}

	_, err = worktree.Add(".")
	if err != nil {
		return fmt.Errorf("could not stage changes: %w", err)
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Ironbark",
			Email: "ironbark@brightsparklabs.com",
		},
	})
	if err != nil {
		return fmt.Errorf("could not commit changes: %w", err)
	}

	// Push to Gitea.
	if err := ironbarkGit.PushRepoArgoCDAppOfApps(ctx, localDir); err != nil {
		return fmt.Errorf("could not push repository: %w", err)
	}

	slog.Info("Successfully initialised ArgoCD App of Apps repository")
	return nil
}

// InitArgoCDApp deploys the ArgoCD App of Apps.
// Requires Zarf to be initialized first.
func InitArgoCDApp() error {
	slog.Info("Deploying ArgoCD App of Apps ...")

	// Check prerequisite: Zarf must be initialized.
	if err := requireZarfInitialized(context.Background()); err != nil {
		return err
	}

	// Apply the bootstrap App of Apps manifest.
	bootstrapManifest, err := resources.ReadFile("bootstrap-argocd-app-of-apps.yaml")
	if err != nil {
		return fmt.Errorf("could not read bootstrap manifest: %w", err)
	}

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(bootstrapManifest))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("could not apply bootstrap manifest: %w", err)
	}

	slog.Info("Successfully deployed ArgoCD App of Apps")
	return nil
}

// InitAll runs all initialization steps in order.
// This is smart and idempotent - it will skip steps that are already complete.
func InitAll(ctx context.Context) error {
	slog.Info("Running full initialization...")

	// Step 1: Initialize Zarf (if not already done).
	if err := InitZarf(ctx); err != nil {
		return fmt.Errorf("Zarf initialization failed: %w", err)
	}

	// Step 2: Deploy/mirror packages.
	if err := InitPackages(ctx); err != nil {
		return fmt.Errorf("Package initialization failed: %w", err)
	}

	// Step 3: Create ArgoCD repository secrets.
	if err := InitArgoCDRepoSecrets(ctx); err != nil {
		return fmt.Errorf("ArgoCD secrets initialization failed: %w", err)
	}

	// Step 4: Create and push App of Apps repository.
	if err := InitArgoCDAppOfAppsRepo(ctx); err != nil {
		return fmt.Errorf("App of Apps repository initialization failed: %w", err)
	}

	// Step 5: Deploy ArgoCD App of Apps.
	if err := InitArgoCDApp(); err != nil {
		return fmt.Errorf("App of Apps deployment failed: %w", err)
	}

	slog.Info("Full initialization completed successfully!")
	return nil
}

// -----------------------------------------------------------------------------
// HELPER FUNCTIONS
// -----------------------------------------------------------------------------

func createLocalArgoCDRepo(dir string) (*git.Repository, error) {
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return git.PlainOpen(dir)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create directory: %w", err)
	}

	return git.PlainInit(dir, false)
}

func copyAppOfAppResources(dir string) error {
	sourceDir := "repos/ironbark-argocd-app-of-apps"
	return resources.Copy(sourceDir, dir)
}

// getKubeconfigPath returns the path to the kubeconfig file, preferring
// uploaded over mounted.
func getKubeconfigPath() (string, error) {
	// Check for uploaded kubeconfig first (takes precedence).
	// Must match the path used in internal/api/kubeconfig.go.
	uploadedPath := "/tmp/ironbark/uploaded-kubeconfig"
	if _, err := os.Stat(uploadedPath); err == nil {
		return uploadedPath, nil
	}

	// Fall back to mounted kubeconfig.
	if settings.InContainer() {
		mountedPath := "/mnt/conf/kubeconfig"
		if _, err := os.Stat(mountedPath); err == nil {
			return mountedPath, nil
		}
	}

	// Try standard location.
	homeDir, err := os.UserHomeDir()
	if err == nil {
		defaultPath := filepath.Join(homeDir, ".kube", "config")
		if _, err := os.Stat(defaultPath); err == nil {
			return defaultPath, nil
		}
	}

	return "", fmt.Errorf("no kubeconfig found (upload via API or mount to container)")
}

// DeployAndMirrorPackage deploys a single Zarf package and mirrors its charts.
// This is the same logic used by InitPackages but for a single uploaded package.
func DeployAndMirrorPackage(ctx context.Context, packagePath string) error {
	slog.Info("Deploying and mirroring package", "package", packagePath)

	// Create a temporary directory for the package.
	tmpDir, err := os.MkdirTemp("", "ironbark-package-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Copy package to temp directory (zarf commands expect a directory).
	packageName := filepath.Base(packagePath)
	tmpPackagePath := filepath.Join(tmpDir, packageName)
	input, err := os.ReadFile(packagePath)
	if err != nil {
		return fmt.Errorf("failed to read package: %w", err)
	}
	if err := os.WriteFile(tmpPackagePath, input, 0644); err != nil {
		return fmt.Errorf("failed to copy package to temp dir: %w", err)
	}

	// Deploy package.
	if err := zarf.DeployPackages(tmpDir); err != nil {
		return fmt.Errorf("failed to deploy package: %w", err)
	}

	// Mirror images and charts.
	if err := zarf.MirrorPackagesWithCharts(ctx, tmpDir); err != nil {
		return fmt.Errorf("failed to mirror package: %w", err)
	}

	slog.Info("Package deployed and charts mirrored", "package", packageName)
	return nil
}
