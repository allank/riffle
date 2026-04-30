package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/allank/riffle/internal/output"
	"github.com/stretchr/testify/assert"
)

func TestPlainQuery(t *testing.T) {
	results := []output.QueryResult{
		{Path: "security/oauth2", Score: 0.91},
		{Path: "projects/auth", Score: 0.87},
	}
	var buf bytes.Buffer
	output.WritePlainQuery(&buf, results)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Equal(t, "security/oauth2", lines[0])
	assert.Equal(t, "projects/auth", lines[1])
}

func TestJSONQuery(t *testing.T) {
	results := []output.QueryResult{
		{Path: "security/oauth2", Score: 0.91},
	}
	var buf bytes.Buffer
	output.WriteJSONQuery(&buf, "OAuth", "/vault", true, results)
	assert.Contains(t, buf.String(), `"query"`)
	assert.Contains(t, buf.String(), `"security/oauth2"`)
	assert.Contains(t, buf.String(), `0.91`)
}

func TestIndexLLM(t *testing.T) {
	var buf bytes.Buffer
	output.WriteIndexLLM(&buf, "/vault", 14, 312, []string{".md"}, 2.1)
	out := buf.String()
	assert.Contains(t, out, "path=/vault")
	assert.Contains(t, out, "changed=14")
	assert.Contains(t, out, "skipped=312")
	assert.Contains(t, out, "ext=.md")
}
