# Riffle

A single-binary CLI that builds and queries a semantic index of a local directory hierarchy — primarily aimed at Obsidian vaults and other large Markdown knowledge bases.

---

## The Hypothesis

If you use an LLM agent to explore a large wiki or Obsidian vault, you have a token problem.

The naive approach is to hand the agent a file tree and let it figure out where to look. On a vault of any real size — a few hundred folders, a few thousand notes — that file listing alone costs hundreds of tokens before the agent has read a single word of actual content. The agent then has to reason about paths it has never seen, make guesses, open wrong files, backtrack. Each round trip burns context.

The underlying issue is that **the agent is doing navigation work it shouldn't have to do**. The directory structure is stable, the semantic content of folders changes infrequently, and the cost of navigating it is paid over and over on every conversation.

Riffle's hypothesis is: **pre-compute the navigation layer offline, keep it cheap to maintain, and let agents skip straight to the relevant part of the tree.**

Instead of asking "where might information about OAuth token refresh live?", an agent asks Riffle — receives `security/oauth2` and `projects/auth-service/token` in under 50 tokens — and reads from there. The semantic work was already done at index time, amortised across every future query.

### Why Folders, Not Files?

File-level retrieval is well-covered ground (RAG pipelines, embeddings databases, etc.). The folder level is the navigation layer — it tells an agent *where to look*, not *what the answer is*. A folder path is 5–15 tokens. A file path is similar. But returning 3–5 folder paths gives the agent a targeted region of the knowledge base to read in full, which is often more useful than returning 10 file snippets out of context.

Folder-level indexing also degrades gracefully. A folder with no Markdown files still gets indexed based on its path and subdirectory names — it remains findable as a container, even if its content isn't described.

### Why Merkle Hashing?

A vault changes incrementally — you update a few notes, add a new file. Re-embedding the entire tree on every change would make the tool unusable for large vaults. Riffle uses a Merkle tree where each directory's hash is derived from its children's hashes (filtered to the relevant extensions). This means:

- A single file change propagates upward only along its ancestor path.
- Re-indexing after one note change costs exactly `depth` re-embeddings — typically 4–8 on a well-organised vault — regardless of how large the vault is.
- Changing a `.png` attachment in a `.md`-only index changes nothing.

### Why an Embedded Offline Model?

Sending folder summaries to an external embedding API at index time creates a dependency, a cost, and a privacy concern. Riffle embeds `all-MiniLM-L6-v2` — a 90MB sentence-transformers model — directly into the binary at build time. The model runs locally via ONNX Runtime. No network call, no API key, no data leaving the machine. The binary is large (~110MB) but it is the complete, self-contained unit.

### The Output Design Decision

The default output mode is deliberately terse and machine-readable:

```
security/oauth2
projects/auth-service/token
projects/api-gateway/middleware
```

Three paths. No scores, no decorations, no colour codes. An agent can paste this directly into context or use it as input to a file-reading step. Human-readable output — scored tables, progress bars, styled status panels — is opt-in via `--pretty`. The tool is designed for the machine first and the human second.

---

> [!WARNING]
> ## This is a vibe coded experiment
>
> This project was designed and built almost entirely through conversational AI — prompting, reviewing, and iterating with Claude rather than writing code by hand. It is being **dog-fooded**: I am currently running Riffle against my own Obsidian vault to see how well the hypothesis holds in practice.
>
> **No claims are being made about the effectiveness of this approach.** Whether semantic folder indexing meaningfully reduces token usage, improves agent navigation quality, or is worth the overhead of maintaining an index is an open question I am trying to answer through use.
>
> **No claims are being made that a better solution doesn't already exist.** It probably does. This is an experiment of my own choosing, built to satisfy my own curiosity about the problem space and about what it feels like to vibe-code a non-trivial Go project from scratch.
>
> Use accordingly.

---

## Commands

### `riffle index <path>`

Builds or incrementally updates the semantic index for the directory rooted at `<path>`. The index is written to `<path>/.riffle/index.bin`.

```bash
riffle index ~/vault
```

On first run, every directory is walked, summarised, and embedded. On subsequent runs, only directories whose Merkle hash has changed are re-embedded — the rest are skipped in O(1) per node.

```
indexed path=/home/user/vault changed=14 skipped=312 ext=.md duration=2.1s
```

With `--pretty`:
```
 Indexing ~/vault  [ext: .md]
 ████████████████████░░░░  83%  326 / 394 dirs
 Changed: 14   Skipped: 312   Elapsed: 2.1s
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--full` | false | Force full re-index, ignore existing hashes |
| `--depth <n>` | unlimited | Maximum directory depth to index |
| `--ext <list>` | `.md` | Comma-separated file extensions (e.g. `.md,.txt`) |
| `--concurrency <n>` | NumCPU | Goroutine count for parallel embedding |
| `--pretty` | false | Show progress bar |

---

### `riffle watch <path>`

Starts a foreground daemon that watches `<path>` for filesystem changes and automatically re-indexes on change. Also exposes a Model Context Protocol (MCP) Streamable HTTP server so LLM agents can query the live index directly.

```bash
riffle watch ~/vault
```

On startup, loads the existing index from `<path>/.riffle/index.bin`. If no index exists, performs a full initial index first. Once running:

```
watching path=/home/user/vault listen=127.0.0.1:7424 mode=events
```

**File changes** are debounced with a 500ms quiet window before triggering a re-index. Only directories whose Merkle hash has changed are re-embedded — the same incremental logic as `riffle index`.

**MCP server** listens on `127.0.0.1:7424` (configurable) and exposes two tools:

- **`riffle_query`** — find semantically relevant directories for a natural-language query
- **`riffle_status`** — return index statistics and watcher health (including `mode: events|polling`)

**Health endpoint** — `GET /health` for liveness checks from scripts, launchd, and systemd:
```json
{ "ok": true, "watching": "/home/user/vault", "mode": "events" }
```

**Watcher modes:**

| Mode | Meaning |
|---|---|
| `events` | Kernel event subscription active (inotify / FSEvents); changes delivered in real time |
| `polling` | Subscription lost; watcher polls every 30 seconds |

If the event subscription is lost, the daemon logs a warning and falls back to polling automatically — no restart needed.

**Signals:**
- `SIGINT` / `SIGTERM` — graceful shutdown: finish any in-progress re-index, write the index, exit 0
- `SIGHUP` — force a full re-index (equivalent to `riffle index --full`)

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--listen <addr:port>` | `127.0.0.1:7424` | MCP server bind address |

**MCP tool schemas:**

`riffle_query`:
```json
{
  "q":         { "type": "string",  "description": "Natural-language search query" },
  "top":       { "type": "integer", "default": 5 },
  "threshold": { "type": "number",  "default": 0.0, "description": "Minimum similarity score (0.0–1.0)" }
}
```

`riffle_status`: no parameters — returns index stats plus current watcher `mode`.

**No daemonisation.** `riffle watch` is a foreground process. For background operation, use your OS process supervisor. Sample unit files:

<details>
<summary>macOS launchd plist</summary>

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>         <string>com.riffle.watch</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/riffle</string>
    <string>watch</string>
    <string>/Users/you/vault</string>
  </array>
  <key>RunAtLoad</key>     <true/>
  <key>KeepAlive</key>     <true/>
  <key>StandardOutPath</key> <string>/tmp/riffle.log</string>
  <key>StandardErrorPath</key> <string>/tmp/riffle.log</string>
</dict>
</plist>
```

Save to `~/Library/LaunchAgents/com.riffle.watch.plist` and run `launchctl load ~/Library/LaunchAgents/com.riffle.watch.plist`.
</details>

<details>
<summary>Linux systemd unit</summary>

```ini
[Unit]
Description=Riffle vault watcher
After=network.target

[Service]
ExecStart=/usr/local/bin/riffle watch /home/you/vault
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
```

Save to `~/.config/systemd/user/riffle-watch.service` and run `systemctl --user enable --now riffle-watch`.
</details>

---

### `riffle query <text>`

Finds the most semantically relevant directories for a natural-language query. Auto-discovers the nearest `.riffle` index by walking up from CWD.

```bash
riffle query "OAuth token refresh"
```

Default output (one path per line, relative to index root):
```
security/oauth2
projects/auth-service/token
projects/api-gateway/middleware
```

JSON output (`--format json`):
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

Pretty table (`--pretty`):
```
 Query: "OAuth token refresh"   root: ~/vault

  Score  Path
  ─────  ──────────────────────────────────────────
  0.91   security/oauth2
  0.87   projects/auth-service/token
  0.79   projects/api-gateway/middleware
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--index <path>` | Auto-discover from CWD | Path to index root |
| `--top <n>` | 5 | Number of results |
| `--threshold <f>` | 0.0 | Minimum similarity score (0.0–1.0) |
| `--format <fmt>` | `plain` | `plain`, `json`, or `yaml` |
| `--relative` | true | Relative paths (vault-portable) |
| `--pretty` | false | Human-readable scored table |

**Exit codes:** `0` success, `1` no results above threshold, `2` error.

---

### `riffle status`

Shows health and statistics for the current index.

```
index=vault/.riffle/index.bin dirs=394 size=4.2MB stale=0 ext=.md relative=true model=all-MiniLM-L6-v2 built=2025-04-30T14:22:01Z
```

With `--pretty`:
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

### `riffle clean`

Removes the `.riffle/` directory entirely.

```bash
riffle clean
# removed /home/user/vault/.riffle
```

---

## Configuration

`~/.config/riffle/config.toml` — all settings can be overridden per-invocation with flags:

```toml
[defaults]
top = 5
format = "plain"    # or "json", "yaml"
pretty = false
relative = true

[index]
exclude = ["__pycache__", ".DS_Store", ".trash"]
ext = [".md"]
depth = 0           # 0 = unlimited
concurrency = 0     # 0 = NumCPU

[watch]
listen      = "127.0.0.1:7424"   # bind address; "0.0.0.0:7424" for network access
debounce_ms = 500                 # quiet window before triggering re-index after FS events
```

The following directories are **always excluded** regardless of config or flags:

| Directory | Reason |
|---|---|
| `.git` | Version control internals |
| `node_modules` | Dependency trees |
| `.riffle` | The index itself |
| `.obsidian` | Obsidian application config |

---

## How the Index Works

### Folder Summary Construction

Each directory is distilled into a short text string before embedding. The summary is constructed from:

1. The directory's relative path
2. Filenames of immediate children matching the extension filter
3. The first 5 non-empty, non-frontmatter lines from each matching file under 50KB (YAML frontmatter blocks delimited by `---` are skipped)

The result is truncated to approximately 512 tokens before being passed to the embedding model. Directories with no matching files are still indexed — their path and subdirectory names form a sparse summary.

Example summary that gets embedded:
```
security/oauth2
token-refresh.md pkce-flow.md client-credentials.md
OAuth 2.0 Token Refresh
Describes the refresh token rotation pattern used in SPAs
Related: [[PKCE Flow]], [[Token Storage]]
```

### Merkle-Driven Incremental Updates

```
DirHash(d)  = SHA256(sort(ChildHash(c) for c in children(d) if matches_ext(c)))
FileHash(f) = SHA256(f.name + f.mtime + f.size)
```

File hashes are derived from metadata only (mtime + size), not content. This makes them fast to compute but also means Riffle trusts the filesystem's mtime — if you touch a file without changing it, Riffle will re-embed that directory's ancestors.

Only files matching the active extension filter contribute to hashes. Changing a `.png` in a `.md`-only index is a no-op.

### Vector Search

For indexes with fewer than 2,000 directories, Riffle uses a flat brute-force cosine similarity search. Above that threshold it uses HNSW (Hierarchical Navigable Small World) via USearch, which gives sub-linear query time at the cost of approximate results.

Memory footprint at scale:

| Dirs | Index size (approx) |
|---|---|
| 500 | ~0.8 MB |
| 5,000 | ~8 MB |
| 50,000 | ~80 MB |

---

## Building from Source

### Prerequisites

- Go 1.22+
- A C compiler (`gcc` or `clang`) — required for the ONNX Runtime CGo bindings
- ONNX Runtime shared library:
  - macOS: `brew install onnxruntime`
  - Linux: download from the [ONNX Runtime releases](https://github.com/microsoft/onnxruntime/releases) and place `libonnxruntime.so` somewhere on `LD_LIBRARY_PATH`

### 1. Fetch the embedding model (one-time)

```bash
make fetch-model
```

This downloads `all-MiniLM-L6-v2` (~90MB ONNX model) and its tokenizer from HuggingFace into:

```
internal/embedder/model/model.onnx
internal/tokenizer/data/tokenizer.json
```

Both paths are gitignored. They must be present before any build.

### 2. Dev build (dynamically links ONNX Runtime)

```bash
go build -o riffle .
# or
make build
```

The dev binary does **not** embed the model. You must tell it where the files are at runtime via environment variables:

```bash
export RIFFLE_MODEL_PATH=internal/embedder/model/model.onnx
export RIFFLE_TOKENIZER_PATH=internal/tokenizer/data/tokenizer.json
./riffle index ~/vault
```

### 3. Release build (model embedded in binary)

```bash
make build-release
```

This compiles with `-tags embedmodel`, which activates `//go:embed` directives for the model and tokenizer files. The result is a single self-contained binary (~110MB) that requires no environment variables and no external files at runtime.

```bash
./riffle index ~/vault   # just works, no env vars needed
```

### Versioning

The version string is injected at build time from the current git tag:

```bash
./riffle --version
# riffle v1.0.0
```

Untagged builds report the nearest tag with a commit suffix (e.g. `v1.0.0-3-gabcdef1`) or `dev` if there are no tags. To cut a release:

```bash
make tag VERSION=v1.0.0   # creates an annotated git tag
git push origin v1.0.0
make build-release
```

---

## Roadmap

### v1.1
- `riffle diff` — shows which directories are stale without re-indexing
- `--exclude` glob flag support
- Homebrew distribution tap

### v2.0
- ~~**Watch mode** — daemon that re-indexes on `inotify`/`FSEvents` file changes~~ ✓ shipped
- **`riffle explain <path>`** — renders the folder summary that was embedded for a given path, useful for debugging why a path does or doesn't appear in results
- **Hybrid retrieval** — term/trigram inverted index alongside the vector index; combines keyword precision with semantic recall
- **Multi-root indexes** — query across several vaults or directory trees simultaneously
- **Obsidian Wikilink resolution** — expand `[[Link]]` references in summaries to include the linked note's content, improving semantic coverage for heavily cross-linked vaults
- **Configurable token budget** — currently fixed at 512 tokens per folder summary

---

## Colophon

Riffle was designed and built through conversational AI — specifically through an extended session with **Claude** (Anthropic), using Claude Code as the development environment. The project went from a problem statement to a working, tested Go binary without writing code by hand in the conventional sense. The experience of doing this is part of what the project is testing.

**Language & runtime**
- [Go](https://go.dev/) 1.22+ — chosen for single-binary distribution, good concurrency primitives, and CGo interop for the two native libraries

**Embedding**
- [all-MiniLM-L6-v2](https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2) (sentence-transformers) — 384-dimensional sentence embedding model; good balance of quality, size (~90MB ONNX), and inference speed
- [ONNX Runtime](https://onnxruntime.ai/) — cross-platform ML inference engine; used via [`yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go) CGo bindings

**Vector search**
- [USearch](https://github.com/unum-cloud/usearch) — HNSW approximate nearest-neighbour index with int8 quantisation; used for indexes ≥ 2,000 directories

**Watch mode**
- [fsnotify](https://github.com/fsnotify/fsnotify) — cross-platform filesystem event notifications (inotify on Linux, FSEvents on macOS); used by `riffle watch` to trigger incremental re-indexing

**CLI & terminal UI**
- [Cobra](https://github.com/spf13/cobra) — command structure and flag parsing
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — styled terminal output (scored query tables, status panels)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework for the `--pretty` progress bar during indexing
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering (used in planned v2 `explain` command)
- [Charmbracelet Log](https://github.com/charmbracelet/log) — structured, styled logging

**Configuration & testing**
- [BurntSushi/toml](https://github.com/BurntSushi/toml) — TOML config file parsing
- [testify](https://github.com/stretchr/testify) — test assertions and requirements

**Development tooling**
- [Claude Code](https://claude.ai/code) — the AI coding environment in which this project was built
- [Claude Sonnet 4.6](https://www.anthropic.com/) — the model that wrote the code
