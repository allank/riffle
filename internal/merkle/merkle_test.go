package merkle_test

import (
	"testing"
	"time"

	"github.com/allank/riffle/internal/merkle"
	"github.com/stretchr/testify/assert"
)

func TestFileHashDeterministic(t *testing.T) {
	mtime := time.Unix(1000, 0)
	h1 := merkle.FileHash("foo.md", mtime, 512)
	h2 := merkle.FileHash("foo.md", mtime, 512)
	assert.Equal(t, h1, h2)
}

func TestFileHashVariesByInput(t *testing.T) {
	mtime := time.Unix(1000, 0)
	h1 := merkle.FileHash("foo.md", mtime, 512)
	h2 := merkle.FileHash("bar.md", mtime, 512)
	h3 := merkle.FileHash("foo.md", mtime, 1024)
	assert.NotEqual(t, h1, h2)
	assert.NotEqual(t, h1, h3)
}

func TestDirHashOrderIndependent(t *testing.T) {
	mtime := time.Unix(1000, 0)
	h1 := merkle.FileHash("a.md", mtime, 1)
	h2 := merkle.FileHash("b.md", mtime, 2)
	d1 := merkle.DirHash([][32]byte{h1, h2})
	d2 := merkle.DirHash([][32]byte{h2, h1})
	assert.Equal(t, d1, d2, "DirHash must sort children before hashing")
}

func TestDirHashChangesWhenChildChanges(t *testing.T) {
	mtime := time.Unix(1000, 0)
	h1 := merkle.FileHash("a.md", mtime, 1)
	h2 := merkle.FileHash("b.md", mtime, 2)
	h2b := merkle.FileHash("b.md", mtime, 999)
	d1 := merkle.DirHash([][32]byte{h1, h2})
	d2 := merkle.DirHash([][32]byte{h1, h2b})
	assert.NotEqual(t, d1, d2)
}
