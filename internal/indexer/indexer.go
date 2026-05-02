package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/allank/riffle/internal/embedder"
	"github.com/allank/riffle/internal/store"
	"github.com/allank/riffle/internal/vector"
	"github.com/allank/riffle/internal/walker"
)

// QueryResult is one hit from a semantic search.
type QueryResult struct {
	Path  string  `json:"path"`
	Score float32 `json:"score"`
}

// Status holds index health information returned to callers.
type Status struct {
	Index string
	Dirs  int
	Size  string
	Stale int
	Ext   []string
	Model string
	Built string
}

// Options holds optional walker settings for the Manager.
type Options struct {
	Excludes []string
	MaxDepth int
}

// Manager holds the in-memory index and coordinates reads and writes via RWMutex.
type Manager struct {
	mu        sync.RWMutex
	current   *store.Store
	root      string
	indexPath string
	emb       embedder.Embedder
	excludes  []string
	maxDepth  int
}

// New creates a Manager for the vault at root. Call Load or Reindex before querying.
func New(root string, emb embedder.Embedder, opts ...Options) *Manager {
	m := &Manager{
		root:      root,
		indexPath: filepath.Join(root, ".riffle", "index.bin"),
		emb:       emb,
	}
	if len(opts) > 0 {
		m.excludes = opts[0].Excludes
		m.maxDepth = opts[0].MaxDepth
	}
	return m
}

// Load reads the existing index from disk into memory.
// Returns an error (wrapping os.ErrNotExist) if no index file exists.
func (m *Manager) Load() error {
	idx, err := vector.New(384, 5000)
	if err != nil {
		return err
	}
	s, err := store.Open(m.indexPath, idx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.current = s
	m.mu.Unlock()
	return nil
}

// Reindex walks the vault and builds a new in-memory index, swapping it in under
// a write lock. When full is true, the existing store is ignored and everything
// is re-embedded from scratch.
func (m *Manager) Reindex(ctx context.Context, full bool) error {
	m.mu.RLock()
	existing := m.current
	if full {
		existing = nil
	}
	var exts []string
	if existing != nil {
		exts = existing.ExtList
	} else {
		exts = []string{".md"}
	}
	m.mu.RUnlock()

	if err := os.MkdirAll(filepath.Join(m.root, ".riffle"), 0755); err != nil {
		return err
	}

	wCfg := walker.Config{
		Root:        m.root,
		Extensions:  exts,
		Excludes:    m.excludes,
		MaxDepth:    m.maxDepth,
		Concurrency: runtime.NumCPU(),
	}
	w := walker.New(wCfg, m.emb)

	results, errs := w.Walk(ctx)

	newStore := store.New(m.root, exts)
	idx, err := vector.New(384, 1000)
	if err != nil {
		return err
	}
	newStore.Vector = idx

	var nextVID uint32

	for r := range results {
		vid := nextVID
		nextVID++

		if existing != nil {
			if existNode, ok := existing.NodeByPath(r.RelPath); ok {
				if existNode.MerkleHash == r.MerkleHash {
					if existing.Vector != nil {
						if vec, ok := existing.Vector.Get(uint64(existNode.VectorID)); ok {
							_ = idx.Add(uint64(vid), vec)
						}
					}
					newStore.AddNode(store.Node{
						RelPath:    r.RelPath,
						MerkleHash: r.MerkleHash,
						VectorID:   vid,
						MTime:      existNode.MTime,
					})
					continue
				}
			}
		}

		if r.Vector != nil {
			_ = idx.Add(uint64(vid), r.Vector)
		}
		newStore.AddNode(store.Node{
			RelPath:    r.RelPath,
			MerkleHash: r.MerkleHash,
			VectorID:   vid,
		})
	}

	for e := range errs {
		return e
	}

	if err := newStore.Save(m.indexPath); err != nil {
		return fmt.Errorf("save index: %w", err)
	}

	m.mu.Lock()
	m.current = newStore
	m.mu.Unlock()

	return nil
}

// Query embeds the query text and returns the top-k nearest directories above threshold.
// Paths are relative to the vault root.
func (m *Manager) Query(ctx context.Context, q string, top int, threshold float64) ([]QueryResult, error) {
	vec, err := m.emb.Embed(q)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	s := m.current
	m.mu.RUnlock()

	if s == nil {
		return nil, fmt.Errorf("no index loaded")
	}
	if s.Vector == nil {
		return nil, fmt.Errorf("index has no vector data")
	}

	hits, err := s.Vector.Search(vec, top)
	if err != nil {
		return nil, err
	}

	vidToNode := make(map[uint32]store.Node, len(s.Nodes))
	for _, n := range s.Nodes {
		vidToNode[n.VectorID] = n
	}

	var results []QueryResult
	for _, h := range hits {
		if float64(h.Score) < threshold {
			continue
		}
		node, ok := vidToNode[uint32(h.ID)]
		if !ok {
			continue
		}
		results = append(results, QueryResult{Path: node.RelPath, Score: h.Score})
	}
	return results, nil
}

// Status returns a snapshot of the current index statistics. Safe to call concurrently.
func (m *Manager) Status() Status {
	m.mu.RLock()
	s := m.current
	m.mu.RUnlock()

	if s == nil {
		return Status{Index: m.indexPath}
	}

	size := ""
	if info, err := os.Stat(m.indexPath); err == nil {
		mb := float64(info.Size()) / (1024 * 1024)
		size = fmt.Sprintf("%.1fMB", mb)
	}

	built := time.Unix(s.Header.BuildTime, 0).UTC().Format(time.RFC3339)

	return Status{
		Index: m.indexPath,
		Dirs:  int(s.Header.DirCount),
		Size:  size,
		Stale: 0,
		Ext:   s.ExtList,
		Model: "all-MiniLM-L6-v2",
		Built: built,
	}
}
