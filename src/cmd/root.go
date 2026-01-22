/*
Copyright © 2026 brightSPARK Labs <www.brightsparklabs.com>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	//zarf "github.com/zarf-dev/zarf/src/cmd"

	_ "unsafe" // For go:linkname
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

//go:linkname newK9sCommand github.com/zarf-dev/zarf/src/cmd.newK9sCommand
func newK9sCommand() *cobra.Command

var nestedK9sCmd = &cobra.Command{
	Use: "k9s-zsh",
	Run: func(cmd *cobra.Command, args []string) {
		k9sCmd := newK9sCommand()
		k9sCmd.Run(cmd, []string{"completion", "zsh"})
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
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
