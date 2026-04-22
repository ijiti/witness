# witness — Reference

Claude Code session log viewer and live agent monitor. Reads JSONL session files from `~/.claude/` and serves a web UI.

## Build & Run

```bash
go build -o witness ./cmd/witness/
./witness
```

Visit http://127.0.0.1:7070

## Environment Variables

- `WITNESS_ADDR` — listen address (default: `127.0.0.1:7070`, loopback only)
- `WITNESS_CLAUDE_DIR` — Claude projects directory (overrides the default lookup)
- `CLAUDE_CONFIG_DIR` — if set (and `WITNESS_CLAUDE_DIR` is unset), witness uses `$CLAUDE_CONFIG_DIR/projects`. Same env var Claude Code itself honors.

Default projects directory when neither override is set:

| Platform | Path |
|---|---|
| Linux   | `/home/<user>/.claude/projects` |
| macOS   | `/Users/<user>/.claude/projects` |
| Windows | `C:\Users\<user>\.claude\projects` |

(Windows note: Go's `os.UserHomeDir()` returns `%USERPROFILE%`, matching Claude Code's own resolution via Node's `os.homedir()`.)

## Architecture

- `internal/claudelog/` — JSONL wire types, parser, session builder, cost calculation, history/agent enrichment
- `internal/discovery/` — filesystem walker, session cache, inotify watcher, SSE broadcaster
- `internal/render/` — syntax highlighting (regex tokenizer), unified diff (LCS algorithm)
- `internal/web/` — chi HTTP server, embedded templates + static assets, handlers
- `internal/web/handlers/` — HTTP handlers: index, sessions, SSE streaming
- `internal/web/static/` — vendored htmx.min.js, tailwind.css, live.js (SSE client)

## Session JSONL Format

8 record types: `user`, `assistant`, `system`, `progress`, `file-history-snapshot`, `queue-operation`, `agent-setting`, `custom-title`.

Key parsing challenges:
- `user.message.content` is polymorphic: string (human turn) OR `[]tool_result` (tool return)
- Each streaming `assistant` record has exactly 1 content block — merge collects all blocks across records sharing the same `message.id`
- `toolUseResult` on user records has no type discriminator — use field-presence probing

## Data Sources in `~/.claude/`

| Path | Format | What it provides |
|---|---|---|
| `projects/<id>/*.jsonl` | JSONL | Session conversation records (primary source) |
| `projects/<id>/*/subagents/agent-*.jsonl` | JSONL | Subagent session records |
| `history.jsonl` | JSONL | Prompt index: display text, timestamp, project, sessionId |
| `stats-cache.json` | JSON | Daily activity stats: messages, sessions, tool calls, tokens per model |
| `agents/*.md` | Markdown | Agent persona definitions (system prompts) |
| `plans/*.md` | Markdown | Plan mode documents with implementation designs |
| `audit/*.jsonl` | JSONL | Tool call audit log: tool, args, session, result (per day) |

## Feature Roadmap

### Session 2: Rich rendering + data enrichment (PR #26, merged)
- history.jsonl integration for session list titles
- Syntax-highlighted code in Read results (regex tokenizer, 12 languages)
- Unified diff rendering for Edit operations (LCS algorithm)
- Structured Bash output (command header + output)
- Token cost display per turn and session totals
- Agent persona badges from `agents/*.md`

### Session 3: Live monitoring (PR #27, merged)
- inotify file watcher (raw syscall, no fsnotify dep)
- SSE endpoints for real-time session and project updates
- Event broadcaster for fan-out to multiple SSE clients
- Live auto-updating session view (new turns append, header refreshes)
- Session list auto-refresh on any project change
- External live.js with reconnect logic and connection indicator

### Session 4: Subagents, search, and audit (PR #28)
- Three-way subagent linkage: Task tool_use → progress agentId → subagent JSONL file
- Subagent tree panel with type badges, cost/duration rollup, click-to-navigate
- Subagent session viewer with back-to-parent navigation
- Cyan agent badges on Task tool calls linking to subagent view
- Cross-session search via sidebar (full-text over history.jsonl, 1400 entries)
- Audit log overlay: standard tool calls, canary detections (red), content sanitizer (orange)
- Plan file linkage: orange "plan" badge in session header, plan content viewer

### Session 5: Dashboard and polish (PR #34, merged)
- Dashboard page using `stats-cache.json` (daily activity SVG chart, model usage bars, hour-of-day heatmap, summary cards)
- Compaction markers in session view (orange "context compacted" dividers)
- Lazy-load turns via HTMX `hx-trigger="revealed"` (first 30 inline, rest on demand)
- Response caching with ETag based on file mtime (304 on dashboard + session detail)
- Unit tests for cost, stats, session, and discovery (37 tests across 4 files)

### Session 6: Hardening, tests, and competitive analysis (PR #TBD)
- **ParseToolInput hardening**: Generic fallback with logging on JSON unmarshal failures. Uses Go generics (`unmarshalOrFallback[T]`) — on parse error, logs warning and returns `ToolInputGeneric` so templates degrade gracefully.
- **Search pagination**: `SearchHistory` accepts offset/limit, returns total count. Handler passes pagination params. Template shows "Showing X-Y of Z results" with HTMX "Load more" button. Removes hard-coded 50-result cap.
- **Audit date parsing guard**: Error checks on `time.Parse` calls in `LoadAuditForSession`.
- **Test coverage expansion**: ~90+ tests (up from 37). New test files for parser, audit, history/search, render (highlight + diff), and HTTP handlers. All stdlib, table-driven.

### Session 7: Multi-session comparison
- **Compare page** (`/compare`): side-by-side session comparison with HTMX session picker
- **Metric cards**: turns, cost, input/output tokens, duration — with delta color coding
- **Tool usage bars**: paired horizontal bar chart comparing tool frequency across sessions
- **Turn timelines**: compact scrollable turn lists for each session with turn index, duration, tool count, cost
- **Session picker**: project dropdown triggers HTMX to load sessions, form submits to `/compare?a=proj/sess&b=proj/sess`
- **Title enrichment**: `Discoverer.EnrichTitle()` method to populate session titles from history.jsonl
- New files: `handlers/compare.go`, `templates/pages/compare.html`

## Competitive Analysis: witness vs claude-devtools

[claude-devtools](https://github.com/matt1398/claude-devtools) is a third-party Electron desktop app that reads the same `~/.claude/` session files.

### witness advantages
- **Audit trail transparency** — canary detection overlays (red), content sanitizer events (orange), standard tool call timeline
- **Plan integration** — session-to-plan file linking via slug matching, orange "plan" badge, plan content viewer
- **Dashboard analytics** — daily activity SVG chart, model usage breakdown bars, hour-of-day heatmap, summary cards from stats-cache.json
- **Lazy-loading** — HTMX `hx-trigger="revealed"` for large sessions (first 30 inline, rest on demand)
- **ETag caching** — 304 responses on dashboard and session detail
- **Zero-dependency Go binary** — ~10MB, compiles in 3s, no npm/Electron supply chain (vs Electron ~200MB+, React, Fastify, 50+ npm deps)
- **inotify raw syscall** — direct kernel file watching, no fsnotify/polling abstraction layer

### claude-devtools advantages
- **Context window fill visualization** — 7-category token attribution (CLAUDE.md, skills, @-mentions, tool I/O, thinking, team, prompts)
- **SSH remote inspection** — connect over SSH to view sessions on remote machines
- **Custom regex alerts** — user-defined patterns for sensitive file access, errors, token thresholds
- **Desktop app** — native OS integration, menu bar, keyboard shortcuts (Cmd+K command palette)
- **Team coordination UI** — teammate messages and team lifecycle events visualization

### Future Roadmap (inspired by comparison)
- Context window visualization (show token breakdown by category at each turn)
- Custom alert rules (regex pattern matching on tool calls or output)
