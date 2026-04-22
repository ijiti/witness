# witness

A local web viewer for your Claude Code conversation history. Reads the JSONL session files Claude Code writes to `~/.claude/` and serves a browsable UI on `http://127.0.0.1:7070`.

Everything runs on your machine — no network calls, no telemetry, no upload of session content. The binary listens on loopback only by default, so the UI is reachable from your machine alone unless you opt in to a different bind address. Just a single static binary that points at a folder you already have.

## What you get

- Session list with titles drawn from your prompt history
- Per-turn rendering: syntax-highlighted code, unified diffs for edits, structured Bash output, agent badges
- Subagent tree per session (Task tool calls → child sessions, with cost/duration rollup)
- Live updates as new turns arrive (Linux only — see "Cross-platform notes" below)
- Cross-session full-text search
- Dashboard: daily activity chart, model usage, hour-of-day heatmap
- Side-by-side session comparison
- Audit log overlay (canary detections, content sanitizer events, tool call timeline) when present

## Install

### From source

Requires Go 1.22+.

```bash
git clone https://github.com/ijiti/witness
cd witness
go build -o witness ./cmd/witness/
./witness
```

Then open http://127.0.0.1:7070.

### Pre-built binary

Once a release is cut, binaries for Linux, macOS, and Windows are attached to each [GitHub release](https://github.com/ijiti/witness/releases). Download the one for your platform, `chmod +x` it on Linux/macOS, and run:

```bash
chmod +x witness-darwin-arm64
./witness-darwin-arm64
```

## Configuration

Three environment variables, all optional:

| Variable | Default | Purpose |
|---|---|---|
| `WITNESS_ADDR` | `127.0.0.1:7070` | listen address (loopback only by default) |
| `WITNESS_CLAUDE_DIR` | see below | Claude Code projects directory (overrides the lookup below) |
| `CLAUDE_CONFIG_DIR` | unset | If set, witness uses `$CLAUDE_CONFIG_DIR/projects` — same env var Claude Code itself honors |

When `WITNESS_CLAUDE_DIR` is unset, the projects directory is resolved as:

| Platform | Default |
|---|---|
| Linux   | `/home/<user>/.claude/projects` |
| macOS   | `/Users/<user>/.claude/projects` |
| Windows | `C:\Users\<user>\.claude\projects` |

Example — bind to all interfaces on a different port:

```bash
WITNESS_ADDR=:8080 WITNESS_CLAUDE_DIR=/path/to/.claude/projects ./witness
```

## What it reads

Witness only reads the files Claude Code already writes locally. Nothing is modified, nothing is uploaded.

| Path | Used for |
|---|---|
| `~/.claude/projects/<id>/*.jsonl` | session conversations |
| `~/.claude/projects/<id>/**/subagents/agent-*.jsonl` | subagent sessions |
| `~/.claude/history.jsonl` | session titles, search index |
| `~/.claude/stats-cache.json` | dashboard activity charts |
| `~/.claude/agents/*.md` | agent persona definitions |
| `~/.claude/plans/*.md` | plan-mode documents |
| `~/.claude/audit/*.jsonl` | audit overlay (optional) |

## Cross-platform notes

The static viewer (browse history, search, render) works on Linux, macOS, and Windows.

**Live updates** (sessions appear as you use Claude Code) currently only work on Linux — the watcher uses inotify directly. macOS and Windows fall back to a no-op watcher; refresh the page to see new sessions. A kqueue + ReadDirectoryChangesW implementation would lift this; PRs welcome.

## License

MIT — see [LICENSE](LICENSE). Third-party components (htmx, Tailwind CSS, chi) are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
