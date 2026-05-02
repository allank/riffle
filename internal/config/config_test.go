package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allank/riffle/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	cfg := config.Defaults()
	assert.Equal(t, 5, cfg.Top)
	assert.Equal(t, "plain", cfg.Format)
	assert.False(t, cfg.Pretty)
	assert.True(t, cfg.Relative)
	assert.Equal(t, []string{".md"}, cfg.Ext)
	assert.Equal(t, 0, cfg.Depth)
	assert.Equal(t, 0, cfg.Concurrency)
}

func TestLoadMergesFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(f, []byte(`
[defaults]
top = 10
format = "json"
`), 0644))

	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, 10, cfg.Top)
	assert.Equal(t, "json", cfg.Format)
	assert.True(t, cfg.Relative) // default preserved
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := config.Load("/nonexistent/config.toml")
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.Top)
}

func TestWatchDefaults(t *testing.T) {
	cfg := config.Defaults()
	assert.Equal(t, "127.0.0.1:7424", cfg.WatchListen)
	assert.Equal(t, 500, cfg.WatchDebounceMs)
}

func TestWatchLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(f, []byte(`
[watch]
listen = "0.0.0.0:9000"
debounce_ms = 250
`), 0644))

	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:9000", cfg.WatchListen)
	assert.Equal(t, 250, cfg.WatchDebounceMs)
}
