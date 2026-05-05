//go:build darwin

package embedder

import "os"

func onnxLibPath() string {
	if p := onnxLibEnv(); p != "" {
		return p
	}
	// Homebrew on Apple Silicon installs to /opt/homebrew; Intel Macs use /usr/local.
	for _, p := range []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/opt/homebrew/lib/libonnxruntime.dylib"
}
