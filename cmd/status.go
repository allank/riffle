package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/allank/riffle/internal/output"
	"github.com/allank/riffle/internal/store"
	"github.com/allank/riffle/internal/vector"
)

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().String("index", "", "Path to index root")
	statusCmd.Flags().Bool("pretty", false, "Human-readable output")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index health and statistics",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	pretty, _ := cmd.Flags().GetBool("pretty")

	indexRoot, _ := cmd.Flags().GetString("index")
	var err error
	if indexRoot == "" {
		indexRoot, err = discoverIndexRoot()
		if err != nil {
			return err
		}
	}
	indexPath := filepath.Join(indexRoot, ".riffle", "index.bin")

	idx, _ := vector.New(384, 0)
	s, err := store.Open(indexPath, idx)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}

	info, err := os.Stat(indexPath)
	if err != nil {
		return err
	}
	sizeMB := float64(info.Size()) / (1024 * 1024)
	buildTime := time.Unix(s.Header.BuildTime, 0).UTC().Format(time.RFC3339)

	if pretty {
		output.WritePrettyStatus(os.Stdout, indexPath, int(s.Header.DirCount), sizeMB,
			0, s.ExtList, true, "all-MiniLM-L6-v2", buildTime)
	} else {
		output.WriteStatusLLM(os.Stdout, indexPath, int(s.Header.DirCount), sizeMB,
			0, s.ExtList, true, "all-MiniLM-L6-v2", buildTime)
	}
	return nil
}
