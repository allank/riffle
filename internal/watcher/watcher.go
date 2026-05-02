package watcher

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	ModeEvents  = "events"
	ModePolling = "polling"
)

// Watcher watches a directory tree for changes and sends on Notify() when a
// re-index should be triggered. Debounces bursts of events into a single signal.
type Watcher struct {
	root         string
	debounce     time.Duration
	pollInterval time.Duration
	mode         atomic.Value // stores string: ModeEvents or ModePolling
	notifyCh     chan struct{}
}

// New creates a Watcher for the directory at root with the given debounce window.
func New(root string, debounce time.Duration) *Watcher {
	w := &Watcher{
		root:         root,
		debounce:     debounce,
		pollInterval: 30 * time.Second,
		notifyCh:     make(chan struct{}, 1),
	}
	w.mode.Store(ModeEvents)
	return w
}

// Mode returns the current event delivery mode: "events" or "polling".
func (w *Watcher) Mode() string {
	return w.mode.Load().(string)
}

// Notify returns a channel that receives a struct{} when a re-index is needed.
// The channel is buffered(1); if a signal is already pending, a new one is dropped.
func (w *Watcher) Notify() <-chan struct{} {
	return w.notifyCh
}

func (w *Watcher) sendNotify() {
	select {
	case w.notifyCh <- struct{}{}:
	default:
	}
}

// Start begins watching. Returns an error if the watcher cannot be initialised.
// The watch goroutine stops when ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Add the root and all current subdirectories.
	if err := filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return fw.Add(path)
		}
		return nil
	}); err != nil {
		fw.Close()
		return err
	}

	go w.run(ctx, fw)
	return nil
}

func (w *Watcher) run(ctx context.Context, fw *fsnotify.Watcher) {
	defer fw.Close()

	var debounceTimer *time.Timer

	resetDebounce := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(w.debounce, w.sendNotify)
	}

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-fw.Events:
			if !ok {
				return
			}
			// When a new directory is created, watch it immediately.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = fw.Add(event.Name)
				}
			}
			resetDebounce()

		case watchErr, ok := <-fw.Errors:
			if !ok {
				return
			}
			log.Printf("warn event_subscription_lost=true falling_back=polling interval=30s err=%v", watchErr)
			w.mode.Store(ModePolling)
			fw.Close()
			w.runPolling(ctx)
			return
		}
	}
}

func (w *Watcher) runPolling(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sendNotify()
		}
	}
}
