# Security

## Shell Safety

- Shell tool is disabled by default (read-only mode uses grep only).
- Shell tool is enabled only when a command allowlist is configured.
- Allowlist entries are command prefixes (e.g., `git` or `git status`).
- Potentially destructive commands are blocked unless `--unsafe-shell` is explicitly set.
- Network utilities like `curl` are blocked by default even if allowlisted.
- Interactive commands (vim/less/etc.) are not allowed in v1.

## Secret Handling

- Repository context excludes sensitive files by denylist:
  - `.env*`, `*.pem`, `*.key`, `*.p12`, `*.pfx`, `id_rsa*`, `.aws/credentials`, `.npmrc`, `.docker/config.json`
- Snippets are redacted for common secret patterns.
- Tool output is redacted before sending to the model.
- Optional shell history context is redacted and can be disabled via `--no-history`.

## API Keys

- API keys can be provided via environment variables or the config file; they are never written to disk by the app.
- Run logs do not include secrets and can be disabled via `persist_runs: false`.
