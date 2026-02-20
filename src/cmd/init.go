/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"brightsparklabs.com/ironbark/internal/zarf"
	"brightsparklabs.com/ironbark/resources"
	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func newInitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialises Ironbark components",
	}

	initCmd.AddCommand(newInitAll())
	initCmd.AddCommand(newInitArgoSecretsCmd())
	initCmd.AddCommand(newInitArgoAppCmd())

	return initCmd
}

func newInitAll() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "all",
		Short: "Initialises all Ironbark components",
		Run:   execInitAll,
	}

	return initCmd
}

func execInitAll(cmd *cobra.Command, args []string) {
	logger.Info("Initialising Ironbark components ...")

	err := initArgoRepoSecrets(cmd.Context())
	exitOnError(err, "Could not add ArgoCD repository secrets")
	err = initArgoApp()
	exitOnError(err, "Could not apply ArgoCD App of Apps definition")

	logger.Info("Successfully initialised all Ironbark components")
}

func newInitArgoSecretsCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "argocd-repo-secrets",
		Short: "Initialises Ironbark ArgoCD repository secrets",
		Run:   execInitArgoSecrets,
	}

	return initCmd
}

func execInitArgoSecrets(cmd *cobra.Command, args []string) {
	logger.Info("Adding ArgoCD repository secrets ...")
	err := initArgoRepoSecrets(cmd.Context())
	exitOnError(err, "Could not add ArgoCD repository secrets")
	logger.Info("Successfully added ArgoCD repository secrets")
}

func initArgoRepoSecrets(ctx context.Context) error {
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

	logger.Info("Adding Helm OCI HTTP", "url", "repository-zarf-helm-oci-http")
	helmSecret := v1ac.Secret("repository-zarf-helm-oci-http", "bsl-ironbark-argocd").
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

	logger.Info("Adding Helm OCI HTTPS", "url", "internal-tls-proxy.bsl-ironbark-internal-tls-proxy.svc.cluster.local")
	helmTlsSecret := v1ac.Secret("repository-zarf-helm-oci-https", "bsl-ironbark-argocd").
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
		return fmt.Errorf("could not create ArgoCD zarf registry secret: %w", err)
	}

	logger.Info("Adding Git HTTP", "url", "http://zarf-gitea-http.zarf.svc.cluster.local:3000/zarf-git-user/ironbark-argocd-app-of-apps")
	gitSecret := v1ac.Secret("repository-zarf-git-http", "bsl-ironbark-argocd").
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
	_, err = zarfCluster.Clientset.CoreV1().Secrets(*helmSecret.Namespace).Apply(
		ctx, gitSecret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	if err != nil {
		return fmt.Errorf("could not create ArgoCD zarf git server secret: %w", err)
	}

	return nil
}

func newInitArgoAppCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "argocd-start",
		Short: "Dpeloys ArgoCD app of apps to start syncing state",
		Run:   execInitArgoApp,
	}

	return initCmd
}

func execInitArgoApp(cmd *cobra.Command, args []string) {
	logger.Info("Deploying and starting ArgoCD app of apps ...")
	err := initArgoApp()
	exitOnError(err, "Could not apply ArgoCD App of Apps definition")
	logger.Info("Successfully deployed ArgoCD app of apps")
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
	logger.Info("Successfully deployed ArgoCD app of apps")
	return nil
}
