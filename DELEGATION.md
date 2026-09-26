# Codex and Claude collaboration

Use separate Git branches and worktrees for parallel work. Both agents read `ARCHITECTURE.md`, `AGENTS.md`, and `PROGRESS.md`; Claude also reads `CLAUDE.md`. Each agent updates its branch's `PROGRESS.md` after every iteration. The integrator reconciles both progress logs and checklist changes when merging.

## First parallel assignment

| Agent | Branch | Owns | Completion evidence |
| --- | --- | --- | --- |
| Codex | `agent/codex-foundation` | `go.mod`, `cmd/`, `internal/app/`, `internal/config/`, and the health/diagnostics portion of `internal/api/` | `go test ./...` and a local health request; missing tools produce useful diagnostics |
| Claude | `agent/claude-ui-shell` | `web/` only | A usable static page with URL input, preset choice, queue/history display, empty/error states, and responsive layout; record how it was previewed |

Codex owns backend startup and the initial HTTP contract. Claude may use fixtures while building the UI, but should not claim server integration or live downloads are complete. Neither agent should edit the other agent's owned code paths. If an interface must change, document the proposal in its branch and leave the other agent's files alone until integration.

The shared documentation files are exceptions to path ownership. Each agent may update `PROGRESS.md` as required, with a dated log heading that includes its agent name. Keep changes to `ARCHITECTURE.md`, `AGENTS.md`, and `CLAUDE.md` limited to decisions required by the assigned task. The integrator resolves any documentation merge conflicts and retains both agents' log entries.

## Prepare local worktrees (PowerShell)

After committing this coordination documentation on `master`, run these from the repository root:

```powershell
git worktree add .worktrees/codex -b agent/codex-foundation master
git worktree add .worktrees/claude -b agent/claude-ui-shell master
```

Open `.worktrees/codex` in Codex and `.worktrees/claude` in Claude Code. Keep the agents in their own directories. Both `codex` and `claude` commands are available on the current machine, so a terminal in each directory is also an option. Confirm the active directory and branch before issuing each task.

For terminal sessions, start each command from its own worktree:

```powershell
# Terminal 1
Set-Location .worktrees/codex
codex

# Terminal 2, started from the repository root
Set-Location .worktrees/claude
claude
```

### Prompt for Codex

```text
Read AGENTS.md, ARCHITECTURE.md, DELEGATION.md, and PROGRESS.md. Work only on the Codex assignment in DELEGATION.md on branch agent/codex-foundation. Build the Go application foundation: module, configuration, graceful startup/shutdown, loopback HTTP server, and health/dependency diagnostics. Keep the scope to your owned paths. Verify with go test ./... and a local health request. Update PROGRESS.md after each iteration, check and strike through only genuinely completed steps, and log your verification. Do not merge branches.
```

### Prompt for Claude

```text
Read CLAUDE.md, AGENTS.md, ARCHITECTURE.md, DELEGATION.md, and PROGRESS.md. Work only on the Claude assignment in DELEGATION.md on branch agent/claude-ui-shell. Build the static local UI shell inside web/: URL input, video/audio preset choice, queue/history display, and helpful empty/error states. Make it responsive and preview it. Use fixtures until the backend API is ready. Update PROGRESS.md after each iteration, check and strike through only genuinely completed steps, and log your verification. Do not edit Go files or merge branches.
```

## Integration

1. Have each agent finish its task, run its verification, and commit its branch. Review each diff against its owned paths.
2. Merge one branch into `master`, run its checks, then merge the other. Resolve `PROGRESS.md` by retaining both iteration entries and marking only fully verified milestones complete.
3. Run `go test ./...`, start the app, and inspect the web page. The UI may still be static after this merge; record backend integration as a later task.
4. Assign the next pair of non-overlapping tasks only after agreeing on any shared Go interfaces or API response shapes.

Do not run two agents in the same worktree at the same time. Do not have both agents edit an uncommitted shared working tree. `PROGRESS.md` is branch-local during parallel work and becomes the combined source of truth only after integration.
