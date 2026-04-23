# witness — agent operating notes

You are working in the standalone `witness` repo. This is a local web viewer for Claude Code session history. The user runs the binary on their own machine; it reads JSONL files Claude Code writes to `~/.claude/` and serves a browsable UI.

Read this file before doing any work in this repo. The rules below are short — follow them exactly.

## What this is

Single static Go binary, ~10 MB, no CGO, no runtime dependencies, no network calls. It walks `~/.claude/projects/`, parses session JSONL records, and serves an HTMX/Tailwind frontend on `127.0.0.1:7070` (loopback only by default). That's it. Resist the urge to add features that require shipping more than the one binary.

## Quick start

```bash
# Run the dev binary
go build -o witness ./cmd/witness/
./witness                           # http://127.0.0.1:7070

# Bind to all interfaces, or pick a different port
WITNESS_ADDR=:7070 ./witness        # all interfaces, port 7070
WITNESS_ADDR=127.0.0.1:7072 ./witness

# Point at a non-default Claude dir (rare). If CLAUDE_CONFIG_DIR is set, witness
# honors it the same way Claude Code itself does.
WITNESS_CLAUDE_DIR=/some/other/.claude/projects ./witness
CLAUDE_CONFIG_DIR=/some/other/.claude ./witness

# Tests
go test ./...

# Cross-compile (CGO_ENABLED=0 is mandatory; the binary must stay static)
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags='-s -w' -o dist/witness-darwin-arm64 ./cmd/witness/
```

The binary's first log line tells you the listen address. It serves immediately — no migrations, no config file.

## Repo layout

```
cmd/witness/main.go            entrypoint: parses env, wires Discoverer + http server
internal/claudelog/            JSONL parser and session/subagent/cost/history/audit assembly
internal/discovery/            filesystem walker, session cache, inotify watcher (Linux), SSE broadcaster
internal/render/               syntax highlighter (regex tokenizer, ~12 languages) and unified diff (LCS)
internal/web/                  chi HTTP server, embedded templates and static assets, route registration
internal/web/handlers/         per-page handlers (index, sessions, dashboard, compare, search, SSE)
internal/format/               byte/duration/string formatters (Bytes, Truncate, EnvOr)
internal/costlog/              per-model token-pricing table and cost computation
```

`REFERENCE.md` at the repo root carries the original design notes — record types, field-presence quirks, feature roadmap by session. Read it before changing parser or session-assembly code.

## Hard rules

1. **No CGO.** `CGO_ENABLED=0` must continue to produce a working binary on every platform. If you reach for a sqlite driver or any C-linked library, stop and find another way.
2. **No telemetry, no outbound network, ever.** This tool's appeal is that it never phones home. Don't add analytics, error reporting, update checks, or "anonymous usage stats." Don't fetch anything from the public internet at runtime.
3. **No write access to `~/.claude/`.** Witness is read-only. Never modify, rename, delete, or create files under the Claude data directory. If you find yourself needing to write state, put it under XDG_STATE_HOME or alongside the binary.
4. **No new runtime deps without a strong reason.** Current dep tree is one package: `github.com/go-chi/chi/v5`. The frontend is vendored htmx + Tailwind compiled CSS. Adding a dep means writing the case in the PR description.
5. **Keep the static viewer cross-platform.** All Linux-specific syscalls live behind `//go:build linux` with a stub in `watcher_other.go`. `go build` for darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64 must all succeed before you commit.
6. **No paths or assumptions specific to any one user or org.** No hardcoded `/home/...`, no references to a specific monorepo, no embedded API keys. The only thing this binary "knows" about the host is the value of `$HOME` (and even that is overridable via `WITNESS_CLAUDE_DIR`).

## How Claude Code writes the data we read

Critical context for parser work:

- Each session is a JSONL file under `~/.claude/projects/<flattened-cwd>/<session-id>.jsonl`. Project ID is the cwd with `/` replaced by `-`, e.g. `-home-alice-myproject`.
- Eight record types: `user`, `assistant`, `system`, `progress`, `file-history-snapshot`, `queue-operation`, `agent-setting`, `custom-title`. New types appear as Claude Code evolves; the parser must degrade gracefully (unknown types are skipped, not fatal).
- `assistant` records are streamed: each record carries one content block, and you merge by `message.id` to assemble a complete turn.
- `user.message.content` is polymorphic: a string for human turns, an array of `tool_result` objects for tool returns. Don't assume one shape.
- `toolUseResult` has no type discriminator — the parser does field-presence probing in `internal/claudelog/parser.go`.
- Subagent sessions live at `~/.claude/projects/<id>/<session-id>/subagents/agent-<task-id>.jsonl`; their parent session references them via the `Task` tool's `tool_use_id`, and the subagent's `progress` records carry a matching `agentId`. Three-way linkage is in `internal/claudelog/subagent.go`.

When Claude Code adds a new record type or field, **the failure mode must be "render what we understand, ignore the rest"** — never crash, never drop the whole session.

## Cross-platform watcher

`internal/discovery/watcher.go` uses raw inotify syscalls and is `//go:build linux`. `watcher_other.go` is the `!linux` stub: it satisfies the same exported API (`NewWatcher`, `Events`, `Start`, `Stop`) but never emits events, so SSE clients on macOS/Windows just don't get live updates.

If you're asked to add live updates on macOS or Windows, write platform-specific files alongside `watcher.go`:

- `watcher_darwin.go` (`//go:build darwin`) — kqueue (`golang.org/x/sys/unix`) or `FSEvents` via cgo. Prefer kqueue — keeps the no-CGO rule.
- `watcher_windows.go` (`//go:build windows`) — `ReadDirectoryChangesW` via `golang.org/x/sys/windows`.

Then narrow `watcher_other.go`'s build tag to `!linux && !darwin && !windows`. Keep the exported API identical so `discovery.go` doesn't need to know which platform it's on.

## Frontend

HTMX + Tailwind. Templates live in `internal/web/templates/`, static assets (htmx.min.js, tailwind.css, live.js) in `internal/web/static/`. Both are embedded into the binary via `embed.FS` — no separate asset deployment.

**Tailwind CSS is precompiled and committed.** Source is `tailwind.input.css`; config is `tailwind.config.js` (content globs scan `internal/web/templates/**/*.html`). To rebuild after changing templates or adding utility classes:

```bash
scripts/build-css.sh    # uses npx tailwindcss@3.4.17, writes to internal/web/static/tailwind.css
```

Node is a **build-time only** dep. The shipped binary remains pure-Go static. If you add a new utility class to a template, run the script and commit the regenerated `tailwind.css` — the embed picks it up at next `go build`. Custom colors (e.g. the `surface-700/800/900` palette) live in `tailwind.config.js`'s `theme.extend.colors`. Add new ones there.

- Layout entry: `internal/web/templates/layouts/base.html`
- Pages: `internal/web/templates/pages/*.html`
- Partials: `internal/web/templates/partials/*.html`

Lazy-loading uses HTMX `hx-trigger="revealed"`: the first 30 turns of a session render inline, the rest fetch on demand. ETag caching is keyed on session-file mtime — when you change rendering logic, bust by stamping the build time into the ETag (see `handlers.SessionDetail`).

Live updates: `live.js` opens an SSE stream to `/events`; the broadcaster fan-outs `WatchEvent`s from the inotify watcher.

## Adding a feature — checklist

When the user asks for a new feature, walk this list before implementing:

1. **Does it require modifying `~/.claude/`?** If yes, push back. Witness is read-only.
2. **Does it require a network call at runtime?** If yes, push back. Witness is local-only.
3. **Does it require a CGO dep or a new runtime dependency?** If yes, justify in the PR description.
4. **Does it work on Linux, macOS, and Windows?** If you're using platform-specific syscalls, gate them with build tags and provide a stub or alternative.
5. **Does it bloat the binary noticeably?** Current size is ~9-10 MB. Adding 50 MB of embedded fonts is not OK.
6. **Does it need new state?** Witness has no persistent state of its own — caches are in-memory and rebuilt on startup. If you must persist, use XDG_STATE_HOME, document it, and make it deletable without losing functionality.
7. **Have you added a test?** Parser changes need fixture-driven tests (see `internal/claudelog/*_test.go`). Render changes need golden-output tests. HTTP handlers need an `httptest` round trip.

## Releasing

GitHub Actions workflow at `.github/workflows/release.yml` triggers on tag push (`v*`). It cross-compiles the five target binaries with `-ldflags='-s -w'` and uploads them to a GitHub Release.

To cut a release:

```bash
git tag v0.2.0
git push origin v0.2.0
```

Build it locally first to make sure all five targets compile:

```bash
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 \
    go build -ldflags='-s -w' -o /tmp/witness-test ./cmd/witness/ \
    && echo "ok $target" || echo "FAIL $target"
done
```

## Things that are tempting but wrong

- **"Let's add a way to edit conversations from the UI."** No. Read-only is a design constraint.
- **"Let's auto-update the binary."** No. Auto-update implies network calls and signing infrastructure neither of which we want to own.
- **"Let's port the frontend to React/Vue/Svelte."** No. HTMX + Tailwind keeps the binary self-contained and the dev loop instant.
- **"Let's switch to fsnotify for cross-platform watching."** Tempting. fsnotify pulls in a small dep tree but reduces our raw-syscall surface area. If you want to do this, propose it as an explicit tradeoff: smaller code, one new dep, looser control over event coalescing semantics.
- **"Let's make the watcher write a cache to disk so startup is faster."** Measure first. Current cold-start is fast enough for sessions in the thousands; adding a cache means cache invalidation problems.

## Asking the user

If a request is ambiguous about goal, scope, or what counts as done — ask first. Don't guess. The user keeps witness deliberately small; defaults toward "do less."
