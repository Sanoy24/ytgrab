# Repository instructions for coding agents

Read [ARCHITECTURE.md](ARCHITECTURE.md) and [PROGRESS.md](PROGRESS.md) before changing the application. Treat the architecture as the current design baseline; document a meaningful change in the architecture file before or alongside its implementation.

## Progress tracking is required

After **every iteration** of work in this repository, update [PROGRESS.md](PROGRESS.md) before finishing the iteration. An iteration is a coherent change or investigation that you would report back to the user, even when no code was changed.

- Mark each finished checklist step with `[x]` **and** strike through its text, for example `- [x] ~~Add the health route.~~`.
- Leave unfinished steps as `- [ ]`; split a partially completed step into completed and remaining steps rather than marking it done early.
- Add a dated entry to the iteration log stating what changed or was learned, what was verified, any blocker or decision, and the next step.
- If work changes the plan, edit the checklist to match the new plan and keep completed history visible.
- Do not claim a feature is complete merely because files were created; verify its behavior at the appropriate level.

## Implementation guidance

- Keep Go responsible for orchestration; `yt-dlp` owns extraction and downloading, and `ffmpeg` owns media processing.
- Pass URLs and options as `os/exec` arguments. Do not interpolate user input into a shell command.
- Keep the HTTP server bound to loopback unless a separately designed remote-access feature is requested.
- Keep active job count bounded and support cancellation and recovery of interrupted jobs.
- Avoid routing media bytes through the Go process or browser API.
- Add useful tests for state transitions, output parsing, and cancellation; run relevant checks before marking a step complete.
- Preserve user changes in the working tree and avoid unrelated edits.

## Code discovery: codebase-memory-mcp

This project uses codebase-memory-mcp to maintain a knowledge graph of the codebase. Prefer MCP graph tools over grep/glob/file search for code discovery when the graph is available and indexed.

1. `search_graph` — find functions, classes, routes, and variables by pattern.
2. `trace_path` — trace callers and callees.
3. `get_code_snippet` — read specific function or class source.
4. `query_graph` — run Cypher queries for complex patterns.
5. `get_architecture` — inspect a high-level project summary.

Use `rg` for string literals, errors, config values, non-code files, or when graph results are insufficient. If this repository has not been indexed yet, ordinary file search is appropriate; index the repository when code exists and the graph service is available.
