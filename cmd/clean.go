package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().String("index", "", "Path to index root")
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove the .riffle index directory",
	RunE:  runClean,
}

func runClean(cmd *cobra.Command, args []string) error {
	indexRoot, _ := cmd.Flags().GetString("index")
	var err error
	if indexRoot == "" {
		indexRoot, err = discoverIndexRoot()
		if err != nil {
			return err
		}
	}
	riffleDir := filepath.Join(indexRoot, ".riffle")
	if err := os.RemoveAll(riffleDir); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", riffleDir)
	return nil
}
