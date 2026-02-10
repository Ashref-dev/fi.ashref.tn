# Contributing

## Development Setup

- Go 1.24+
- Optional: `rg` for faster grep tool

## Build

```bash
go build -o fi ./cmd/fi-cli
```

## Test

```bash
go test ./...
```

## Lint / Vet

```bash
go vet ./...
```

## Formatting

```bash
gofmt -w $(rg --files -g'*.go')
```

## Mock LLM

For deterministic test runs without API calls:

```bash
FICLI_MOCK_LLM=1 go run ./cmd/fi-cli --json "test question"
```
