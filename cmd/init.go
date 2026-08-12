/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"brightsparklabs.com/ironbark/internal/constants"
	ironbarkGit "brightsparklabs.com/ironbark/internal/git"
	ironbarkInit "brightsparklabs.com/ironbark/internal/init"
	"brightsparklabs.com/ironbark/internal/settings"
	"brightsparklabs.com/ironbark/internal/zarf"
	"brightsparklabs.com/ironbark/resources"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
)

// -----------------------------------------------------------------------------
// COMMAND: ROOT
// -----------------------------------------------------------------------------

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialises Ironbark components",
	}

	cmd.AddCommand(newInitAllCmd())
	cmd.AddCommand(newInitPackagesCmd())
	cmd.AddCommand(newInitArgoSecretsCmd())
	cmd.AddCommand(newInitArgoAppOfAppsRepoCmd())
	cmd.AddCommand(newInitArgoAppCmd())

	return cmd
}

func newInitAllCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "all",
		Short: "Initialises all Ironbark components",
		RunE:  execInitAll,
	}
}

// execInitAll runs every Ironbark initialisation step in order.
// Failures are surfaced as user errors via the top-level handler in
// `Execute`.
func execInitAll(cmd *cobra.Command, args []string) error {
	slog.Info("Initialising Ironbark components ...")

	if err := initPackages(cmd.Context()); err != nil {
		return WrapUserError(err, "Could not initialise Ironbark packages")
	}
	if err := initArgoRepoSecrets(cmd.Context()); err != nil {
		return WrapUserError(err, "Could not add ArgoCD repository secrets")
	}
	if err := initArgoAppOfAppsRepo(cmd.Context()); err != nil {
		return WrapUserError(err, "Could not initialise ArgoCD App of Apps repo")
	}
	if err := initArgoApp(); err != nil {
		return WrapUserError(err, "Could not apply ArgoCD App of Apps definition")
	}

	slog.Info("Successfully initialised all Ironbark components")
	return nil
}

// -----------------------------------------------------------------------------
// COMMAND: packages
// -----------------------------------------------------------------------------

func newInitPackagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "packages",
		Short: "Deploys and mirrors Ironbark packages",
		RunE:  execInitPackages,
	}
}

// execInitPackages deploys and mirrors all Ironbark Zarf packages.
// Failures are surfaced as user errors via the top-level handler in
// `Execute`.
func execInitPackages(cmd *cobra.Command, args []string) error {
	slog.Info("Initialising packages ...")
	if err := initPackages(cmd.Context()); err != nil {
		return WrapUserError(err, "Could not initialise packages")
	}
	slog.Info("Successfully initialised packages")
	return nil
}

// initPackages mirrors and deploys every Ironbark Zarf package found
// under the configured internal packages directory.
//
// Charts staged inside mirror packages are pushed to the in-cluster
// registry as a post-processing step (see AFC-32 and
// `zarf.MirrorPackageCharts`) because `zarf package mirror-resources`
// itself does not mirror Helm charts.
func initPackages(ctx context.Context) error {
	packagesDir := settings.InternalPackagesDir()

	slog.Info("Mirroring packages ...")
	mirrorPackages := filepath.Join(packagesDir, "mirror")
	if err := zarf.MirrorPackages(mirrorPackages); err != nil {
		return fmt.Errorf("could not mirror packages: %w", err)
	}

	slog.Info("Mirroring helm charts from packages ...")
	if err := zarf.MirrorPackageCharts(ctx, mirrorPackages); err != nil {
		return fmt.Errorf("could not mirror package charts: %w", err)
	}

	slog.Info("Deploying packages ...")
	deployPackages := filepath.Join(packagesDir, "deploy")
	if err := zarf.DeployPackages(deployPackages); err != nil {
		return fmt.Errorf("could not deploy packages: %w", err)
	}

	return nil
}

// -----------------------------------------------------------------------------
// COMMAND: argocd-repo-secrets
// -----------------------------------------------------------------------------

func newInitArgoSecretsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "argocd-repo-secrets",
		Short: "Initialises Ironbark ArgoCD repository secrets",
		RunE:  execInitArgoSecrets,
	}
}

// execInitArgoSecrets creates the Kubernetes Secrets ArgoCD needs to
// authenticate against the internal Git server. Failures are surfaced
// as user errors via the top-level handler in `Execute`.
func execInitArgoSecrets(cmd *cobra.Command, args []string) error {
	slog.Info("Adding ArgoCD repository secrets ...")
	if err := initArgoRepoSecrets(cmd.Context()); err != nil {
		return WrapUserError(err, "Could not add ArgoCD repository secrets")
	}
	slog.Info("Successfully added ArgoCD repository secrets")
	return nil
}

func initArgoRepoSecrets(ctx context.Context) error {
	// Delegate to shared implementation in internal/init package.
	return ironbarkInit.InitArgoCDRepoSecrets(ctx)
}

// -----------------------------------------------------------------------------
// COMMAND: argocd-app-of-apps-repo
// -----------------------------------------------------------------------------

func newInitArgoAppOfAppsRepoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "argocd-app-of-apps-repo",
		Short: "Initialises the ArgoCD App of Apps repo on the internal Git server",
		RunE:  execInitArgoAppOfAppsRepo,
	}
}

// execInitArgoAppOfAppsRepo creates the ArgoCD App of Apps repository
// on the internal Git server and pushes the initial templates to it.
// Failures are surfaced as user errors via the top-level handler in
// `Execute`.
func execInitArgoAppOfAppsRepo(cmd *cobra.Command, args []string) error {
	slog.Info("Initialising ArgoCD App of Apps repo ...")
	if err := initArgoAppOfAppsRepo(cmd.Context()); err != nil {
		return WrapUserError(err, "Could not initialise ArgoCD App of Apps repo")
	}
	slog.Info("Successfully initialised ArgoCD App of Apps repo")
	return nil
}

// initArgoExec initialises the local ArgoCD app of apps repo and pushes it to the internal Git server.
func initArgoAppOfAppsRepo(ctx context.Context) error {
	repoName := constants.ArgoCDRepoName
	slog.Info("Creating ArgoCD repo ...", "repo", repoName)

	repo, err := ironbarkGit.CreateRepoArgoCDAppOfApps(ctx)
	if err != nil {
		return fmt.Errorf("could not create remote ArgoCD repo: %w", err)
	}
	slog.Info("Successfully created repo", "url", repo.HTMLURL)

	argoCDRepoDir := settings.ArgoCDRepoDir()
	if _, err = createLocalArgoCDRepo(argoCDRepoDir); err != nil {
		return fmt.Errorf("could not create local ArgoCD repo: %w", err)
	}

	err = ironbarkGit.PushRepoArgoCDAppOfApps(ctx, argoCDRepoDir)
	if err != nil {
		return fmt.Errorf("could not push argo repo: %w", err)
	}
	slog.Info("Successfully pushed repo", "url", repo.HTMLURL)

	return nil
}

// createLocalArgoCDRepo creates the local ArgoCD app of apps repo in the specified directory.
func createLocalArgoCDRepo(dir string) (*git.Repository, error) {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("could not create repo dir: %w", err)
	}
	slog.Info("Created argo repo dir", "dir", settings.DisplayPath(dir))

	localRepo, err := git.PlainInit(dir, false)
	if err != nil {
		return nil, fmt.Errorf("could not init repo: %w", err)
	}
	slog.Info("Initialised argo local repo")

	copyAppOfAppResources(dir)

	w, _ := localRepo.Worktree()
	_, _ = w.Add(".")
	_, err = w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Ironbark",
			Email: "ironbark@brightsparklabs.dev",
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("could not create initial commit: %w", err)
	}

	return localRepo, nil
}

// copyAppOfAppResources populates the local ArgoCD app of apps repo directory using the embedded resources template.
func copyAppOfAppResources(dir string) error {
	err := resources.Copy("repos/ironbark-argocd-app-of-apps", dir)
	return err
}

// -----------------------------------------------------------------------------
// COMMAND: argocd-app
// -----------------------------------------------------------------------------

func newInitArgoAppCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "argocd-app",
		Short: "Deploys ArgoCD app of apps to start syncing state",
		RunE:  execInitArgoApp,
	}
}

// execInitArgoApp applies the ArgoCD App of Apps definition that
// makes ArgoCD start reconciling the cluster against the managed
// repository. Failures are surfaced as user errors via the top-level
// handler in `Execute`.
func execInitArgoApp(cmd *cobra.Command, args []string) error {
	slog.Info("Deploying and starting ArgoCD app of apps ...")
	if err := initArgoApp(); err != nil {
		return WrapUserError(err, "Could not apply ArgoCD App of Apps definition")
	}
	slog.Info("Successfully deployed ArgoCD app of apps")
	return nil
}

func initArgoApp() error {
	dir, err := os.MkdirTemp("", "ironbark-")
	if err != nil {
		return fmt.Errorf("could not create temporary file for ArgoCD App of Apps definition: %w", err)
	}
	defer os.RemoveAll(dir)

	definitionFilename := "bootstrap-argocd-app-of-apps.yaml"
	definitionFile := filepath.Join(dir, definitionFilename)
	err = resources.Copy(definitionFilename, definitionFile)
	if err != nil {
		return fmt.Errorf("could not extract ArgoCD App of Apps definition: %w", err)
	}

	command := exec.Command("kubectl", "apply", "-f", definitionFile)
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("could not apply ArgoCD App of Apps definition: %w", err)
	}

	fmt.Println(string(output))
	slog.Info("Successfully deployed ArgoCD app of apps")
	return nil
}
