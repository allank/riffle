package watcher

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	ModeEvents  = "events"
	ModePolling = "polling"
)

var hardExcludes = map[string]bool{
	".git": true, "node_modules": true, ".riffle": true, ".obsidian": true,
}

// errTooManyFiles is a sentinel used to abort the WalkDir when EMFILE is hit.
var errTooManyFiles = errors.New("too many open files")

func isTooManyFiles(err error) bool {
	return errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE)
}

// Watcher watches a directory tree for changes and sends on Notify() when a
// re-index should be triggered. Debounces bursts of events into a single signal.
type Watcher struct {
	root         string
	excludes     []string
	debounce     time.Duration
	pollInterval time.Duration
	mode         atomic.Value // stores string: ModeEvents or ModePolling
	notifyCh     chan struct{}
}

// New creates a Watcher for the directory at root with the given debounce window.
// excludes is a list of patterns matched against directory basenames or relative
// paths from root (e.g. "vendor", "work/monorepo"). Matched directories and their
// subtrees are skipped entirely — not watched and not indexed.
func New(root string, debounce time.Duration, excludes ...string) *Watcher {
	w := &Watcher{
		root:         root,
		excludes:     excludes,
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

// isExcluded reports whether absPath should be excluded from watching.
// A path is excluded if its basename or its path relative to root matches
// any pattern in w.excludes (using filepath.Match glob syntax), or if the
// relative path is equal to or a sub-path of an exclude pattern.
func (w *Watcher) isExcluded(absPath string) bool {
	name := filepath.Base(absPath)
	rel, _ := filepath.Rel(w.root, absPath)
	for _, pat := range w.excludes {
		// Basename glob match: "vendor" or "*.gen"
		if matched, _ := filepath.Match(pat, name); matched {
			return true
		}
		// Relative path exact or prefix match: "work/monorepo"
		cleanPat := filepath.Clean(pat)
		if rel == cleanPat || strings.HasPrefix(rel, cleanPat+string(os.PathSeparator)) {
			return true
		}
		// Relative path glob match: "work/*/vendor"
		if matched, _ := filepath.Match(cleanPat, rel); matched {
			return true
		}
	}
	return false
}

// Start begins watching. Returns an error if the watcher cannot be initialised.
// The watch goroutine stops when ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Add the root and all non-excluded subdirectories.
	// Skip hard-excluded and user-excluded dirs to conserve file descriptors.
	// On EMFILE (too many open files), fall back to polling rather than failing.
	walkErr := filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if hardExcludes[d.Name()] || w.isExcluded(path) {
			return filepath.SkipDir
		}
		if addErr := fw.Add(path); addErr != nil {
			if isTooManyFiles(addErr) {
				return errTooManyFiles
			}
		}
		return nil
	})
	if errors.Is(walkErr, errTooManyFiles) {
		fw.Close()
		log.Printf("warn too_many_open_files=true falling_back=polling interval=30s")
		w.mode.Store(ModePolling)
		go w.runPolling(ctx)
		return nil
	}
	if walkErr != nil {
		fw.Close()
		return walkErr
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
			// When a new directory is created, watch it unless excluded.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !w.isExcluded(event.Name) {
						if addErr := fw.Add(event.Name); addErr != nil && isTooManyFiles(addErr) {
							log.Printf("warn too_many_open_files=true falling_back=polling interval=30s")
							w.mode.Store(ModePolling)
							fw.Close()
							w.runPolling(ctx)
							return
						}
					}
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
