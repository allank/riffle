BINARY  := riffle
GO      := go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/allank/riffle/cmd.Version=$(VERSION)
GOFLAGS := -trimpath -ldflags "$(LDFLAGS)"

.PHONY: build test fetch-model clean clean-bin build-release build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64 tag

build: fetch-model
	$(GO) build $(GOFLAGS) -o $(BINARY) .

test:
	$(GO) test ./...

# Downloads all-MiniLM-L6-v2 ONNX model + tokenizer from HuggingFace.
# tokenizer.json is placed in both locations:
#   internal/embedder/model/  — required by the //go:embed directive in embedded.go
#   internal/tokenizer/data/  — required by the tokenizer package tests
fetch-model:
	@if [ ! -f internal/embedder/model/model.onnx ] || [ ! -f internal/embedder/model/tokenizer.json ]; then \
		echo "Downloading ONNX model (~90MB)..."; \
		mkdir -p internal/embedder/model internal/tokenizer/data; \
		curl -fsSL "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx" \
			-o internal/embedder/model/model.onnx; \
		curl -fsSL "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json" \
			-o internal/embedder/model/tokenizer.json; \
		cp internal/embedder/model/tokenizer.json internal/tokenizer/data/tokenizer.json; \
	fi

clean:
	rm -f $(BINARY) $(BINARY)-*

clean-bin:
	rm -f $(BINARY)

build-release: fetch-model
	$(GO) build $(GOFLAGS) -tags embedmodel -o $(BINARY) .

build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY)-darwin-arm64 .

build-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY)-darwin-amd64 .

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY)-linux-amd64 .

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY)-linux-arm64 .

# Create an annotated git tag: make tag VERSION=v1.0.0
tag:
	git tag -a $(VERSION) -m "Release $(VERSION)"
