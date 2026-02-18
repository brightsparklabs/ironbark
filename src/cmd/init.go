/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
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

	return initCmd
}

func initExec(cmd *cobra.Command, args []string) {
	err := addArgoRepoSecret(cmd)
	exitOnError(err, "Could not add ArgoCD repository secret to k8s")

	logger.Info("Successfully added ArgoCD repository secret to k8s")
}

func addArgoRepoSecret(cmd *cobra.Command) error {
	ctx := cmd.Context()
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

	helmSecret := v1ac.Secret("repository-zarf-helm-oci", "bsl-ironbark-argocd").
		WithLabels(map[string]string{
			"argocd.argoproj.io/secret-type": "repository",
			"zarf.dev/agent":                 "ignore",
		}).
		WithData(map[string][]byte{
			"url":                  []byte("zarf-docker-registry.zarf.svc.cluster.local:5000"),
			"username":             []byte(registryInfo.PullUsername),
			"password":             []byte(registryInfo.PullPassword),
			"type":                 []byte("helm"),
			"enableOCI":            []byte("true"),
			"insecure":             []byte("true"),
			"insecureOCIForceHttp": []byte("true"),
		})
	_, err = zarfCluster.Clientset.CoreV1().Secrets(*helmSecret.Namespace).Apply(
		cmd.Context(), helmSecret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	if err != nil {
		return fmt.Errorf("could not create ArgoCD zarf registry secret: %w", err)
	}

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
		cmd.Context(), gitSecret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	if err != nil {
		return fmt.Errorf("could not create ArgoCD zarf git server secret: %w", err)
	}

	return nil
}
