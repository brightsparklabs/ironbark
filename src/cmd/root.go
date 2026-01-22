/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	zarf "github.com/zarf-dev/zarf/src/cmd"
)

const envCliName = "IRONBARK_CLI_NAME"

func getCliName() string {
	cliName := os.Getenv(envCliName)
	if cliName == "" {
		cliName = "ironbark"
	}
	return cliName
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "ironbark",
	Annotations: map[string]string{
		cobra.CommandDisplayNameAnnotation: getCliName(),
	},
	Short: "A brief description of your application",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	zarfCmd := zarf.NewZarfCommand()
	rootCmd.AddCommand(zarfCmd)

	var toolsCmd *cobra.Command
	for _, cmd := range zarfCmd.Commands() {
		if cmd.Use == "tools" {
			toolsCmd = cmd
			break
		}
	}
	if toolsCmd == nil {
		fmt.Printf("Could not find `tools` command")
		os.Exit(1)
	}

	var k9sCmd *cobra.Command
	for _, cmd := range toolsCmd.Commands() {
		if cmd.Use == "monitor" {
			k9sCmd = cmd
			break
		}
	}
	if k9sCmd == nil {
		fmt.Printf("Could not find `k9s` command")
		os.Exit(1)
	}

	var nestedK9sCmd = &cobra.Command{
		Use: "k9s-zsh",
		Run: func(cmd *cobra.Command, args []string) {
			k9sCmd.Run(cmd, []string{"completion", "zsh"})
		},
	}
	rootCmd.AddCommand(nestedK9sCmd)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.ironbark.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
