package indexer_test

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/allank/riffle/internal/indexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmbedder returns a deterministic 384-dim vector derived from the text hash.
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(text string) ([]float32, error) {
	h := sha256.Sum256([]byte(text))
	v := make([]float32, 384)
	for i := range v {
		v[i] = float32(h[i%32]+1) / 256.0
	}
	return v, nil
}

func (m *mockEmbedder) Close() error { return nil }

func makeVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dirs := map[string]string{
		"security/oauth2": "OAuth 2.0 token refresh and PKCE flows",
		"projects/auth":   "Authentication service for the platform",
	}
	for dir, content := range dirs {
		abs := filepath.Join(root, dir)
		require.NoError(t, os.MkdirAll(abs, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(abs, "notes.md"), []byte(content), 0644))
	}
	return root
}

func TestManagerReindex(t *testing.T) {
	vault := makeVault(t)
	mgr := indexer.New(vault, &mockEmbedder{})

	require.NoError(t, mgr.Reindex(context.Background(), false))

	st := mgr.Status()
	assert.Equal(t, 4, st.Dirs)
	assert.Equal(t, []string{".md"}, st.Ext)
	assert.Equal(t, "all-MiniLM-L6-v2", st.Model)
	assert.NotEmpty(t, st.Built)
	assert.NotEmpty(t, st.Index)
}

func TestManagerQuery(t *testing.T) {
	vault := makeVault(t)
	mgr := indexer.New(vault, &mockEmbedder{})
	require.NoError(t, mgr.Reindex(context.Background(), false))

	results, err := mgr.Query(context.Background(), "OAuth token", 5, 0.0)
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	// All returned paths must be relative
	for _, r := range results {
		assert.False(t, filepath.IsAbs(r.Path), "path should be relative: %s", r.Path)
	}
}

func TestManagerIncrementalReindex(t *testing.T) {
	vault := makeVault(t)
	mgr := indexer.New(vault, &mockEmbedder{})
	require.NoError(t, mgr.Reindex(context.Background(), false))

	st1 := mgr.Status()

	// Reindex again — nothing changed, built time should advance but dirs same
	require.NoError(t, mgr.Reindex(context.Background(), false))
	st2 := mgr.Status()

	assert.Equal(t, st1.Dirs, st2.Dirs)
}

func TestManagerLoad(t *testing.T) {
	vault := makeVault(t)
	mgr := indexer.New(vault, &mockEmbedder{})
	require.NoError(t, mgr.Reindex(context.Background(), false))

	// New manager loading from disk should have same dir count
	mgr2 := indexer.New(vault, &mockEmbedder{})
	require.NoError(t, mgr2.Load())
	assert.Equal(t, mgr.Status().Dirs, mgr2.Status().Dirs)
}

func TestManagerLoadMissing(t *testing.T) {
	vault := t.TempDir()
	mgr := indexer.New(vault, &mockEmbedder{})
	err := mgr.Load()
	assert.Error(t, err) // index doesn't exist yet
}
