package vector_test

import (
	"bytes"
	"testing"

	"github.com/allank/riffle/internal/vector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlatAddAndSearch(t *testing.T) {
	idx := vector.NewFlat(4)
	require.NoError(t, idx.Add(1, []float32{1, 0, 0, 0}))
	require.NoError(t, idx.Add(2, []float32{0, 1, 0, 0}))
	require.NoError(t, idx.Add(3, []float32{0, 0, 1, 0}))

	results, err := idx.Search([]float32{1, 0, 0, 0}, 2)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, uint64(1), results[0].ID, "exact match should be top result")
	assert.InDelta(t, 1.0, results[0].Score, 0.001)
}

func TestFlatRoundtrip(t *testing.T) {
	idx := vector.NewFlat(4)
	require.NoError(t, idx.Add(10, []float32{0.5, 0.5, 0, 0}))

	var buf bytes.Buffer
	require.NoError(t, idx.Save(&buf))

	idx2 := vector.NewFlat(4)
	require.NoError(t, idx2.Load(&buf))

	results, err := idx2.Search([]float32{0.5, 0.5, 0, 0}, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, uint64(10), results[0].ID)
}

func TestNewChoosesFlat(t *testing.T) {
	idx, err := vector.New(384, 100)
	require.NoError(t, err)
	assert.Equal(t, "flat", idx.Type())
}

func TestNewChoosesHNSW(t *testing.T) {
	idx, err := vector.New(384, 3000)
	require.NoError(t, err)
	assert.Equal(t, "hnsw", idx.Type())
}
