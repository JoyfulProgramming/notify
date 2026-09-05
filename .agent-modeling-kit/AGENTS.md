# Learnings

Reusable learnings accumulated while processing prompts for this board. Append new
ones in a compressed, reusable form; only add if not already covered here.

- `/place-element` requires an existing column — create one via the timeline API if missing.
- `/wdyt` posts QUESTION comments onto nodes — use for analysis only, not modifications.
- The `board_id`, `timeline_id`, and `organization_id` from each prompt provide full context — pass them to skills that need them.
- If a prompt's `context.timelineId` is present and non-null, it overrules the prompt's own `timeline_id` field — it's the chapter the user was pointing at on the canvas, which can differ from whatever chapter the prompt/voice session was scoped to. Resolve `TIMELINE_ID` from `context.timelineId` first, falling back to `timeline_id` only when it's absent, before passing it to any skill.
- Same pattern for node references: if a prompt's `context.selectedNodes` array is present and non-empty, its first entry overrules the prompt's own `node_id` field (e.g. for `/handle-comment`'s `nodeId`) — it reflects the actual canvas selection at prompt time, whereas `node_id` is only set when the prompt originated from a specific node/comment.
- Same pattern for cell references: if a prompt's `context.selectedCell.id` is present, it overrules any cell reference (e.g. `"A2"`) parsed from the prompt text — pass it as `/place-element`'s `cellName` argument and skip the text-parsing fast path entirely.
- Node events POST to `/api/boards/:boardId/nodes/events` using `node:created`, `node:changed`, `node:deleted`.
- `/update-slice-status` rejects moving a slice into a status it's already in — this is a concurrency guard so two agents can't both claim the same slice. Treat this as `ALREADY_IN_STATUS`, not a task failure: drop the prompt, move on to the next task, and do not retry the same update.
- macOS/BSD `date` silently ignores GNU-only format specifiers like `%N`/`%3N` (sub-second precision) instead of erroring — it prints the literal characters, producing a malformed timestamp that only fails downstream. Don't shell out to `date` for sub-second precision; use `$(( $(date +%s) * 1000 ))` for whole-second-in-ms, or a runtime call (`Date.now()`, `process.hrtime()`) instead.
- Before retrying a failed shell command a second time, diagnose why it failed (e.g. a GNU/BSD flag mismatch) rather than re-running it unchanged — repeating the same command produces the same failure and just burns retries.
- For a truly empty board (CHAPTER exists with default rows but no content nodes), "create an initial screen" means placing the first HTML_SCREEN at the actor row of column A (cellName "A1"), and its content should be designed from project memory about the app's purpose rather than from any existing board content (there is none).

- This board's live comments endpoint (POST .../nodes/:nodeId/comments) rejects type=QUESTION — its swagger schema only allows COMMENT/TASK, contradicting the handle-comment skill and CLAUDE.md's documented QUESTION type. Use type=COMMENT (prefix the text with "QUESTION:") as the working substitute until the skill/API are reconciled.
