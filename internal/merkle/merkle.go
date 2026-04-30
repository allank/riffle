package merkle

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// FileHash derives a hash from name, mtime and size — not file content (fast, mtime-based).
func FileHash(name string, mtime time.Time, size int64) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", name, mtime.UnixNano(), size)))
}

// DirHash derives a directory hash from sorted child hashes.
func DirHash(children [][32]byte) [32]byte {
	sorted := make([][32]byte, len(children))
	copy(sorted, children)
	sort.Slice(sorted, func(i, j int) bool {
		for k := 0; k < 32; k++ {
			if sorted[i][k] != sorted[j][k] {
				return sorted[i][k] < sorted[j][k]
			}
		}
		return false
	})
	h := sha256.New()
	for _, c := range sorted {
		h.Write(c[:])
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
