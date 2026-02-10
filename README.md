# fi.ashref.tn

fi.ashref.tn is a terminal-native agent orchestrator that answers repository questions by reasoning over local files (and optionally the web). The CLI command is `fi`.

## Requirements

- Go 1.24+
- `rg` (ripgrep) recommended
- FI API key
- Exa API key (optional, enables web search)

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
3. Set your FI API key and model:
   ```bash
   export FI_API_KEY=...
   export FI_MODEL=openrouter/pony-alpha
   ```
4. Run it:
   ```bash
   fi "question here"
   ```

## Configuration

Environment variables (FI_* are the primary, unique names):

- `FI_API_KEY` (required)
- `FI_MODEL` (model override)
- `FI_BASE_URL` (OpenAI-spec base URL, default: `https://openrouter.ai/api/v1`)
- `EXA_API_KEY` (optional)
- `FI_MAX_STEPS` (default: 8)
- `FI_TIMEOUT_SECONDS` (default: 60)
- `FI_HTTP_REFERER`, `FI_TITLE` (optional OpenRouter attribution headers)
- `FI_PERSIST_RUNS`, `FI_NO_PLAN`, `FI_QUIET`, `FI_LOG_FILE`
- `FI_HISTORY_LINES` (default: 50), `FI_NO_HISTORY`

Fallback compatibility (optional):

- `OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL`
- legacy `AGCLI_*` env vars are still accepted

Config file (persistent, easy to edit):

- `~/.config/fi.ashref.tn/config.yaml`
- `~/.config/fi.ashref.tn/config.json`

Create/edit quickly:

```bash
mkdir -p ~/.config/fi.ashref.tn
${EDITOR:-nano} ~/.config/fi.ashref.tn/config.yaml
```

## Usage

```bash
fi "what is the tech stack here?"
fi --no-web "where is auth implemented?"
fi --json "summarize the repo"
fi --unsafe-shell "run tests and summarize failures"
fi --quiet "how do I deploy?"
fi --no-plan "show the docker command"
fi --log-file ./fi.log "summarize deployment steps"
```

## Troubleshooting

- Missing API key: set `FI_API_KEY` before running.
- `rg` not installed: the grep tool falls back to a slower Go scanner.
- Exa key missing: web search is disabled automatically.

## License

MIT. See `LICENSE`.
