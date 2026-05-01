package output

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// RenderProgress returns the 3-line progress string.
func RenderProgress(root string, exts []string, total, changed, skipped int, elapsed float64) string {
	done := changed + skipped
	extStr := strings.Join(exts, ",")
	if total > 0 {
		pct := done * 100 / total
		bar := progressBar(pct, 24)
		return fmt.Sprintf(
			" Indexing %s  [ext: %s]\n %s  %3d%%  %d / %d dirs\n Changed: %d   Skipped: %d   Elapsed: %.1fs",
			root, extStr, bar, pct, done, total, changed, skipped, elapsed,
		)
	}
	return fmt.Sprintf(
		" Indexing %s  [ext: %s]\n %d dirs processed\n Changed: %d   Skipped: %d   Elapsed: %.1fs",
		root, extStr, done, changed, skipped, elapsed,
	)
}

// StartProgress starts a ticker that overwrites a 3-line progress display on w every 250ms.
// getChanged and getSkipped are called on each tick to read the current counts.
// Returns a stop function; call it when indexing is done to render the final state.
func StartProgress(w io.Writer, root string, exts []string, total int, start time.Time,
	getChanged, getSkipped func() int) func() {

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	var mu sync.Mutex
	linesWritten := 0

	render := func() {
		c, s := getChanged(), getSkipped()
		text := RenderProgress(root, exts, total, c, s, time.Since(start).Seconds())
		if linesWritten > 0 {
			// cursor up N rows, carriage-return to col 0, erase to end of screen
			fmt.Fprintf(w, "\033[%dA\r\033[J", linesWritten)
		}
		fmt.Fprint(w, text)
		linesWritten = strings.Count(text, "\n") + 1
	}

	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				render()
				mu.Unlock()
			case <-stopCh:
				return
			}
		}
	}()

	return func() {
		close(stopCh)
		<-doneCh
		mu.Lock()
		render() // final render with actual end-state counts
		fmt.Fprintln(w)
		mu.Unlock()
	}
}

func progressBar(pct, width int) string {
	filled := pct * width / 100
	var sb strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			sb.WriteRune('█')
		} else {
			sb.WriteRune('░')
		}
	}
	return sb.String()
}
