# fi-cli

fi-cli is a terminal-native agent orchestrator that answers repository questions by reasoning over local files (and optionally the web). The CLI command is `fi`.

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
- `EXA_API_KEY` (optional, enables web search)

Config file (optional):

- `~/.config/fi-cli/config.yaml`
- `~/.config/fi-cli/config.json`

## Usage

```bash
fi "what is the tech stack here?"
fi --no-web "where is auth implemented?"
fi --json "summarize the repo"
fi --unsafe-shell "run tests and summarize failures"
```

## Troubleshooting

- Missing API key: set `FICLI_API_KEY` (or `OPENROUTER_API_KEY`/`OPENAI_API_KEY`) before running.
- `rg` not installed: the grep tool falls back to a slower Go scanner.
- Exa key missing: web search is disabled automatically.

## License

MIT. See `LICENSE`.
