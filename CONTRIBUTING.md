# Contributing

## Development Setup

- Go 1.24+
- Optional: `rg` for faster grep tool

## Build

```bash
go build -o ag-cli ./cmd/ag-cli
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
AGCLI_MOCK_LLM=1 go run ./cmd/ag-cli --json "test question"
```
