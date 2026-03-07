# fi-cli

fi-cli is a terminal-native agent orchestrator for repository Q&A. It prioritizes local evidence (grep/context), can optionally use web search, and is read-only by default.

## Install

1. Build:
   ```bash
   go build -o fi-cli ./cmd/fi-cli
   ```
2. Install into PATH:
   ```bash
   install -m 0755 fi-cli ~/.local/bin/fi-cli
   export PATH="$HOME/.local/bin:$PATH"
   ```
3. Optional alias:
   ```bash
   alias fi='command fi-cli'
   ```
4. Initialize config:
   ```bash
   fi-cli init
   ```
5. Edit the generated config file and set `api_key`.
6. Run:
   ```bash
   fi-cli "what's the tech stack here?"
   ```
7. Inspect runtime policy/settings any time:
   ```bash
   fi-cli about
   fi-cli policy check
   ```

## First-Run Onboarding

If no API key is configured, fi-cli prints onboarding instructions and exits with code `2`.

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
tool_limits:
  grep_max_calls: 30
  shell_max_calls: 30
  web_max_calls: 30
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
fi-cli policy check
fi-cli policy test "git status -sb"
```

Modes:
- `read-only`: grep/context only
- `allowlist`: shell enabled only for configured command prefixes
- `unsafe`: enabled explicitly with `--unsafe-shell`

Tool call budgets (default):
- `grep`: 30 calls/run
- `shell`: 30 calls/run
- `exa_search`: 30 calls/run

## Usage

```bash
fi-cli "where is auth implemented?"
fi-cli --mode operator "how do I run this project?"
fi-cli --plan --show-header "summarize architecture"
fi-cli --no-tools "quick summary"
fi-cli --shell-allow "git status" "show git status"
```

Default output is concise:
```text
tool: grep ok (12ms, 8 lines, 644 bytes)
fi: <answer>
```

## License

MIT. See `LICENSE`.
