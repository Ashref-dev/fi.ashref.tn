# Tech Stack & Library Decisions

- Language: Go 1.24 (fast startup, static binaries, strong stdlib for CLI tooling).
- CLI: `github.com/spf13/cobra` (mature flags/commands).
- Output: plain text streaming renderer (stdout; no full‑screen takeover).
- Config: `github.com/spf13/viper` (env + config file support).
- Logging: `go.uber.org/zap` (structured logging, dev/prod configs).
- HTTP retry: `github.com/hashicorp/go-retryablehttp` (Exa API reliability).
- UUID: `github.com/google/uuid` (run IDs).
- LLM client: `github.com/openai/openai-go/v3` (OpenAI-compatible schema + streaming) with OpenRouter base URL.
- Web search: Exa Search API (HTTP POST `https://api.exa.ai/search`).

Chosen versions (pinned):
- cobra v1.10.1
- (no TUI dependencies)
- viper v1.21.0
- zap v1.27.0
- openai-go v3.18.0
- uuid v1.6.0
- retryablehttp v0.7.8 (>=0.7.7)

Assumptions noted: OpenRouter model defaults to `openrouter/pony-alpha` but can be overridden by `OPENAI_MODEL` or `FICLI_MODEL`. Exa key optional.

# Interpreted Feature Summary

`fi-cli` is a terminal-native agent orchestrator for codebase Q&A. It builds a lightweight repository summary, calls an OpenRouter LLM with tool definitions, executes tool calls (grep/shell/web), streams plain-text output (plan, tool calls, final answer), optionally adds recent shell history context, and ends with a cited final answer. The CLI command is `fi`. JSON mode outputs a full run log for automation.

# Assumptions & Unknowns

- Assumption: Users will provide `FI_API_KEY` and optionally `EXA_API_KEY`.
- Assumption: `rg` is installed for best performance; fallback exists when not.
- Assumption: `openrouter/pony-alpha` supports tool calling and streaming.
- Unknown: Exact repo size and file types; context size limits may need tuning per user.
- Unknown: Whether users need custom HTTP-Referer/X-Title for OpenRouter attribution.

# Success Criteria

## User Success

- Can run `fi "question"` and receive a concise, cited answer.
- Sees plan/tool calls/final answer in scrollback (quiet/no-plan supported).
- Shell commands are safely constrained by default.

## System / Business Success

- Typical run completes < 60s.
- Structured JSON output is valid and complete when `--json` is used.
- Graceful degradation when rg/Exa are missing.

## Non-Goals

- No persistent multi-turn chat sessions.
- No vector DB / embeddings search.
- No GUI or server backend in v1.

## Constraints & Assumptions

- Go-only implementation.
- OpenRouter-compatible API required.
- Context size capped (default 80KB).
- Web search optional and disabled when Exa key missing.

# UX Plan

## Primary Flow

1. User runs `fi "question"`.
2. CLI prints header + model + repo root.
3. Model-generated plan (3–8 bullets) appears.
4. Trace events stream (tool calls + results).
5. Final answer streams with citations.

## UI States & Edge Cases

- Loading/plan pending: placeholder until plan arrives.
- Tool failures: show error in trace and continue.
- No matches: tool output indicates zero results.
- Max steps reached: final answer includes warning.
- JSON mode: stdout is a single JSON document (no streaming).

## Accessibility

- Section headers and consistent ordering.
- Readable without color.
- No interactive selection required.

## UX Copy / Messaging

- Clear error messages (missing API key, blocked shell command).
- Tool outputs show status, duration, line/byte count, truncation flag.

# Technical Strategy (Conceptual)

## Architecture Overview

- `cmd/fi-cli`: Cobra CLI entry.
- `internal/agent`: orchestration loop (plan → tool calls → final answer).
- `internal/tools`: grep/shell/exa tools with validation + truncation.
- `internal/repo`: repo root + context builder + denylist.
- `internal/render`: Bubble Tea UI + JSON mode.
- `internal/llm`: OpenRouter client (OpenAI-compatible).

## Data Model

- RunResult (run_id, timestamps, model, status, final_answer, tool_calls, events).
- ToolCallRecord (name, input, output, duration, status).

## API Contracts

- LLM: OpenRouter OpenAI-compatible Chat Completions.
- Exa Search: POST to `https://api.exa.ai/search` with `query` + `numResults` + optional `contents.text`.

## Security & Privacy

- Denylist sensitive files (`.env*`, `*.pem`, etc.).
- Redact secret-like patterns from context and tool output.
- Shell tool allowlist + destructive pattern blocking by default.

## Performance Considerations

- Context size capped to 80KB.
- Tool output truncation for grep/shell/web.
- Ripgrep used when available.

## Logging / Monitoring

- Zap structured logs to stderr.
- JSON mode includes full run log payload.

## Migration Strategy

- No DB migrations in v1.
- Optional run persistence writes JSON files to `~/.local/share/fi-cli/runs/`.

# Risks & Tradeoffs

- Tradeoff: Bubble Tea full-screen rendering vs. pure stdout (chosen for structured UI).
- Risk: Tool output may still miss secrets not matching redaction patterns.
- Risk: Some OpenRouter models may not fully support tool calling.
- Risk: Large repos may hit context cap and require more tool calls.

# Step-by-Step Execution Plan

## Milestone 1

Objective: Core CLI + agent loop

Tasks:
- Cobra command and flag parsing
- OpenRouter client wrapper
- Plan generation and basic final answer flow

Validation:
- `go test ./...`

Acceptance:
- CLI runs and returns final answer with model calls

## Milestone 2

Objective: Tools + tool calling

Tasks:
- Tool registry
- grep tool (rg + fallback)
- shell tool with allowlist
- tool call loop

Validation:
- Unit tests for tools and guardrails

Acceptance:
- Tool calls executed with structured trace

## Milestone 3

Objective: Repo context + web search

Tasks:
- Repo root detection
- Context builder + redaction
- Exa search tool

Validation:
- Repo context tests

Acceptance:
- Web tool works with key, no-web disables

## Milestone 4

Objective: UI polish + JSON mode

Tasks:
- Stdout renderer with sections
- JSON output mode
- Optional run persistence

Validation:
- E2E JSON test

Acceptance:
- Scrollback-friendly output and valid JSON

# Implementation Summary (What Was Built)

## Key Decisions

- OpenRouter via openai-go with configurable base URL and headers.
- Bubble Tea + viewport for structured terminal output.
- Tools emit structured JSON payloads to the model.

## Files / Modules Overview

- `/Users/mohamedashrefbenabdallah/Sideprojects/fi-cli/cmd/fi-cli/main.go`
- `/Users/mohamedashrefbenabdallah/Sideprojects/fi-cli/internal/agent/*`
- `/Users/mohamedashrefbenabdallah/Sideprojects/fi-cli/internal/tools/*`
- `/Users/mohamedashrefbenabdallah/Sideprojects/fi-cli/internal/repo/*`
- `/Users/mohamedashrefbenabdallah/Sideprojects/fi-cli/internal/render/*`
- `/Users/mohamedashrefbenabdallah/Sideprojects/fi-cli/internal/llm/*`

## Feature Flags / Rollout Notes

- `--unsafe-shell` enables non-allowlisted commands.
- `--no-web` disables Exa web search.
- `--json` outputs run log JSON only.
- `--quiet` prints only the final answer.
- `--no-plan` skips plan generation/output.
- `--log-file` writes plain-text output to a file.
- `--no-history` disables shell history context.

# Test & Validation Plan

## Tests Added

- Unit: secret redaction, shell guardrails, repo root detection, grep fallback.
- Integration: agent run with mock LLM.
- E2E: CLI JSON mode with mock LLM.

## Commands Executed (With Results)

- `go test ./...` (pass)
- `go vet ./...` (pass)

# Production Readiness Checklist

- [x] Env var handling (keys not printed)
- [x] Safe shell defaults
- [x] Denylisted files not read
- [x] Context + tool output truncation
- [x] JSON mode valid output
- [x] CI workflow for test/vet/format

# Deployment Guide

1. Build:
   ```bash
   go build -o fi ./cmd/fi-cli
   ```
2. Export keys:
   ```bash
   export FI_API_KEY=...
   export EXA_API_KEY=... # optional
   ```
3. Run:
   ```bash
   ./fi "your question"
   ```
4. Release (CI):
   - Push a tag like `v0.1.0` to trigger multi‑arch builds and `checksums.txt` artifacts.

# Rollback Plan

- Replace the binary with the prior version.
- Remove optional run logs at `~/.local/share/fi-cli/runs/` if needed.

# Handoff Pack for Implementation Agents

## Ordered Checklist

1. Confirm Go 1.24+ installed.
2. Set `FI_API_KEY`.
3. Build with `go build -o fi ./cmd/fi-cli`.
4. Run `go test ./...` and `go vet ./...`.
5. Run CLI in a sample repo and validate output sections.

## Definition of Done

- CLI prints all required sections.
- Tool calls execute with guardrails.
- JSON mode outputs valid JSON only.
- Tests and vet pass.

## What NOT to Change Without Revisiting the Plan

- Tool safety allowlist/denylist.
- Context size and tool output limits.
- OpenRouter base URL and model default.
