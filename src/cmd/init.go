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

type UsernamePassword struct {
	Username string
	Password string
}

func addArgoRepoSecret(cmd *cobra.Command) error {
	registryInfo, err := zarf.GetRegistryInfo()
	if err != nil {
		return fmt.Errorf("could not load zarf registry info: %w", err)
	}

	secret := v1ac.Secret("zarf-helm-oci", "bsl-baseline-argocd").
		WithLabels(map[string]string{
			"argocd.argoproj.io/secret-type": "repository",
			"zarf.dev/agent":                 "ignore",
		}).
		WithData(map[string][]byte{
			"url":                  []byte("zarf-docker-registry.zarf.svc.cluster.local/ironbark-helm-charts"),
			"username":             []byte(registryInfo.PullUsername),
			"password":             []byte(registryInfo.PullPassword),
			"type":                 []byte("helm"),
			"enableOCI":            []byte("true"),
			"insecure":             []byte("true"),
			"insecureOCIForceHttp": []byte("true"),
		})

	zarfCluster, err := zarf.GetCluster()
	if err != nil {
		return fmt.Errorf("could not load zarf cluster: %w", err)
	}
	_, err = zarfCluster.Clientset.CoreV1().Secrets(*secret.Namespace).Apply(
		cmd.Context(), secret, metav1.ApplyOptions{Force: true, FieldManager: "ironbark"})
	return err
}
