package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allank/riffle/internal/merkle"
	"github.com/allank/riffle/internal/store"
	"github.com/allank/riffle/internal/vector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.bin")

	idx, err := vector.New(4, 5)
	require.NoError(t, err)
	require.NoError(t, idx.Add(0, []float32{1, 0, 0, 0}))
	require.NoError(t, idx.Add(1, []float32{0, 1, 0, 0}))

	mtime := time.Unix(1000, 0)
	h1 := merkle.FileHash("a.md", mtime, 100)
	h2 := merkle.FileHash("b.md", mtime, 200)

	s := store.New("/tmp/root", []string{".md"})
	s.AddNode(store.Node{
		RelPath:    "security/oauth2",
		MerkleHash: h1,
		VectorID:   0,
		MTime:      mtime.Unix(),
	})
	s.AddNode(store.Node{
		RelPath:    "projects/auth",
		MerkleHash: h2,
		VectorID:   1,
		MTime:      mtime.Unix(),
	})
	s.SetRootHash([32]byte{0xAB})
	s.Vector = idx

	require.NoError(t, s.Save(path))

	loaded, err := store.Open(path, vector.NewFlat(4))
	require.NoError(t, err)
	assert.Equal(t, uint32(2), loaded.Header.DirCount)
	assert.Equal(t, []string{".md"}, loaded.ExtList)
	assert.Equal(t, "security/oauth2", loaded.Nodes[0].RelPath)
	assert.Equal(t, "projects/auth", loaded.Nodes[1].RelPath)
	assert.Equal(t, [32]byte{0xAB}, loaded.Header.RootHash)
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.bin")
	s := store.New("/tmp/root", []string{".md"})
	idx, _ := vector.New(4, 5)
	s.Vector = idx
	require.NoError(t, s.Save(path))
	// No .tmp file should remain
	_, err := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "tmp file must be cleaned up after atomic rename")
}
