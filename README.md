# V-CLI

V-CLI is a terminal-native agent orchestrator for repository Q&A. It prioritizes local evidence (grep/context), can optionally use web search, and is read-only by default.

## Install

1. Build:
   ```bash
   go build -o vcli ./cmd/fi-cli
   ```
2. Install into PATH:
   ```bash
   install -m 0755 vcli ~/.local/bin/vcli
   export PATH="$HOME/.local/bin:$PATH"
   ```
3. Optional alias:
   ```bash
   alias v='vcli'
   ```
4. Initialize config:
   ```bash
   vcli init
   ```
5. Edit the generated config file and set `api_key`.
6. Run:
   ```bash
   vcli "what's the tech stack here?"
   ```

## First-Run Onboarding

If no API key is configured, V-CLI prints onboarding instructions and exits with code `2`.

## Configuration

Config file locations:
- `~/.config/fi.ashref.tn/config.yaml`
- `~/.config/fi.ashref.tn/config.json`
- `~/Library/Application Support/fi.ashref.tn/config.yaml` (macOS)

Example config:

```yaml
api_key: "your_openrouter_key"
model: openrouter/pony-alpha
openrouter_base_url: "https://openrouter.ai/api/v1"
response_mode: quick
show_header: false
show_tools: true
no_plan: true
# shell_allowlist:
#   - git status
#   - git log
```

Environment variables:
- `FICLI_API_KEY` (preferred; fallback: `OPENROUTER_API_KEY`, `OPENAI_API_KEY`)
- `FICLI_MODEL`, `FICLI_OPENROUTER_BASE_URL`
- `FICLI_TIMEOUT_SECONDS`, `FICLI_MAX_STEPS`
- `FICLI_RESPONSE_MODE` (`quick`, `operator`, `explain`)
- `FICLI_SHOW_HEADER`, `FICLI_SHOW_TOOLS`, `FICLI_NO_TOOLS`, `FICLI_NO_PLAN`
- `FICLI_SHELL_ALLOWLIST`, `FICLI_LOG_FILE`, `FICLI_PERSIST_RUNS`
- `FICLI_HISTORY_LINES`, `FICLI_NO_HISTORY`
- `EXA_API_KEY` (optional; enables `exa_search`)

## Safety Policy

Default mode is `read-only` (shell disabled).

Use:
```bash
vcli policy check
vcli policy test "git status -sb"
```

Modes:
- `read-only`: grep/context only
- `allowlist`: shell enabled only for configured command prefixes
- `unsafe`: enabled explicitly with `--unsafe-shell`

## Usage

```bash
vcli "where is auth implemented?"
vcli --mode operator "how do I run this project?"
vcli --plan --show-header "summarize architecture"
vcli --no-tools "quick summary"
vcli --shell-allow "git status" "show git status"
```

Default output is concise:
```text
tool: grep ok (12ms, 8 lines, 644 bytes)
v: <answer>
```

## License

MIT. See `LICENSE`.
