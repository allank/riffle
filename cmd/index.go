package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/allank/riffle/internal/config"
	"github.com/allank/riffle/internal/output"
	"github.com/allank/riffle/internal/store"
	"github.com/allank/riffle/internal/vector"
	"github.com/allank/riffle/internal/walker"
)

func init() {
	rootCmd.AddCommand(indexCmd)
	indexCmd.Flags().Bool("full", false, "Force full re-index")
	indexCmd.Flags().Int("depth", 0, "Maximum directory depth (0=unlimited)")
	indexCmd.Flags().StringSlice("ext", nil, "File extensions to include")
	indexCmd.Flags().Int("concurrency", 0, "Goroutine count (0=NumCPU)")
	indexCmd.Flags().Bool("pretty", false, "Show progress")
}

var indexCmd = &cobra.Command{
	Use:   "index <path>",
	Short: "Build or update the semantic index",
	Args:  cobra.ExactArgs(1),
	RunE:  runIndex,
}

func runIndex(cmd *cobra.Command, args []string) error {
	root, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return err
	}
	if ext, _ := cmd.Flags().GetStringSlice("ext"); len(ext) > 0 {
		cfg.Ext = ext
	}
	if d, _ := cmd.Flags().GetInt("depth"); d > 0 {
		cfg.Depth = d
	}
	if c, _ := cmd.Flags().GetInt("concurrency"); c > 0 {
		cfg.Concurrency = c
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = runtime.NumCPU()
	}
	pretty, _ := cmd.Flags().GetBool("pretty")
	full, _ := cmd.Flags().GetBool("full")

	indexDir := filepath.Join(root, ".riffle")
	if err := os.MkdirAll(indexDir, 0755); err != nil {
		return err
	}
	indexPath := filepath.Join(indexDir, "index.bin")

	emb, err := loadEmbedder()
	if err != nil {
		return fmt.Errorf("embedder: %w", err)
	}
	defer emb.Close()

	// Load existing store for incremental hashing
	var existing *store.Store
	if !full {
		if idx, err := vector.New(384, 0); err == nil {
			if s, err := store.Open(indexPath, idx); err == nil {
				existing = s
			}
		}
	}

	wCfg := walker.Config{
		Root:        root,
		Extensions:  cfg.Ext,
		MaxDepth:    cfg.Depth,
		Concurrency: cfg.Concurrency,
	}
	w := walker.New(wCfg, emb)

	start := time.Now()
	results, errs := w.Walk(cmd.Context())

	newStore := store.New(root, cfg.Ext)
	idx, err := vector.New(384, 1000)
	if err != nil {
		return err
	}
	newStore.Vector = idx

	var changed, skipped int
	var nextVID uint32

	for r := range results {
		if existing != nil {
			if existNode, ok := existing.NodeByPath(r.RelPath); ok {
				if existNode.MerkleHash == r.MerkleHash {
					newStore.AddNode(store.Node{
						RelPath:    r.RelPath,
						MerkleHash: r.MerkleHash,
						VectorID:   existNode.VectorID,
						MTime:      existNode.MTime,
					})
					skipped++
					if pretty {
						output.WriteProgress(os.Stderr, changed, skipped)
					}
					continue
				}
			}
		}
		vid := nextVID
		nextVID++
		if r.Vector != nil {
			_ = idx.Add(uint64(vid), r.Vector)
		}
		newStore.AddNode(store.Node{
			RelPath:    r.RelPath,
			MerkleHash: r.MerkleHash,
			VectorID:   vid,
		})
		changed++
		if pretty {
			output.WriteProgress(os.Stderr, changed, skipped)
		}
	}
	for err := range errs {
		return err
	}
	if pretty {
		fmt.Fprintln(os.Stderr)
	}

	if err := newStore.Save(indexPath); err != nil {
		return fmt.Errorf("save index: %w", err)
	}

	dur := time.Since(start).Seconds()
	output.WriteIndexLLM(os.Stdout, root, changed, skipped, cfg.Ext, dur)
	return nil
}
