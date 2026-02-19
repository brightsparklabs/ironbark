/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"context"
	"fmt"

	"brightsparklabs.com/ironbark/internal/zarf"
	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func newInitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialises Ironbark components",
		Run:   initExec,
	}

	initCmd.AddCommand(newInitArgoSecretsCmd())

	return initCmd
}

func initExec(cmd *cobra.Command, args []string) {
	logger.Info("Initialising Ironbark components ...")

	addArgoRepoSecret(cmd.Context())

	logger.Info("Successfully initialised all Ironbark components")
}

func newInitArgoSecretsCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "argocd-repo-secrets",
		Short: "Initialises Ironbark ArgoCD repository secrets",
		Run:   initArgoSecrets,
	}

	return initCmd
}

func initArgoSecrets(cmd *cobra.Command, args []string) {
	logger.Info("Adding ArgoCD repository secrets ...")
	err := addArgoRepoSecret(cmd.Context())
	exitOnError(err, "Could not add ArgoCD repository secrets")
	logger.Info("Successfully added ArgoCD repository secrets")
}

func addArgoRepoSecret(ctx context.Context) error {
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
