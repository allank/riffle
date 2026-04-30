package embedder_test

import (
	"os"
	"testing"

	"github.com/allank/riffle/internal/embedder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfNoModel(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("model/model.onnx"); os.IsNotExist(err) {
		t.Skip("model/model.onnx not present; run make fetch-model")
	}
}

func TestEmbedDimensions(t *testing.T) {
	skipIfNoModel(t)
	emb, err := embedder.NewONNX("model/model.onnx", "../tokenizer/data/tokenizer.json")
	require.NoError(t, err)
	defer emb.Close()

	vec, err := emb.Embed("hello world")
	require.NoError(t, err)
	assert.Len(t, vec, 384, "all-MiniLM-L6-v2 outputs 384-dim vectors")
}

func TestEmbedNormalized(t *testing.T) {
	skipIfNoModel(t)
	emb, err := embedder.NewONNX("model/model.onnx", "../tokenizer/data/tokenizer.json")
	require.NoError(t, err)
	defer emb.Close()

	vec, err := emb.Embed("OAuth token refresh")
	require.NoError(t, err)

	var norm float32
	for _, v := range vec {
		norm += v * v
	}
	assert.InDelta(t, 1.0, norm, 0.01)
}

func TestEmbedSimilarTextsCloser(t *testing.T) {
	skipIfNoModel(t)
	emb, err := embedder.NewONNX("model/model.onnx", "../tokenizer/data/tokenizer.json")
	require.NoError(t, err)
	defer emb.Close()

	a, _ := emb.Embed("OAuth token authentication")
	b, _ := emb.Embed("OAuth access token refresh")
	c, _ := emb.Embed("gardening soil composition")

	simAB := cosineSim(a, b)
	simAC := cosineSim(a, c)
	assert.Greater(t, simAB, simAC, "semantically similar texts should score higher")
}

func cosineSim(a, b []float32) float32 {
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot
}
