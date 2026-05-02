# Riffle V2 — Watch Mode & MCP Server Design

**Date:** 2026-05-02  
**Status:** Approved  
**Scope:** `riffle watch` command — filesystem watcher + MCP Streamable HTTP server

---

## 1. Overview

V2 adds a single new command: `riffle watch <path>`. It starts a long-running foreground process that:

1. Watches `<path>` for filesystem changes and triggers incremental re-indexing automatically.
2. Exposes a Model Context Protocol (MCP) Streamable HTTP server so LLM agents can query the live index directly.

All existing commands (`index`, `query`, `status`, `clean`) are unchanged.

---

## 2. Architecture

Two concurrent components run in a single process:

**File watcher** — uses `inotify` (Linux) or `FSEvents` (macOS) to receive kernel filesystem events. On change, triggers an incremental re-index via the existing Merkle machinery. Events are debounced with a 500ms quiet window before a re-index is triggered, to avoid redundant work when an editor writes multiple files in quick succession.

**MCP HTTP server** — implements the MCP Streamable HTTP transport, serving two tools (`riffle_query`, `riffle_status`) and one plain HTTP endpoint (`GET /health`).

Both components share the in-memory index via a read/write mutex. Queries acquire a read lock; re-indexing acquires a write lock and swaps in the new index atomically before releasing it. The on-disk write continues to use `os.Rename` for atomicity, as today.

---

## 3. Startup & Shutdown

**Startup sequence:**
1. Load existing index from `<path>/.riffle/index.bin` if present. If not present, perform a full initial index before starting the watcher or server.
2. Start the file watcher.
3. Start the MCP HTTP server.
4. Log one line to stdout: `watching path=<path> listen=127.0.0.1:7424 mode=events`

**Signals:**
- `SIGINT` / `SIGTERM` — graceful shutdown: stop accepting new MCP requests, finish any in-progress re-index, write the index file, exit 0.
- `SIGHUP` — force a full re-index (equivalent to `riffle index --full`).

**No daemonisation.** `riffle watch` is a foreground process. Users who want background operation use their OS process supervisor. The README will include sample `launchd` (macOS) and `systemd` (Linux) unit files.

---

## 4. Watcher Mode Tracking

The file watcher tracks its current event delivery mode:

| Mode | Meaning |
|---|---|
| `events` | Kernel event subscription active; changes are delivered in real time |
| `polling` | Subscription lost (e.g. macOS FSEvents overrun); watcher polls every 30 seconds |

When the daemon falls back to polling it:
- Logs a structured warning line: `warn event_subscription_lost=true falling_back=polling interval=30s`
- Updates the mode reported by `/health` and `riffle_status`

The mode is surfaced on the MCP interfaces only — not on the `riffle status` CLI command, which has no concept of a running daemon.

---

## 5. MCP Server

### Transport

MCP Streamable HTTP. Single endpoint: `POST /mcp`, accepting JSON-RPC 2.0 messages.

Default bind address: `127.0.0.1:7424`. Configurable via `--listen` flag or `config.toml`.

When `--listen` binds to a non-loopback address the daemon logs:
```
warn listen=0.0.0.0:7424 auth=none network_accessible=true
```

### Tools

**`riffle_query`**

Find the most semantically relevant directories for a natural-language query.

Input schema:
```json
{
  "q":         { "type": "string",  "description": "Natural-language search query" },
  "top":       { "type": "integer", "default": 5 },
  "threshold": { "type": "number",  "default": 0.0, "description": "Minimum similarity score (0.0–1.0)" }
}
```

Response: JSON array of result objects.
```json
[
  { "path": "security/oauth2",              "score": 0.91 },
  { "path": "projects/auth-service/token",  "score": 0.87 }
]
```

Paths are relative to the vault root, consistent with `riffle query --relative` (the default).

---

**`riffle_status`**

Return health and statistics for the current index.

Input schema: `{}` (no parameters)

Response: JSON object with status fields plus watcher mode.
```json
{
  "index":   "vault/.riffle/index.bin",
  "dirs":    394,
  "size":    "4.2MB",
  "stale":   0,
  "ext":     [".md"],
  "model":   "all-MiniLM-L6-v2",
  "built":   "2025-04-30T14:22:01Z",
  "mode":    "events"
}
```

`mode` is `"events"` or `"polling"` — reflects the current watcher state.

### Health Endpoint

`GET /health` — plain HTTP, not MCP. Returns `200 OK` with:
```json
{ "ok": true, "watching": "/home/user/vault", "mode": "events" }
```

`mode` is `"events"` or `"polling"`. Intended for liveness checks from scripts, launchd, and systemd — callers that do not speak JSON-RPC.

---

## 6. Configuration

New `[watch]` section in `~/.config/riffle/config.toml`:

```toml
[watch]
listen      = "127.0.0.1:7424"   # bind address; "0.0.0.0:7424" for network access
debounce_ms = 500                 # quiet window before triggering re-index after FS events
```

The `--listen <addr:port>` flag overrides the config value.

Indexing settings (`ext`, `depth`, `exclude`, `concurrency`) are inherited from the `[index]` section already baked into the index file at build time — they are not re-specified on `riffle watch`.

---

## 7. Error Handling

| Scenario | Behaviour |
|---|---|
| Index file missing on startup | Perform full initial index before starting watcher and server |
| Re-index fails (e.g. I/O error) | Log error, retain previous in-memory index, continue watching |
| MCP request arrives during re-index | Served from previous index under read lock; never blocked |
| Watcher event subscription lost | Fall back to 30s polling; log warning; update mode in `/health` and `riffle_status` |
| `--listen` address already in use | Exit 1 with clear error message |

---

## 8. Testing

The existing integration test covers the full index/query/status/clean cycle and is unchanged.

Watch mode testing uses a temporary directory:
1. Start `riffle watch <tempdir>` as a subprocess.
2. Poll `GET /health` until `{"ok": true}`.
3. Trigger file writes under `<tempdir>`.
4. Assert via `POST /mcp` (`riffle_query`) that results update within a 3-second timeout.
5. Assert `riffle_status` `mode` field reflects the active watcher mode.
6. Send `SIGTERM`; assert clean exit.

---

## 9. Out of Scope for V2

- Authentication / TLS (no-auth warning logged when non-loopback bind is used).
- Multiple vault support (single vault per process).
- Push notifications to MCP clients when the index updates (pull-only).
- `riffle explain`, hybrid retrieval, multi-root indexes, Wikilink resolution (see PRD §10 for future phases).
