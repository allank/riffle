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

func TestWatcherExcludeBasename(t *testing.T) {
	root := t.TempDir()
	// Create a subdirectory that should be excluded.
	excluded := filepath.Join(root, "monorepo")
	require.NoError(t, os.MkdirAll(filepath.Join(excluded, "pkg"), 0755))
	kept := filepath.Join(root, "notes")
	require.NoError(t, os.MkdirAll(kept, 0755))

	w := watcher.New(root, 50*time.Millisecond, "monorepo")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, w.Start(ctx))

	// A write inside the excluded subtree should produce no notification.
	require.NoError(t, os.WriteFile(filepath.Join(excluded, "main.go"), []byte("x"), 0644))
	select {
	case <-w.Notify():
		t.Fatal("received notification for write inside excluded directory")
	case <-time.After(300 * time.Millisecond):
		// Good — excluded directory events are not surfaced.
	}

	// A write in a non-excluded directory should still notify.
	require.NoError(t, os.WriteFile(filepath.Join(kept, "note.md"), []byte("x"), 0644))
	select {
	case <-w.Notify():
		// Good.
	case <-time.After(2 * time.Second):
		t.Fatal("expected notification for write in non-excluded directory")
	}
}

func TestWatcherExcludeRelPath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "work/monorepo/pkg"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "work/notes"), 0755))

	w := watcher.New(root, 50*time.Millisecond, "work/monorepo")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, w.Start(ctx))

	// Write inside the relative-path-excluded subtree — no notification.
	require.NoError(t, os.WriteFile(filepath.Join(root, "work/monorepo/main.go"), []byte("x"), 0644))
	select {
	case <-w.Notify():
		t.Fatal("received notification for write inside relative-path-excluded directory")
	case <-time.After(300 * time.Millisecond):
		// Good.
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
