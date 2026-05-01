package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/allank/riffle/internal/config"
	"github.com/allank/riffle/internal/output"
	"github.com/allank/riffle/internal/store"
	"github.com/allank/riffle/internal/vector"
)

func init() {
	rootCmd.AddCommand(queryCmd)
	queryCmd.Flags().String("index", "", "Path to index root (auto-discovers from CWD if empty)")
	queryCmd.Flags().Int("top", 0, "Number of results (0=use config)")
	queryCmd.Flags().Float64("threshold", 0.0, "Minimum similarity score")
	queryCmd.Flags().String("format", "", "Output format: plain, json, yaml")
	queryCmd.Flags().Bool("relative", true, "Output relative paths")
	queryCmd.Flags().Bool("pretty", false, "Human-readable table")
}

var queryCmd = &cobra.Command{
	Use:   "query <text>",
	Short: "Find the most semantically relevant directories",
	Args:  cobra.ExactArgs(1),
	RunE:  runQuery,
}

func runQuery(cmd *cobra.Command, args []string) error {
	queryText := args[0]
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return err
	}
	if top, _ := cmd.Flags().GetInt("top"); top > 0 {
		cfg.Top = top
	}
	if f, _ := cmd.Flags().GetString("format"); f != "" {
		cfg.Format = f
	}
	pretty, _ := cmd.Flags().GetBool("pretty")
	relative, _ := cmd.Flags().GetBool("relative")
	threshold, _ := cmd.Flags().GetFloat64("threshold")

	indexRoot, _ := cmd.Flags().GetString("index")
	if indexRoot == "" {
		indexRoot, err = discoverIndexRoot()
		if err != nil {
			return err
		}
	}
	indexPath := filepath.Join(indexRoot, ".riffle", "index.bin")

	idx, err := vector.New(384, 5000)
	if err != nil {
		return err
	}
	s, err := store.Open(indexPath, idx)
	if err != nil {
		return fmt.Errorf("open index %s: %w (run riffle index first)", indexPath, err)
	}

	emb, err := loadEmbedder()
	if err != nil {
		return err
	}
	defer emb.Close()

	vec, err := emb.Embed(queryText)
	if err != nil {
		return err
	}

	hits, err := s.Vector.Search(vec, cfg.Top)
	if err != nil {
		return err
	}

	var results []output.QueryResult
	for _, h := range hits {
		if float64(h.Score) < threshold {
			continue
		}
		if int(h.ID) >= len(s.Nodes) {
			continue
		}
		path := s.Nodes[h.ID].RelPath
		if !relative {
			path = filepath.Join(indexRoot, path)
		}
		results = append(results, output.QueryResult{Path: path, Score: h.Score})
	}

	if len(results) == 0 {
		os.Exit(1)
	}

	switch {
	case pretty:
		output.WritePrettyQuery(os.Stdout, queryText, indexRoot, results)
	case cfg.Format == "json":
		output.WriteJSONQuery(os.Stdout, queryText, indexRoot, relative, results)
	default:
		output.WritePlainQuery(os.Stdout, results)
	}
	return nil
}
