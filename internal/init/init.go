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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// -----------------------------------------------------------------------------
// VALIDATION FUNCTIONS
// -----------------------------------------------------------------------------

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
	if zarf.IsInitialized(ctx) {
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
	if !zarf.IsInitialized(ctx) {
		return fmt.Errorf("Zarf not initialized. Run InitZarf() first")
	}

	// Get packages directory from settings.
	packagesDir := settings.InternalPackagesDir()

	// Deploy packages.
	slog.Info("Deploying packages...", "dir", packagesDir)
	deployDir := filepath.Join(packagesDir, "deploy")
	if err := zarf.DeployPackages(deployDir); err != nil {
		return fmt.Errorf("failed to deploy packages: %w", err)
	}

	// Mirror packages.
	slog.Info("Mirroring packages...", "dir", packagesDir)
	mirrorDir := filepath.Join(packagesDir, "mirror")
	if err := zarf.MirrorPackages(mirrorDir); err != nil {
		return fmt.Errorf("failed to mirror packages: %w", err)
	}

	slog.Info("Package initialization completed successfully")
	return nil
}

// InitArgoCDRepoSecrets creates ArgoCD repository secrets for accessing
// the internal Zarf Git and OCI registry.
// Requires Zarf to be initialized first.
func InitArgoCDRepoSecrets(ctx context.Context) error {
	slog.Info("Adding ArgoCD repository secrets...")

	// Check prerequisite: Zarf must be initialized.
	if !zarf.IsInitialized(ctx) {
		return fmt.Errorf("Zarf not initialized. Run InitZarf() first")
	}

	zarfCluster, err := zarf.GetCluster(ctx)
	if err != nil {
		return fmt.Errorf("could not get cluster: %w", err)
	}

	zarfState, err := zarfCluster.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("could not get zarf state: %w", err)
	}

	gitServerInfo := zarfState.GitServer
	registryInfo := zarfState.RegistryInfo

	k8s := zarfCluster.Clientset

	namespace := "bsl-ironbark-argocd"

	// Create Git repository secret.
	gitSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repository-zarf-git-http",
			Namespace: namespace,
			Labels: map[string]string{
				"argocd.argoproj.io/secret-type": "repository",
			},
		},
		StringData: map[string]string{
			"url":      gitServerInfo.Address,
			"username": gitServerInfo.PushUsername,
			"password": gitServerInfo.PushPassword,
		},
	}

	_, err = k8s.CoreV1().Secrets(namespace).Create(ctx, gitSecret, metav1.CreateOptions{})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create Git repository secret: %w", err)
		}
		slog.Info("Git repository secret already exists, skipping")
	} else {
		slog.Info("Created Git repository secret")
	}

	// Create OCI registry secret.
	registrySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repository-zarf-helm-oci-http",
			Namespace: namespace,
			Labels: map[string]string{
				"argocd.argoproj.io/secret-type": "repository",
			},
		},
		StringData: map[string]string{
			"type":          "helm",
			"name":          "zarf-registry",
			"url":           registryInfo.Address,
			"username":      registryInfo.PullUsername,
			"password":      registryInfo.PullPassword,
			"enableOCI":     "true",
			"tlsClientCert": "",
			"tlsClientKey":  "",
		},
	}

	_, err = k8s.CoreV1().Secrets(namespace).Create(ctx, registrySecret, metav1.CreateOptions{})
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create OCI registry secret: %w", err)
		}
		slog.Info("OCI registry secret already exists, skipping")
	} else {
		slog.Info("Created OCI registry secret")
	}

	slog.Info("Successfully added ArgoCD repository secrets")
	return nil
}

// InitArgoCDAppOfAppsRepo creates and pushes the ArgoCD App of Apps repository.
// Requires Zarf to be initialized first.
func InitArgoCDAppOfAppsRepo(ctx context.Context) error {
	slog.Info("Initialising ArgoCD App of Apps repository ...")

	// Check prerequisite: Zarf must be initialized.
	if !zarf.IsInitialized(ctx) {
		return fmt.Errorf("Zarf not initialized. Run InitZarf() first")
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

	// Check prerequisite: Zarf must be initialized (pass empty context for this check).
	if !zarf.IsInitialized(context.Background()) {
		return fmt.Errorf("Zarf not initialized. Run InitZarf() first")
	}

	// Apply the bootstrap App of Apps manifest.
	bootstrapManifest, err := resources.ReadFile("resources/bootstrap-argocd-app-of-apps.yaml")
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
