package tokenizer_test

import (
	"testing"

	"github.com/allank/riffle/internal/tokenizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustLoad(t *testing.T) *tokenizer.Tokenizer {
	t.Helper()
	tok, err := tokenizer.LoadFile("data/tokenizer.json")
	require.NoError(t, err)
	return tok
}

func TestEncodeBasic(t *testing.T) {
	tok := mustLoad(t)
	out := tok.Encode("hello world", 512)
	// [CLS]=101, tokens, [SEP]=102
	assert.Equal(t, int64(101), out.InputIDs[0])
	assert.Equal(t, int64(102), out.InputIDs[len(out.InputIDs)-1])
	assert.Len(t, out.AttentionMask, len(out.InputIDs))
	for _, m := range out.AttentionMask {
		assert.Equal(t, int64(1), m)
	}
	assert.Len(t, out.TokenTypeIDs, len(out.InputIDs))
}

func TestEncodeTruncation(t *testing.T) {
	tok := mustLoad(t)
	// Generate a long string
	long := ""
	for i := 0; i < 200; i++ {
		long += "hello world "
	}
	out := tok.Encode(long, 32)
	assert.Equal(t, 32, len(out.InputIDs))
	assert.Equal(t, int64(101), out.InputIDs[0])
	assert.Equal(t, int64(102), out.InputIDs[31])
}

func TestEncodeLowercase(t *testing.T) {
	tok := mustLoad(t)
	a := tok.Encode("Hello", 512)
	b := tok.Encode("hello", 512)
	assert.Equal(t, a.InputIDs, b.InputIDs)
}
