package walker_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/allank/riffle/internal/walker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTree(t *testing.T) string {
	root := t.TempDir()
	dirs := []string{
		"security/oauth2",
		"projects/auth",
		"projects/api",
		".git/objects",
		"node_modules/react",
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "security/oauth2/token.md"), []byte("OAuth 2.0"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "projects/auth/readme.md"), []byte("Auth service"), 0644))
	return root
}

func collect(t *testing.T, results <-chan walker.Result, errs <-chan error) []string {
	t.Helper()
	var paths []string
	for r := range results {
		paths = append(paths, r.RelPath)
	}
	for err := range errs {
		require.NoError(t, err)
	}
	sort.Strings(paths)
	return paths
}

func TestWalkerExcludesHardCoded(t *testing.T) {
	root := makeTree(t)
	cfg := walker.Config{Root: root, Extensions: []string{".md"}, Concurrency: 1}
	w := walker.New(cfg, nil)
	results, errs := w.Walk(context.Background())
	paths := collect(t, results, errs)

	for _, p := range paths {
		assert.NotContains(t, p, ".git", ".git must be excluded")
		assert.NotContains(t, p, "node_modules", "node_modules must be excluded")
	}
}

func TestWalkerFindsAllDirs(t *testing.T) {
	root := makeTree(t)
	cfg := walker.Config{Root: root, Extensions: []string{".md"}, Concurrency: 1}
	w := walker.New(cfg, nil)
	results, errs := w.Walk(context.Background())
	paths := collect(t, results, errs)

	assert.Contains(t, paths, "security/oauth2")
	assert.Contains(t, paths, "projects/auth")
	assert.Contains(t, paths, "projects/api")
}

func TestWalkerDepthLimit(t *testing.T) {
	root := makeTree(t)
	cfg := walker.Config{Root: root, Extensions: []string{".md"}, MaxDepth: 1, Concurrency: 1}
	w := walker.New(cfg, nil)
	results, errs := w.Walk(context.Background())
	paths := collect(t, results, errs)

	for _, p := range paths {
		assert.NotContains(t, p, "/", "depth=1 should not yield sub-sub-dirs: "+p)
	}
}
