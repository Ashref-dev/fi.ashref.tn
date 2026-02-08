# Security

## Shell Safety

- Shell tool is allowlisted by default.
- Potentially destructive commands are blocked unless `--unsafe-shell` is explicitly set.
- Network utilities are blocked by default.
- Interactive commands (vim/less/etc.) are not allowed in v1.

## Secret Handling

- Repository context excludes sensitive files by denylist:
  - `.env*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `.aws/credentials`, `.npmrc`, `.docker/config.json`
- Snippets are redacted for common secret patterns.
- Tool output is redacted before sending to the model.
- Optional shell history context is redacted and can be disabled via `--no-history`.

## API Keys

- API keys are read from environment variables only and are never written to disk.
- Run logs do not include secrets and can be disabled via `persist_runs: false`.
