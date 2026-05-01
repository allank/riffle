package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/allank/riffle/cmd.Version=vX.Y.Z".
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "riffle",
	Short: "Semantic directory index CLI",
}

func init() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("riffle {{.Version}}\n")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(2)
	}
}
