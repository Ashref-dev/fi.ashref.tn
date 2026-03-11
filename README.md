# fi-cli

fi-cli is a terminal-native agent orchestrator for repository Q&A. It prioritizes local evidence (`grep` + `list_tree` + repo context), can optionally use web search, and is read-only by default.

This README reflects current behavior validated by integration/unit tests in this repository.

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
3. Optional shorthand alias:
   ```bash
   alias fic='fi-cli'
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
fallback_models:
  - openrouter/anthropic/claude-3.5-sonnet
  - openrouter/openai/gpt-4o-mini
openrouter_base_url: "https://openrouter.ai/api/v1"
response_mode: quick
show_header: false
show_tools: true
no_plan: true
llm_retry_max_attempts: 2
llm_retry_initial_backoff: 300ms
llm_retry_max_backoff: 2s
llm_circuit_failure_threshold: 5
llm_circuit_window: 30s
llm_circuit_open_duration: 15s
tool_parallelism: 4
tool_timeout_seconds: 10
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
- `FICLI_MODEL`, `FICLI_FALLBACK_MODELS` (comma-separated), `FICLI_OPENROUTER_BASE_URL`
- `FICLI_TIMEOUT_SECONDS`, `FICLI_MAX_STEPS`
- `FICLI_TOOL_PARALLELISM`, `FICLI_TOOL_TIMEOUT_SECONDS`
- `FICLI_LLM_RETRY_MAX_ATTEMPTS`, `FICLI_LLM_RETRY_INITIAL_BACKOFF`, `FICLI_LLM_RETRY_MAX_BACKOFF`
- `FICLI_LLM_CIRCUIT_FAILURE_THRESHOLD`, `FICLI_LLM_CIRCUIT_WINDOW`, `FICLI_LLM_CIRCUIT_OPEN_DURATION`
- `FICLI_RESPONSE_MODE` (`quick`, `operator`, `explain`)
- `FICLI_SHOW_HEADER`, `FICLI_SHOW_TOOLS`, `FICLI_NO_TOOLS`, `FICLI_NO_PLAN`
- `FICLI_TIMINGS` (print stage timing diagnostics)
- `FICLI_SHELL_ALLOWLIST`, `FICLI_LOG_FILE`, `FICLI_PERSIST_RUNS`
- `FICLI_HISTORY_LINES`, `FICLI_NO_HISTORY`
- `EXA_API_KEY` (optional; enables `exa_search`)

## Runtime UX

- Planning is opt-in: default behavior is execute directly without a plan.
- Use `--plan` to request planning first.
- Every run starts with an automatic `list_tree` snapshot (`path="."`, `max_depth=2`) so the model sees top-level project layout before answering.
- After structure snapshot, the model is guided to run focused `grep` evidence before final output.
- Text mode streams model output and shows a transient `thinking` spinner while waiting.
- Spinner resumes under plan/tool lines and stops on first model token or run completion.
- `--timings` prints per-stage timing diagnostics and the slowest stage.
- Tool calls in a single model step run in parallel only when every call is read-only and allowlisted (`grep`, `exa_search`, `list_tree`).
- Tool result messages are always fed back to the model in the original call order (deterministic behavior).
- LLM calls use retry/backoff, a per-run circuit breaker, and optional fallback model chain.

## Safety Policy

Default mode is `read-only` (shell disabled).

Use:
```bash
fi-cli policy check
fi-cli policy test "git status -sb"
```

Modes:
- `read-only`: `grep` + `list_tree` + repo context only (shell disabled)
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
fi-cli --timings "where is auth implemented?"
fi-cli "show me the folder structure"
fi-cli --fallback-model openrouter/openai/gpt-4o-mini "analyze this repository"
fi-cli --no-tools "quick summary"
fi-cli --shell-allow "git status" "show git status"
fi-cli --shell-allow "ls" --shell-allow "ls -la" "inspect current repo context"
fi-cli
```

Input tips (zsh/macOS):
- You can run `fi-cli` with no args and type at `question>`.
- If your inline question contains apostrophes (`'`), prefer double quotes around the full question.
- `quote>` in zsh means your shell is waiting for a closing quote before fi-cli starts.

Default output is concise:
```text
tool: grep ok (12ms, 8 lines, 644 bytes)
fi: <answer>
```

Structure discovery output uses `list_tree`:
```text
tool: list_tree ok (6ms, 42 lines, 2103 bytes)
fi: <answer>
```

Timing output example:
```text
timings:
  question_resolution_input: 1ms
  config_load: 2ms
  repo_root_resolution: 1ms
  repo_context_build: 7ms
  history_load: 1ms
  planning: 0ms
  first_model_response_wait: 1834ms
  tool_execution_total: 28ms
  first_answer_token_latency: 1836ms
  total_run_duration: 2042ms
  slowest_stage: first_model_response_wait (1834ms)
```

Shell context tip:
- In allowlist mode, add `ls` / `ls -la` to `shell_allowlist` if you want the agent to quickly inspect top-level context before deeper commands.

History context:
- By default, fi-cli includes recent shell history lines in model context (`FICLI_HISTORY_LINES`, disable with `--no-history` / `FICLI_NO_HISTORY`).

## License

MIT. See `LICENSE`.
