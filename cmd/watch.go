package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/allank/riffle/internal/config"
	"github.com/allank/riffle/internal/indexer"
	"github.com/allank/riffle/internal/mcpserver"
	"github.com/allank/riffle/internal/watcher"
)

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().String("listen", "", "MCP server bind address (overrides config)")
	watchCmd.Flags().StringSlice("exclude", nil, "Directory names or relative paths to exclude from watching and indexing")
}

var watchCmd = &cobra.Command{
	Use:   "watch <path>",
	Short: "Watch a vault and serve queries via MCP",
	Args:  cobra.ExactArgs(1),
	RunE:  runWatch,
}

func runWatch(cmd *cobra.Command, args []string) error {
	root, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return err
	}
	listen := cfg.WatchListen
	if l, _ := cmd.Flags().GetString("listen"); l != "" {
		listen = l
	}
	if ex, _ := cmd.Flags().GetStringSlice("exclude"); len(ex) > 0 {
		cfg.Exclude = append(cfg.Exclude, ex...)
	}

	emb, err := loadEmbedder()
	if err != nil {
		return fmt.Errorf("embedder: %w", err)
	}
	defer emb.Close()

	mgr := indexer.New(root, emb, indexer.Options{
		Excludes: cfg.Exclude,
		MaxDepth: cfg.Depth,
	})
	if err := mgr.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load index: %w", err)
		}
		log.Println("No existing index — building initial index...")
		if err := mgr.Reindex(cmd.Context(), false); err != nil {
			return fmt.Errorf("initial index: %w", err)
		}
	}

	debounce := time.Duration(cfg.WatchDebounceMs) * time.Millisecond
	w := watcher.New(root, debounce, cfg.Exclude...)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}

	srv := mcpserver.New(listen, root, mgr, w.Mode)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}

	if host, _, err := net.SplitHostPort(srv.Addr()); err == nil {
		if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
			log.Printf("warn listen=%s auth=none network_accessible=true", srv.Addr())
		}
	}
	fmt.Printf("watching path=%s listen=%s mode=%s\n", root, srv.Addr(), w.Mode())

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigterm)
	defer signal.Stop(sighup)

	// Re-index loop: triggered by watcher events or SIGHUP (full re-index).
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.Notify():
				if err := mgr.Reindex(ctx, false); err != nil && ctx.Err() == nil {
					log.Printf("error reindex=%v", err)
				}
			case <-sighup:
				log.Println("SIGHUP received — forcing full re-index")
				if err := mgr.Reindex(ctx, true); err != nil && ctx.Err() == nil {
					log.Printf("error full_reindex=%v", err)
				}
			}
		}
	}()

	select {
	case <-sigterm:
	case <-cmd.Context().Done():
	}

	log.Println("Shutting down...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	return srv.Shutdown(shutCtx)
}
