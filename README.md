# fi.ashref.tn

fi.ashref.tn is a terminal-native agent orchestrator that answers repository questions by reasoning over local files (and optionally the web). The CLI command is `fi`.

## Quickstart

1. Build the binary:
   ```bash
   go build -o fi ./cmd/fi-cli
   ```
2. Add it to your PATH:
   ```bash
   install -m 0755 fi ~/.local/bin/fi
   export PATH="$HOME/.local/bin:$PATH"
   ```
   Zsh note: `fi` is a reserved keyword in zsh. Add this to `~/.zshrc` once:
   ```bash
   alias fi='command fi'
   ```
3. Set your API key and model:
   ```bash
   export FICLI_API_KEY=...
   export FICLI_MODEL=openrouter/pony-alpha
   ```
4. Run it:
   ```bash
   fi "question here"
   ```

## Configuration

Environment variables:

- `FICLI_API_KEY` (preferred; falls back to `OPENROUTER_API_KEY` or `OPENAI_API_KEY`)
- `FICLI_MODEL` (model override)
- `FICLI_OPENROUTER_BASE_URL` (default: `https://openrouter.ai/api/v1`)
- `FICLI_MAX_STEPS` (default: 8)
- `FICLI_TIMEOUT_SECONDS` (default: 60)
- `FICLI_HTTP_REFERER`, `FICLI_TITLE` (optional OpenRouter attribution headers)
- `FICLI_PERSIST_RUNS`, `FICLI_NO_PLAN`, `FICLI_QUIET`, `FICLI_LOG_FILE`
- `FICLI_HISTORY_LINES` (default: 50), `FICLI_NO_HISTORY`
- `FICLI_SHOW_HEADER`, `FICLI_SHOW_TOOLS`
- `FICLI_SHELL_ALLOWLIST` (comma-separated command prefixes; enables shell tool)
- `EXA_API_KEY` (optional, enables web search)

Config file (optional):

- `~/.config/fi.ashref.tn/config.yaml`
- `~/.config/fi.ashref.tn/config.json`
- macOS also supports `~/Library/Application Support/fi.ashref.tn/config.yaml`

Shell safety:

- By default, `fi` is read-only (grep only). The shell tool is disabled unless you configure an allowlist.
- Allowlist entries are command prefixes. For example, `git` allows any `git ...` command, while `git status` allows only `git status ...`.
- Network utilities like `curl` remain blocked unless `--unsafe-shell` is set.

Example config (`~/.config/fi.ashref.tn/config.yaml`):

```yaml
api_key: your_openrouter_key_here
model: openrouter/pony-alpha
show_header: false
show_tools: false
no_plan: true
shell_allowlist:
  - git status
  - git log
  - aws s3 ls
```

## Usage

```bash
fi "what is the tech stack here?"
fi --no-web "where is auth implemented?"
fi --json "summarize the repo"
fi --shell-allow "git status" "show git status"
fi --unsafe-shell "run tests and summarize failures"
```

## Output customization

By default, output is minimal:

```
fi: <answer>
```

Use flags to show more detail:

- `--show-header` to include run metadata.
- `--show-tools` to include tool call summaries.
- `--plan` to generate and show a plan.

## Troubleshooting

- Missing API key: set `FICLI_API_KEY` (or `OPENROUTER_API_KEY`/`OPENAI_API_KEY`) before running.
- `rg` not installed: the grep tool falls back to a slower Go scanner.
- Exa key missing: web search is disabled automatically.

## License

MIT. See `LICENSE`.
