//go:build integration

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfNoModel(t *testing.T) {
	t.Helper()
	if os.Getenv("RIFFLE_MODEL_PATH") == "" {
		t.Skip("RIFFLE_MODEL_PATH not set; skipping integration test")
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "riffle")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run())
	return bin
}

func makeTestVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dirs := map[string]string{
		"security/oauth2":   "OAuth 2.0 token refresh and PKCE flows",
		"projects/auth":     "Authentication service for the platform",
		"projects/api":      "REST API gateway and middleware",
		"devops/ci":         "Continuous integration pipeline setup",
		"docs/architecture": "System architecture decision records",
	}
	for dir, content := range dirs {
		abs := filepath.Join(root, dir)
		require.NoError(t, os.MkdirAll(abs, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(abs, "notes.md"), []byte(content), 0644))
	}
	return root
}

func TestEndToEnd(t *testing.T) {
	skipIfNoModel(t)
	bin := buildBinary(t)
	vault := makeTestVault(t)

	env := append(os.Environ(),
		"RIFFLE_MODEL_PATH="+os.Getenv("RIFFLE_MODEL_PATH"),
		"RIFFLE_TOKENIZER_PATH="+os.Getenv("RIFFLE_TOKENIZER_PATH"),
	)

	run := func(args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// 1. Index the vault
	out, err := run("index", vault)
	require.NoError(t, err, "index failed: %s", out)
	assert.Contains(t, out, "indexed path="+vault)
	assert.Contains(t, out, "skipped=0")

	// 2. Query for OAuth content — should rank security/oauth2 first or second
	out, err = run("query", "OAuth authentication token", "--index", vault)
	require.NoError(t, err, "query failed: %s", out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	assert.NotEmpty(t, lines)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "oauth2") {
			found = true
			break
		}
	}
	assert.True(t, found, "oauth2 directory should appear in OAuth query results, got: %s", out)

	// 3. Status
	out, err = run("status", "--index", vault)
	require.NoError(t, err, "status failed: %s", out)
	assert.Contains(t, out, "index=")
	assert.Contains(t, out, "ext=.md")

	// 4. Incremental re-index — nothing changed, all should be skipped
	out, err = run("index", vault)
	require.NoError(t, err, "incremental index failed: %s", out)
	assert.Contains(t, out, "changed=0")
	assert.NotContains(t, out, "skipped=0", "all dirs should be skipped on re-index")

	// 5. Modify a file and re-index — should pick up the change
	require.NoError(t, os.WriteFile(
		filepath.Join(vault, "security/oauth2/notes.md"),
		[]byte("Updated OAuth content with new token exchange patterns"),
		0644,
	))
	out, err = run("index", vault)
	require.NoError(t, err, "re-index after change failed: %s", out)
	assert.NotContains(t, out, "changed=0", "changed should be >0 after file modification")

	// 6. Clean — removes .riffle/
	out, err = run("clean", "--index", vault)
	require.NoError(t, err, "clean failed: %s", out)
	assert.Contains(t, out, "removed")
	_, statErr := os.Stat(filepath.Join(vault, ".riffle"))
	assert.True(t, os.IsNotExist(statErr), ".riffle must not exist after clean")
}
