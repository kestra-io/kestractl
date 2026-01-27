# Kestra CLI

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

### Environment Variables

You can also configure the CLI using environment variables, which override config file settings:

```bash
export KESTRA_HOST=http://localhost:8080
export KESTRA_TENANT=main
export KESTRA_TOKEN=YOUR_TOKEN
export KESTRA_OUTPUT=json  # Optional: table or json
```

Configuration precedence (highest to lowest):
1. Command-line flags (`--host`, `--token`, etc.)
2. Environment variables (`KESTRA_HOST`, `KESTRA_TOKEN`, etc.)
3. Config file (`~/.kestra/config`)
4. Default values

## Usage

All commands support global flags for connection and output configuration:
- `--host` - Kestra host URL
- `--token` / `-t` - API authentication token
- `--tenant` - Tenant name
- `--output` / `-o` - Output format (`table` or `json`)

### Flows

```bash
# List flows in a namespace (alias: ls)
kestra flows list my.namespace

# Get a specific flow (aliases: show, describe)
kestra flows get my.namespace my-flow

# Deploy a flow from YAML (aliases: create, apply)
kestra flows deploy path/to/flow.yaml

# Override existing flow
kestra flows deploy path/to/flow.yaml --override
```

### Executions

```bash
# Trigger a flow execution (aliases: trigger, execute)
kestra executions run my.namespace my-flow

# Trigger and wait for completion
kestra executions run my.namespace my-flow --wait

# Get execution details (aliases: show, describe)
kestra executions get 2TLGqHrXC9k8BczKJe5djX

# Kill running executions
kestra executions kill-running
```

### Namespaces

```bash
# List all namespaces (alias: ls)
kestra namespaces list

# Filter namespaces with query
kestra namespaces list --query my.namespace
```

### Output Formats

```bash
# Table output (default, human-readable)
kestra flows list my.namespace

# JSON output (for scripting)
kestra flows list my.namespace --output json
```

### Overriding Configuration

```bash
# Override config settings with flags
kestra flows get my.namespace my-flow \
  --host https://kestra.example.com \
  --tenant production \
  --token YOUR_TOKEN

# Or use environment variables
KESTRA_HOST=https://kestra.example.com \
KESTRA_TENANT=production \
KESTRA_TOKEN=YOUR_TOKEN \
  kestra flows list my.namespace
```

## Architecture

The CLI uses a simple, direct architecture built on [Cobra](https://github.com/spf13/cobra) and the official Kestra Go SDK.

```
main.go → root.go → commands → Client → Kestra SDK → Kestra API
```

### Project Structure

```
kestra-cli/
├── main.go                    # Entrypoint - calls cli.Execute()
├── go.mod                     # Dependencies: cobra, kestra SDK, yaml
└── src/cli/
    ├── root.go                # Root command, global flags, command registration
    ├── client.go              # Client wrapper for SDK with config resolution
    ├── auth.go                # AuthManager - ~/.kestra/config persistence
    ├── helpers.go             # Output formatting utilities
    ├── config.go              # Config subcommands (add, show, use, remove)
    ├── flows.go               # Flows commands (list, get, deploy)
    ├── flows_test.go          # Unit tests
    ├── executions.go          # Executions commands (run, get, kill-running)
    ├── executions_test.go     # Unit tests
    ├── namespaces.go          # Namespaces commands (list)
    ├── namespaces_test.go     # Unit tests
    └── testdata/              # Test fixtures
        └── flow.yaml
```

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Direct SDK calls | No unnecessary abstraction layers. Commands call the SDK directly through a thin `Client` wrapper. |
| Single config resolution | `NewClient()` resolves config once: flags > env vars > config file. Clear precedence. |
| Pure functions for logic | Business logic in testable `run*()` functions separate from Cobra command wiring. |
| Minimal test mocking | Tests focus on pure functions and argument validation. Integration tests for SDK calls. |

## Development

### Local Build

```bash
go build ./...
./kestra --help
```

### Testing

```bash
go test ./...
```

### Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed instructions on adding new commands.

## Requirements

- Go 1.21 or newer
- Access to a Kestra instance and API token
