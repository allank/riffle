package output

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ProgressMsg is sent to the Bubble Tea model to update counts.
type ProgressMsg struct {
	Changed int
	Skipped int
	Total   int
}

type progressModel struct {
	changed int
	skipped int
	total   int
}

func (m progressModel) Init() tea.Cmd { return nil }

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressMsg:
		m.changed = msg.Changed
		m.skipped = msg.Skipped
		m.total = msg.Total
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m progressModel) View() string {
	pct := 0
	if m.total > 0 {
		pct = (m.changed + m.skipped) * 100 / m.total
	}
	bar := progressBar(pct, 24)
	return fmt.Sprintf("\r  Indexing... %s %3d%%  Changed: %d  Skipped: %d",
		bar, pct, m.changed, m.skipped)
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

// WriteProgress writes a simple non-interactive progress line to w.
// Used as the non-TTY fallback when Bubble Tea is not running interactively.
func WriteProgress(w io.Writer, changed, skipped int) {
	fmt.Fprintf(w, "\r  Changed: %d   Skipped: %d  ", changed, skipped)
}
