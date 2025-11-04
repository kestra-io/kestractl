# Kestra CLI (Under Development)

A Go-based command-line interface for managing Kestra flows, executions, and namespaces.

## Installation

```bash
# Download dependencies (requires Go 1.21+)
go mod tidy

# Build the binary
go build -o kestra
```

You can also install it into your `$GOBIN`:

```bash
go install ./...
```

## Quick Setup

Configure your Kestra instance and credentials:

```bash
kestra config add default http://localhost:8080 main --token YOUR_TOKEN --default
```

This creates a configuration file at `~/.kestra/config` with your host, tenant, and authentication token.

## Usage

### Get a flow

```bash
kestra flows get <namespace> <flow_id>
```

### Run an execution

```bash
kestra executions run <namespace> <flow_id>
```

### Deploy a flow

```bash
kestra flows deploy path/to/flow.yaml
```

### Additional Options

All commands support `--output json` for JSON output and can override config with `--host`, `--tenant`, and `--token` flags:

```bash
kestra flows get my.namespace my-flow --output json --host http://localhost:8080 --token YOUR_TOKEN
```

## Development

Project layout:

- `main.go`: CLI entrypoint
- `src/api_client/`: API helpers for Kestra endpoints
- `src/cli/`: Cobra commands and shared CLI helpers

### Local build

```bash
go build ./...
./kestra --help
```

## Requirements

- Go 1.21 or newer
- Access to a Kestra instance and API token
