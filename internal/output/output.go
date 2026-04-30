package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type QueryResult struct {
	Path  string
	Score float32
}

// WritePlainQuery writes one path per line (default LLM output).
func WritePlainQuery(w io.Writer, results []QueryResult) {
	for _, r := range results {
		fmt.Fprintln(w, r.Path)
	}
}

// WriteJSONQuery writes structured JSON for LLM consumption.
func WriteJSONQuery(w io.Writer, query, root string, relative bool, results []QueryResult) {
	type jsonResult struct {
		Path  string  `json:"path"`
		Score float32 `json:"score"`
	}
	payload := struct {
		Query    string       `json:"query"`
		Root     string       `json:"root"`
		Relative bool         `json:"relative"`
		Results  []jsonResult `json:"results"`
	}{
		Query:    query,
		Root:     root,
		Relative: relative,
	}
	for _, r := range results {
		payload.Results = append(payload.Results, jsonResult{Path: r.Path, Score: r.Score})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(payload)
}

// WriteIndexLLM writes the LLM-mode index completion line.
func WriteIndexLLM(w io.Writer, path string, changed, skipped int, exts []string, durSecs float64) {
	fmt.Fprintf(w, "indexed path=%s changed=%d skipped=%d ext=%s duration=%.1fs\n",
		path, changed, skipped, strings.Join(exts, ","), durSecs)
}

// WriteStatusLLM writes the LLM-mode status line.
func WriteStatusLLM(w io.Writer, indexPath string, dirs int, sizeMB float64, stale int, exts []string, relative bool, model string, buildTime string) {
	fmt.Fprintf(w, "index=%s dirs=%d size=%.1fMB stale=%d ext=%s relative=%v model=%s built=%s\n",
		indexPath, dirs, sizeMB, stale, strings.Join(exts, ","), relative, model, buildTime)
}

// Pretty styles (Lip Gloss)
var (
	scoreStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	pathStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	headerStyle = lipgloss.NewStyle().Bold(true).Underline(true)
)

// WritePrettyQuery renders a human-readable scored table.
func WritePrettyQuery(w io.Writer, query, root string, results []QueryResult) {
	fmt.Fprintf(w, "\n Query: %q   root: %s\n\n", query, root)
	fmt.Fprintf(w, "  %s  %s\n", headerStyle.Render("Score"), headerStyle.Render("Path"))
	fmt.Fprintf(w, "  %s  %s\n", strings.Repeat("─", 5), strings.Repeat("─", 42))
	for _, r := range results {
		fmt.Fprintf(w, "  %s   %s\n",
			scoreStyle.Render(fmt.Sprintf("%.2f", r.Score)),
			pathStyle.Render(r.Path))
	}
	fmt.Fprintln(w)
}

// WritePrettyStatus renders human-readable index stats.
func WritePrettyStatus(w io.Writer, indexPath string, dirs int, sizeMB float64, stale int, exts []string, relative bool, model string, buildTime string) {
	label := lipgloss.NewStyle().Bold(true)
	row := func(k, v string) {
		fmt.Fprintf(w, " %-24s %s\n", label.Render(k), v)
	}
	fmt.Fprintf(w, "\n Index: %s\n %s\n", indexPath, strings.Repeat("─", 37))
	row("Directories indexed", fmt.Sprintf("%d", dirs))
	row("Stale entries", fmt.Sprintf("%d", stale))
	row("Index size", fmt.Sprintf("%.1f MB", sizeMB))
	row("Extensions", strings.Join(exts, ", "))
	row("Relative paths", fmt.Sprintf("%v", relative))
	row("Embedding model", model)
	row("Last built", buildTime)
	fmt.Fprintln(w)
}
