package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ATLAS %s\n", Version)
		fmt.Printf("  Git commit: %s\n", GitCommit)
		fmt.Printf("  Build time: %s\n", BuildTime)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
