/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"bytes"
	"fmt"

	"brightsparklabs.com/ironbark/internal/zarf"
	"brightsparklabs.com/ironbark/resources"
	"github.com/spf13/cobra"
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
	secret, err := generateArgoRepoSecret()
	exitOnError(err, "Could not generate ArgoCD repository secret")

	logger.Info("Generated ArgoCD repository secret", "secret", secret)
}

type UsernamePassword struct {
	Username string
	Password string
}

func generateArgoRepoSecret() (string, error) {
	registryInfo, err := zarf.GetRegistryInfo()
	if err != nil {
		return "", fmt.Errorf("could not load zarf registry info: %w", err)
	}

	secretTemplate, err := resources.LoadArgoRepoSecretTemplate()
	if err != nil {
		return "", fmt.Errorf("could not load argo repo secret template : %w", err)
	}

	var buffer bytes.Buffer
	err = secretTemplate.Execute(&buffer, UsernamePassword{Username: registryInfo.PullUsername, Password: registryInfo.PullPassword})
	if err != nil {
		return "", fmt.Errorf("could not populate argo repo secret template : %w", err)
	}

	result := buffer.String()
	return result, nil
}
