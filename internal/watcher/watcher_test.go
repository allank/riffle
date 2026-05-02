package watcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/allank/riffle/internal/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatcherInitialMode(t *testing.T) {
	dir := t.TempDir()
	w := watcher.New(dir, 50*time.Millisecond)
	assert.Equal(t, "events", w.Mode())
}

func TestWatcherStartSucceeds(t *testing.T) {
	dir := t.TempDir()
	w := watcher.New(dir, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, w.Start(ctx))
}

func TestWatcherDebounce(t *testing.T) {
	dir := t.TempDir()
	w := watcher.New(dir, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.Start(ctx))

	// Write several files quickly — should coalesce into one notification.
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file%d.md", i))
		require.NoError(t, os.WriteFile(path, []byte("content"), 0644))
	}

	select {
	case <-w.Notify():
		// Good — at least one notification received.
	case <-time.After(2 * time.Second):
		t.Fatal("expected notification within 2 seconds after file writes")
	}

	// Drain any buffered notifications, then verify no second one arrives quickly.
	draining := true
	for draining {
		select {
		case <-w.Notify():
		default:
			draining = false
		}
	}

	select {
	case <-w.Notify():
		t.Fatal("unexpected notification — debounce not working correctly")
	case <-time.After(200 * time.Millisecond):
		// Good — no spurious second notification.
	}
}

func TestWatcherCancelStops(t *testing.T) {
	dir := t.TempDir()
	w := watcher.New(dir, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	cancel() // Should not hang or panic after cancel.
	time.Sleep(100 * time.Millisecond)
}
