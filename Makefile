BINARY := riffle
GO := go
GOFLAGS := -trimpath

.PHONY: build test fetch-model clean-bin

build: fetch-model
	$(GO) build $(GOFLAGS) -o $(BINARY) .

test:
	$(GO) test ./...

# Downloads all-MiniLM-L6-v2 ONNX model + tokenizer from HuggingFace
fetch-model:
	@if [ ! -f internal/embedder/model/model.onnx ]; then \
		echo "Downloading ONNX model (~90MB)..."; \
		mkdir -p internal/embedder/model internal/tokenizer/data; \
		curl -fsSL "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx" \
			-o internal/embedder/model/model.onnx; \
		curl -fsSL "https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/tokenizer.json" \
			-o internal/tokenizer/data/tokenizer.json; \
	fi

clean-bin:
	rm -f $(BINARY)

build-release:
	$(GO) build $(GOFLAGS) -tags embedmodel -o $(BINARY) .

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY)-darwin-arm64 .

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY)-darwin-amd64 .

build-linux-amd64:
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY)-linux-amd64 .

build-linux-arm64:
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY)-linux-arm64 .
