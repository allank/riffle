# Product Requirements Document
# Riffle — Semantic Directory Index CLI

**Version:** 0.4  
**Status:** Draft  
**Author:** Design session output  
**Changelog:** v0.4 — Closed all open questions. v0.3 — Renamed from Semantica to Riffle throughout. v0.2 — Added Obsidian vault use case; file extension filtering; relative path output; hardened exclude defaults.

---

## 1. Overview

Riffle is a single-binary CLI tool written in Go that builds and queries a semantic index of a local directory hierarchy. It allows users — and especially LLM agents — to find conceptually relevant folders within a large file tree quickly, without scanning individual files. A query like *"OAuth token refresh"* returns the three most relevant folder paths in milliseconds, consuming fewer than 50 tokens when passed to an LLM.

The primary target use case is an **Obsidian vault**: a large folder of Markdown notes organised into a hierarchy of topics. Riffle treats the vault root as the index root and operates exclusively over `.md` files by default, producing relative paths that work naturally within the vault's own addressing scheme.

The index combines two complementary strategies:

- **Structural integrity via Merkle hashing** — only re-index paths affected by changes.
- **Semantic search via vector embeddings** — find folders by concept, not just keyword.

---

## 2. Goals

| Goal | Description |
|---|---|
| LLM-first output | Default output is terse, structured, and token-efficient. Verbose human-readable output is opt-in. |
| Offline & self-contained | No external services required. Embedding model ships inside the binary. |
| Incremental | Re-indexing a changed file costs O(depth) work, not O(tree size). |
| Fast queries | Sub-100ms query response on indexes up to 50,000 folders. |
| Beautiful CLI | Human-facing output uses Charm libraries (Lip Gloss, Bubble Tea, Glamour). |
| Single binary | One file to copy, zero installation steps beyond `chmod +x`. |

### Non-Goals

- File-level retrieval (this is folder-level only by design).
- Network/cloud sync.
- Windows support in v1 (Linux and macOS only).
- Real-time file watching (watch mode is a v2 feature).

---

## 3. User Personas

### Primary: LLM Agent
An automated agent (Claude, GPT, etc.) that needs to locate relevant context before reading files. It invokes `riffle query "concept"` and receives a short machine-readable list of paths. It must not waste tokens on decorative output.

### Secondary: Obsidian / Knowledge Base User
A human managing a large personal knowledge base (Obsidian vault, Zettelkasten, wiki). They run queries interactively with `--pretty` to navigate their own notes. Relative paths are their default since vault links are relative by convention.

### Tertiary: Developer / Power User
A human who wants to find their own content rifflelly across a code or document tree. They run the same commands but may pass `--pretty` to get a richer TUI experience.

---

## 4. Commands

### 4.1 `riffle index <path>`

Builds or incrementally updates the index for the directory rooted at `<path>`.

**Behaviour:**
- Walks the directory tree using `filepath.WalkDir` with goroutine-per-subtree concurrency.
- For each directory node, computes a **folder summary** (see §6.1) and embeds it using all-MiniLM-L6-v2 via ONNX Runtime.
- Computes a **Merkle hash** for each node (see §6.2).
- On re-index, compares Merkle hashes; skips subtrees whose hash is unchanged.
- Writes the index to `<path>/.riffle/index.bin`.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--full` | false | Force full re-index, ignore existing hashes |
| `--depth <n>` | unlimited | Maximum directory depth to index |
| `--exclude <glob>` | see §9 | Comma-separated glob patterns to skip (appends to defaults) |
| `--ext <list>` | `.md` | Comma-separated file extensions to include in summaries and hashing (e.g. `.md,.txt,.org`) |
| `--concurrency <n>` | NumCPU | Goroutine count for parallel embedding |
| `--pretty` | false | Show progress bar and live stats (human mode) |

> **Extension filtering note:** Only files matching `--ext` contribute to folder summaries and Merkle hashes. Directories are still traversed regardless, but a directory containing only non-matching files will produce a sparse summary derived from filenames and path alone.

**LLM output (default):**
```
indexed path=/home/user/vault changed=14 skipped=312 ext=.md duration=2.1s
```

**Human output (`--pretty`):**
```
 Indexing ~/vault  [ext: .md]
 ████████████████████░░░░  83%  326 / 394 dirs
 Changed: 14   Skipped: 312   Elapsed: 2.1s
```

---

### 4.2 `riffle query <text>`

Queries the nearest-neighbour folder index for the most rifflelly relevant directories.

**Behaviour:**
- Embeds `<text>` using the same model.
- Runs cosine similarity against all folder vectors.
- Returns the top-K results ranked by score.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--index <path>` | Auto-discover (walks up from CWD) | Path to index root |
| `--top <n>` | 5 | Number of results to return |
| `--threshold <f>` | 0.0 | Minimum similarity score (0.0–1.0) |
| `--format <fmt>` | `plain` | Output format: `plain`, `json`, `yaml` |
| `--relative` | true | Output paths relative to the index root |
| `--pretty` | false | Human-readable scored table |

> **Relative paths:** `--relative` is `true` by default, which is the correct default for Obsidian vaults and most LLM consumption (shorter tokens, vault-portable). Pass `--relative=false` to get absolute paths.

**LLM output — plain (default, relative paths):**
```
security/oauth2
projects/auth-service/token
projects/api-gateway/middleware
```

**LLM output — JSON (`--format json`):**
```json
{
  "query": "OAuth token refresh",
  "root": "/home/user/vault",
  "relative": true,
  "results": [
    { "path": "security/oauth2", "score": 0.91 },
    { "path": "projects/auth-service/token", "score": 0.87 },
    { "path": "projects/api-gateway/middleware", "score": 0.79 }
  ]
}
```

**Human output (`--pretty`):**
```
 Query: "OAuth token refresh"   root: ~/vault

  Score  Path
  ─────  ──────────────────────────────────────────
  0.91   security/oauth2
  0.87   projects/auth-service/token
  0.79   projects/api-gateway/middleware
```

---

### 4.3 `riffle status`

Shows the health and statistics of the current index.

**LLM output (default):**
```
index=vault/.riffle/index.bin dirs=394 size=4.2MB stale=0 ext=.md relative=true model=all-MiniLM-L6-v2 built=2025-04-30T14:22:01Z
```

**Human output (`--pretty`):**
```
 Index: vault/.riffle/index.bin
 ─────────────────────────────────────
 Directories indexed   394
 Stale entries           0
 Index size           4.2 MB
 Extensions           .md
 Relative paths       yes
 Embedding model      all-MiniLM-L6-v2
 Last built           2025-04-30 14:22
```

---

### 4.4 `riffle diff`

Shows which directories are stale (file mtimes have changed since last index).

**LLM output:**
```
stale security/oauth2
stale projects/auth-service/token
stale projects/auth-service
```

**Human output (`--pretty`):** Renders a tree with stale nodes highlighted in amber. Paths are relative by default, matching `query` behaviour.

---

### 4.5 `riffle clean`

Removes the `.riffle/` index directory.

```
removed /home/user/projects/.riffle
```

---

## 5. Output Design Philosophy

### LLM Mode (default)
- No decorative characters, no colour codes, no spinner output.
- One result per line for `query`, key=value pairs for status/index.
- JSON/YAML available via `--format` for structured consumption.
- Exit codes are meaningful: `0` success, `1` no results, `2` error.

### Human Mode (`--pretty`)
- Uses **Lip Gloss** for styled, coloured terminal output.
- Uses **Bubble Tea** for progress bars and interactive spinners during indexing.
- Uses **Glamour** to render any markdown output (e.g. `riffle explain` in v2).
- Colours are theme-aware; respects `NO_COLOR` and `TERM=dumb`.

---

## 6. Core Technical Design

### 6.1 Folder Summary Construction

Each directory is summarised into a short text string at index time. This string is what gets embedded — not raw file content. Only files matching the configured `--ext` list contribute; by default this is `.md` only.

**Summary algorithm:**
1. Take the directory's name and relative path.
2. List all immediate child filenames that match the extension filter.
3. For each matching file under 50KB, extract the first 5 non-empty, non-frontmatter lines (skip YAML frontmatter blocks delimited by `---`).
4. Concatenate in this order: `{relpath}\n{filenames}\n{first_lines}`.
5. Truncate to 512 tokens (≈ 380 words) — the model's context limit.
6. If no matching files exist in the directory, the summary is the path and subdirectory names only (the directory is still indexed — it may be a meaningful container).

**Example summary for an Obsidian vault folder:**
```
security/oauth2
token-refresh.md pkce-flow.md client-credentials.md
OAuth 2.0 Token Refresh
Describes the refresh token rotation pattern used in SPAs
Related: [[PKCE Flow]], [[Token Storage]]
```

**Frontmatter handling:** Obsidian notes commonly begin with YAML frontmatter. Riffle skips the `---`-delimited block at the top of each file before extracting lines, so tags, aliases, and dates don't pollute the summary at the expense of actual content.

### 6.2 Merkle Tree Structure

Each node in the tree carries a hash. Leaf (file) hashes are derived from mtime + size (not content, for speed). **Only files matching the extension filter contribute to file hashes** — changing a `.png` attachment in a vault with `--ext .md` does not invalidate any node. Directory hashes are derived from the sorted list of qualifying child hashes.

```
DirHash(d)  = SHA256(sort(ChildHash(c) for c in children(d) if matches_ext(c)))
FileHash(f) = SHA256(f.name + f.mtime + f.size)
```

On re-index, Riffle compares the stored root hash to the recomputed root hash. If equal, nothing has changed. If different, it descends only into directories whose hash has changed, re-embedding only those — O(depth × branching_factor) work per changed file.

**This means:** editing one Markdown note in a vault of 50,000 directories causes exactly `depth` re-embeddings, typically 4–8.

### 6.3 Vector Index

- **Library:** USearch (via CGo bindings).
- **Algorithm:** HNSW (Hierarchical Navigable Small World graph).
- **Distance metric:** Cosine similarity.
- **Quantisation:** `int8` scalar quantisation — reduces memory 4× with <2% quality loss.
- **Dimensions:** 384 (all-MiniLM-L6-v2 output size).
- **Fallback:** For trees with <2,000 directories, a flat brute-force index is used instead of HNSW (simpler, faster at small scale, no graph construction overhead).

**Memory envelope estimate:**

| Dirs | HNSW int8 | Flat float32 |
|---|---|---|
| 500 | 0.2 MB | 0.8 MB |
| 5,000 | 2 MB | 8 MB |
| 50,000 | 20 MB | 80 MB |

### 6.4 Embedding Model

- **Model:** `all-MiniLM-L6-v2` (sentence-transformers).
- **Runtime:** ONNX Runtime via `github.com/yalue/onnxruntime_go` (CGo).
- **Model file:** Compiled into the binary using `go:embed` — the `.onnx` model file (~90MB) is embedded at build time.
- **Tokeniser:** BPE tokeniser implemented in pure Go (no Python dependency). Uses the model's `tokenizer.json` vocabulary, also embedded.
- **Inference:** Single-threaded per embedding call; goroutine pool for batch indexing.
- **Throughput:** ~200 embeddings/sec on a modern laptop CPU.

> **Note on binary size:** The resulting binary will be ~110MB due to the embedded ONNX model. This is intentional and acceptable — it is the price of zero external dependencies.

### 6.5 Index File Format

The index is stored at `<root>/.riffle/index.bin`. It is a single binary file with three logical sections:

```
index.bin
├── Header
│   ├── Magic:      [0x53 0x45 0x4D 0x41]  ("SEMA")
│   ├── Version:    uint16
│   ├── Flags:      uint16
│   ├── RootHash:   [32]byte
│   ├── DirCount:   uint32
│   ├── BuildTime:  int64  (unix timestamp)
│   ├── ExtCount:   uint8
│   └── ExtList:    [ExtCount]string  (e.g. [".md"])  — baked in at index time
│
├── Node Table  (DirCount entries, fixed-stride for O(1) access)
│   └── per entry:
│       ├── PathHash:   [32]byte   (SHA256 of absolute path, for lookup)
│       ├── MerkleHash: [32]byte   (structural hash for change detection)
│       ├── PathOffset: uint32     (offset into string heap)
│       ├── VectorID:   uint32     (ID in USearch index)
│       └── MTime:      int64
│
├── String Heap  (null-terminated, variable-length path strings)
│
└── USearch Index Blob  (opaque bytes, managed by USearch serialisation API)
```

- The file is `mmap`-able: the node table can be scanned without full deserialisation.
- String heap separation keeps the fixed-stride node table cache-friendly.
- The USearch blob is appended last so it can be replaced without rewriting the header or node table.

---

## 7. Dependency Summary

| Dependency | Purpose | Binding |
|---|---|---|
| `github.com/charmbracelet/bubbletea` | TUI progress & interactive mode | Pure Go |
| `github.com/charmbracelet/lipgloss` | Styled terminal output | Pure Go |
| `github.com/charmbracelet/glamour` | Markdown rendering | Pure Go |
| `github.com/charmbracelet/log` | Structured pretty logging | Pure Go |
| `github.com/unum-cloud/usearch` | HNSW vector index | CGo |
| `github.com/yalue/onnxruntime_go` | ONNX model inference | CGo |
| `github.com/spf13/cobra` | CLI command structure | Pure Go |

All other functionality (Merkle hashing, file walking, binary serialisation, tokenisation) is implemented in the Riffle codebase with no additional dependencies.

---

## 8. Build & Distribution

### Build Requirements
- Go 1.22+
- C compiler (for CGo — usearch + onnxruntime)
- ONNX Runtime shared library (linked statically in release builds)

### Build Targets
```
make build-linux-amd64
make build-linux-arm64
make build-darwin-arm64   # Apple Silicon
make build-darwin-amd64
```

### Distribution
- GitHub Releases: pre-built binaries per platform, `~110MB` each.
- Homebrew tap (v1.1): `brew install <tap>/riffle`.
- No Docker image — the single binary is the distribution unit.

---

## 9. Configuration

Riffle is configured via a TOML file at `~/.config/riffle/config.toml`. All settings can be overridden by flags. Settings baked into the index file (ext list) take precedence at query time unless explicitly overridden.

```toml
[defaults]
top = 5
format = "plain"    # or "json", "yaml"
pretty = false      # set true to always use human output
relative = true     # output relative paths by default

[index]
# Hard-coded excludes (always applied, cannot be removed via flag, only extended):
#   .git, node_modules, .riffle, .obsidian
# The --exclude flag appends to this list.
exclude = ["__pycache__", ".DS_Store", ".trash"]
ext = [".md"]       # file extensions to include; change to e.g. [".md",".txt"] for broader coverage
depth = 0           # 0 = unlimited
concurrency = 0     # 0 = NumCPU

[model]
# Override model path to use a custom ONNX model (advanced)
# onnx_path = "/path/to/custom.onnx"
```

### Hard-coded vs configurable excludes

Some directories are **always excluded** regardless of flags or config, because including them would pollute every index:

| Directory | Reason |
|---|---|
| `.git` | Version control internals — never content |
| `node_modules` | Dependency trees — enormous, never user content |
| `.riffle` | The index itself |
| `.obsidian` | Obsidian application config — not notes |

Additional excludes from config and `--exclude` are appended to this list.

---

## 10. Scope / Phasing

### v1.0 — Core
- `index`, `query`, `status`, `clean` commands.
- Merkle incremental updates.
- USearch HNSW vector index with int8 quantisation.
- all-MiniLM-L6-v2 embedded via ONNX.
- Plain, JSON, YAML output formats.
- Charm-based `--pretty` mode.

### v1.1
- `diff` command.
- `--exclude` glob support.
- Homebrew distribution.

### v2.0 (future)
- `watch` mode — daemon that re-indexes on `inotify`/`FSEvents` changes in real time.
- `explain <path>` — renders the folder summary that was embedded (for debugging).
- Term/trigram inverted index alongside the vector index (hybrid retrieval).
- Multi-root indexes (query across several trees simultaneously).

---

## 11. Decisions

All questions resolved.

| # | Question | Resolution |
|---|---|---|
| 1 | Should `.riffle/` be hidden from the index itself? | Always hard-excluded. |
| 2 | How to handle symlinks? | Follow with cycle detection (track visited inodes). |
| 3 | Should the ONNX model be side-loaded instead of embedded? | Embedded by default; `RIFFLE_MODEL_PATH` env var to override. |
| 4 | Should re-indexing be atomic? | Yes — `index.bin.tmp` + `os.Rename`. |
| 5 | Token budget for folder summary: 512 tokens enough? | Yes. Configurable in v1.1. |
| 6 | What if `--ext` is changed between index runs? | At query time, warn and proceed: `warn ext=.txt not in index (built with ext=.md)`. Do not error or block results. |
| 7 | Should `.obsidian/` be hard-excluded? | Yes — hard-excluded alongside `.git`. |
| 8 | Should Obsidian Wikilinks (`[[...]]`) be resolved to boost summary quality? | Deferred to v2 as an Obsidian-mode enhancement. |
