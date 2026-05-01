package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/allank/riffle/internal/embedder"
)

// loadEmbedder loads the ONNX embedder from RIFFLE_MODEL_PATH, embedded bytes, or errors.
func loadEmbedder() (embedder.Embedder, error) {
	modelPath := os.Getenv("RIFFLE_MODEL_PATH")
	tokPath := os.Getenv("RIFFLE_TOKENIZER_PATH")
	if modelPath != "" {
		return embedder.NewONNX(modelPath, tokPath)
	}
	if len(embedder.EmbeddedModel()) > 0 {
		return embedder.NewFromBytes(embedder.EmbeddedModel(), embedder.EmbeddedTokenizer())
	}
	return nil, fmt.Errorf("no model found: set RIFFLE_MODEL_PATH or build with -tags embedmodel")
}

// discoverIndexRoot walks up from CWD looking for a .riffle directory.
func discoverIndexRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".riffle")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no .riffle index found walking up from CWD; run riffle index <path>")
}
