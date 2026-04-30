package embedder

import "os"

func onnxLibEnv() string {
	return os.Getenv("RIFFLE_ONNX_LIB")
}
