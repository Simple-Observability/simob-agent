package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "simob",
	Short: "SimpleObservability agent CLI",
}

func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(statusCmd)
}
