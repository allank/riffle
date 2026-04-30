package summary

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Extensions []string
	MaxTokens  int
}

// Build constructs a short text summary of a directory for embedding.
// Only files matching cfg.Extensions contribute; others are ignored.
func Build(absDir string, relPath string, cfg Config) (string, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return "", err
	}
	extSet := make(map[string]bool, len(cfg.Extensions))
	for _, e := range cfg.Extensions {
		extSet[strings.ToLower(e)] = true
	}

	var filenames []string
	var firstLines []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !extSet[ext] {
			continue
		}
		filenames = append(filenames, e.Name())
		info, err := e.Info()
		if err != nil || info.Size() > 50*1024 {
			continue
		}
		lines, err := extractLines(filepath.Join(absDir, e.Name()), 5)
		if err == nil {
			firstLines = append(firstLines, lines...)
		}
	}

	var sb strings.Builder
	sb.WriteString(relPath)
	if len(filenames) > 0 {
		sb.WriteByte('\n')
		sb.WriteString(strings.Join(filenames, " "))
	}
	if len(firstLines) > 0 {
		sb.WriteByte('\n')
		sb.WriteString(strings.Join(firstLines, "\n"))
	}

	text := sb.String()
	if cfg.MaxTokens > 0 {
		text = truncateToTokens(text, cfg.MaxTokens)
	}
	return text, nil
}

// extractLines returns up to n non-empty, non-frontmatter lines from a file.
func extractLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	inFrontmatter := false
	frontmatterDone := false
	lineNum := 0

	for scanner.Scan() && len(lines) < n {
		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if lineNum == 1 && line == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter && !frontmatterDone {
			if line == "---" {
				frontmatterDone = true
				inFrontmatter = false
			}
			continue
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// truncateToTokens approximates token count as whitespace-separated words.
// Actual token truncation happens in Tokenizer.Encode before embedding.
func truncateToTokens(text string, maxTokens int) string {
	words := strings.Fields(text)
	approx := int(float64(maxTokens) * 0.75) // words ≈ 0.75 × tokens (subword units)
	if approx < 1 {
		approx = 1
	}
	if len(words) <= approx {
		return text
	}
	return strings.Join(words[:approx], " ")
}
