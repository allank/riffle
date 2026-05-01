package walker

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/allank/riffle/internal/embedder"
	"github.com/allank/riffle/internal/merkle"
	"github.com/allank/riffle/internal/summary"
)

var hardExcludes = map[string]bool{
	".git": true, "node_modules": true, ".riffle": true, ".obsidian": true,
}

type Config struct {
	Root        string
	Extensions  []string
	Excludes    []string
	MaxDepth    int
	Concurrency int
}

type Result struct {
	RelPath    string
	AbsPath    string
	MerkleHash [32]byte
	Vector     []float32
}

type Walker struct {
	cfg     Config
	emb     embedder.Embedder
	visited sync.Map // key: uint64 inode → struct{}, for symlink cycle detection
}

// New creates a Walker. emb may be nil (walk only, no embedding — for tests).
func New(cfg Config, emb embedder.Embedder) *Walker {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	return &Walker{cfg: cfg, emb: emb}
}

// Walk traverses the directory tree and emits Results.
func (w *Walker) Walk(ctx context.Context) (<-chan Result, <-chan error) {
	results := make(chan Result, 64)
	errs := make(chan error, 1)

	go func() {
		defer close(results)
		defer close(errs)

		sem := make(chan struct{}, w.cfg.Concurrency)
		var wg sync.WaitGroup

		err := filepath.WalkDir(w.cfg.Root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			// Handle symlinks: WalkDir uses Lstat so symlinks appear as non-dirs.
			if d.Type()&fs.ModeSymlink != 0 {
				info, statErr := os.Stat(path) // follow the symlink
				if statErr != nil {
					return nil
				}
				if !info.IsDir() {
					return nil // symlink to file, skip
				}
				// Symlink to directory: check for cycles via inode
				if st, ok := info.Sys().(*syscall.Stat_t); ok {
					if _, loaded := w.visited.LoadOrStore(st.Ino, struct{}{}); loaded {
						return filepath.SkipDir
					}
				}
				// Fall through to process as a directory
			} else if !d.IsDir() {
				return nil
			}

			rel, _ := filepath.Rel(w.cfg.Root, path)
			if rel == "." {
				return nil
			}

			name := d.Name()
			if hardExcludes[name] || w.isExcluded(name) {
				return filepath.SkipDir
			}

			depth := strings.Count(rel, string(os.PathSeparator)) + 1
			if w.cfg.MaxDepth > 0 && depth > w.cfg.MaxDepth {
				return filepath.SkipDir
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			wg.Add(1)
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Done()
				return ctx.Err()
			}
			go func(absPath, relPath string) {
				defer wg.Done()
				defer func() { <-sem }()
				r := w.processDir(absPath, relPath)
				select {
				case results <- r:
				case <-ctx.Done():
				}
			}(path, rel)
			return nil
		})
		wg.Wait()
		if err != nil {
			errs <- err
		}
	}()
	return results, errs
}

func (w *Walker) processDir(absPath, relPath string) Result {
	entries, _ := os.ReadDir(absPath)
	extSet := make(map[string]bool)
	for _, e := range w.cfg.Extensions {
		extSet[strings.ToLower(e)] = true
	}

	var childHashes [][32]byte
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !extSet[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		childHashes = append(childHashes, merkle.FileHash(e.Name(), info.ModTime(), info.Size()))
	}
	mhash := merkle.DirHash(childHashes)

	var vec []float32
	if w.emb != nil {
		sumText, _ := summary.Build(absPath, relPath, summary.Config{
			Extensions: w.cfg.Extensions,
			MaxTokens:  512,
		})
		vec, _ = w.emb.Embed(sumText)
	}

	return Result{
		RelPath:    relPath,
		AbsPath:    absPath,
		MerkleHash: mhash,
		Vector:     vec,
	}
}

func (w *Walker) isExcluded(name string) bool {
	for _, pat := range w.cfg.Excludes {
		if matched, _ := filepath.Match(pat, name); matched {
			return true
		}
	}
	return false
}
