# fi.ashref.tn

`fi.ashref.tn` is a terminal-native agent orchestrator that answers repository questions by reasoning over local files and (optionally) the web. It streams a clean, plain‑text trace (plan, tool calls, final answer) and returns concise, cited answers.

## Features

- Single-command interface: `fi.ashref.tn "question here"`
- Structured, scrollback-friendly output (plan, tool calls, final answer) streamed as plain text
- Tool calling: `grep` (ripgrep), `shell` (guarded), and `exa_search` (web)
- Repository context builder with redaction and size limits
- Optional JSON output mode for automation
- Optional shell history context (last 50 commands) to improve command recall

## Requirements

- Go 1.24+ for building
- `rg` (ripgrep) recommended for fast grep tool (fallback exists)
- OpenRouter API key
- Exa API key (optional) for web search

## Install (local)

```bash
go build -o fi.ashref.tn ./cmd/ag-cli
```

## Add to PATH

macOS/Linux:

```bash
install -m 0755 fi.ashref.tn ~/.local/bin/fi.ashref.tn
export PATH="$HOME/.local/bin:$PATH"
```

## Quick Start (OpenRouter)

```bash
export OPENROUTER_API_KEY=...
./fi.ashref.tn "what is the tech stack here?"
```

## Quick Start (OpenAI‑spec providers)

```bash
export OPENAI_API_KEY=...
export OPENAI_BASE_URL=https://your-provider.example/v1
export OPENAI_MODEL=your-model
./fi.ashref.tn "how do I run tests?"
```

## Usage

```bash
./fi.ashref.tn "what is the tech stack here?"
./fi.ashref.tn --no-web "where is auth implemented?"
./fi.ashref.tn --json "summarize the repo"
./fi.ashref.tn --unsafe-shell "run tests and summarize failures"
./fi.ashref.tn --quiet "how do I deploy?"
./fi.ashref.tn --no-plan "show the docker command"
./fi.ashref.tn --log-file ./fi.ashref.tn.log "summarize deployment steps"
```

## Environment Variables

- `OPENROUTER_API_KEY` (preferred)
- `OPENAI_API_KEY` (fallback, OpenAI‑spec compatible providers)
- `EXA_API_KEY` (optional, enables web search)
- `AGCLI_MODEL` (optional, default: `openrouter/pony-alpha`)
- `OPENAI_MODEL` (optional, fallback model name)
- `AGCLI_MAX_STEPS` (optional, default: 8)
- `AGCLI_TIMEOUT_SECONDS` (optional, default: `60`)
- `AGCLI_OPENROUTER_BASE_URL` (optional, default: `https://openrouter.ai/api/v1`)
- `OPENAI_BASE_URL` (optional, fallback OpenAI‑spec base URL)
- `AGCLI_HTTP_REFERER` (optional, for OpenRouter attribution headers)
- `AGCLI_TITLE` (optional, for OpenRouter attribution headers)
- `AGCLI_PERSIST_RUNS` (optional, set to `true` to persist run logs)
- `AGCLI_NO_PLAN` (optional, set to `true` to skip plan generation/output)
- `AGCLI_QUIET` (optional, set to `true` to print only the final answer)
- `AGCLI_LOG_FILE` (optional, path to write plain-text output)
- `AGCLI_HISTORY_LINES` (optional, default: `50`)
- `AGCLI_NO_HISTORY` (optional, set to `true` to disable shell history context)

Notes:
- `AGCLI_*` variables and CLI flags take precedence.
- `OPENAI_*` variables are fallbacks for OpenAI‑spec compatible providers.

> Note: `AGCLI_MOCK_LLM=1` enables a deterministic mock client for tests.

## Config File (Persistent Settings)

`fi.ashref.tn` reads a config file from:

- `~/.config/fi.ashref.tn/config.yaml`
- `~/.config/fi.ashref.tn/config.json`
- (legacy) `~/.config/ag-cli/config.yaml` / `config.json`

Example:

```yaml
model: openrouter/pony-alpha
max_steps: 8
timeout: 60s
unsafe_shell_default: false
persist_runs: false
openrouter_base_url: https://openrouter.ai/api/v1
http_referer: https://example.com
title: fi.ashref.tn
output_format: text
no_plan: false
quiet: false
log_file: ""
history_lines: 50
no_history: false

tool_limits:
  grep_max_results: 200
  grep_max_bytes: 20480
  shell_max_bytes: 20480
  web_max_bytes: 30720
  context_max_bytes: 81920
  max_file_bytes: 32768
```

Create/edit the file quickly:

```bash
mkdir -p ~/.config/fi.ashref.tn
${EDITOR:-nano} ~/.config/fi.ashref.tn/config.yaml
```

## JSON Output Mode

```bash
./fi.ashref.tn --json "summarize the repo"
```

JSON mode prints a single JSON document to stdout (no streaming) containing:

- run metadata
- tool call history
- final answer
- timestamps

## Shell History Context

By default, fi.ashref.tn includes the last 50 commands from your shell history (redacted) to improve command‑recall questions. Disable with `--no-history` or set `AGCLI_NO_HISTORY=true`.

## Troubleshooting

- Missing API key: set `OPENROUTER_API_KEY` or `OPENAI_API_KEY` before running.
- `rg` not installed: the grep tool falls back to a slower Go scanner.
- Exa key missing: web search is disabled automatically.

## Releases

Tagging a version like `v0.1.0` triggers the release workflow to build multi‑arch binaries and a `checksums.txt` artifact.

## License

MIT. See `LICENSE`.
