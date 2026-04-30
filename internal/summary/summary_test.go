package summary_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allank/riffle/internal/summary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeVault(t *testing.T) string {
	dir := t.TempDir()
	sub := filepath.Join(dir, "security", "oauth2")
	require.NoError(t, os.MkdirAll(sub, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "token-refresh.md"), []byte(`---
tags: [auth]
---
OAuth 2.0 Token Refresh
Describes the refresh token rotation pattern
Related: [[PKCE Flow]]
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "pkce-flow.md"), []byte(`PKCE Flow
Authorization code with PKCE
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "image.png"), []byte("binary"), 0644))
	return dir
}

func TestSummaryContainsPath(t *testing.T) {
	root := makeVault(t)
	sub := filepath.Join(root, "security", "oauth2")
	s, err := summary.Build(sub, "security/oauth2", summary.Config{Extensions: []string{".md"}, MaxTokens: 512})
	require.NoError(t, err)
	assert.Contains(t, s, "security/oauth2")
}

func TestSummaryContainsFilenames(t *testing.T) {
	root := makeVault(t)
	sub := filepath.Join(root, "security", "oauth2")
	s, err := summary.Build(sub, "security/oauth2", summary.Config{Extensions: []string{".md"}, MaxTokens: 512})
	require.NoError(t, err)
	assert.Contains(t, s, "token-refresh.md")
	assert.Contains(t, s, "pkce-flow.md")
	assert.NotContains(t, s, "image.png", "non-md files must be excluded from summary")
}

func TestSummarySkipsFrontmatter(t *testing.T) {
	root := makeVault(t)
	sub := filepath.Join(root, "security", "oauth2")
	s, err := summary.Build(sub, "security/oauth2", summary.Config{Extensions: []string{".md"}, MaxTokens: 512})
	require.NoError(t, err)
	assert.NotContains(t, s, "tags:", "YAML frontmatter must be stripped")
	assert.Contains(t, s, "OAuth 2.0 Token Refresh")
}

func TestSummaryTruncatesToTokens(t *testing.T) {
	root := makeVault(t)
	sub := filepath.Join(root, "security", "oauth2")
	s, err := summary.Build(sub, "security/oauth2", summary.Config{Extensions: []string{".md"}, MaxTokens: 10})
	require.NoError(t, err)
	words := strings.Fields(s)
	assert.LessOrEqual(t, len(words), 15, "summary must respect approximate token budget")
}
