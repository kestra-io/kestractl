# Kestra CLI

A Go-based command-line interface for managing Kestra flows, executions, and namespaces.

## Installation

```bash
# Download dependencies (requires Go 1.21+)
go mod download

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

This creates a configuration file at `~/.kestra/config.yaml`:

```yaml
contexts:
  default:
    host: http://localhost:8080
    tenant: main
    auth_method: token
    token: YOUR_TOKEN
default_context: default
```

### Multiple Contexts

You can manage multiple Kestra environments (development, staging, production):

```bash
# Add multiple contexts
kestra config add dev http://localhost:8080 main --token DEV_TOKEN
kestra config add prod https://prod.kestra.io production --token PROD_TOKEN

# List all contexts
kestra config show

# Switch between contexts
kestra config use prod
```

### Environment Variables

You can also configure the CLI using environment variables, which override config file settings:

```bash
export KESTRA_HOST=http://localhost:8080
export KESTRA_TENANT=main
export KESTRA_TOKEN=YOUR_TOKEN
export KESTRA_OUTPUT=json  # Optional: table or json
```

### Configuration Precedence

Following the [12-factor app](https://12factor.net/config) methodology, configuration is resolved in this order (highest to lowest):

1. **Command-line flags** (`--host`, `--token`, etc.) - Highest priority
2. **Environment variables** (`KESTRA_HOST`, `KESTRA_TOKEN`, etc.)
3. **Config file** (`~/.kestra/config.yaml` or custom via `--config`)
4. **Default values** - Lowest priority

This allows you to:
- Store credentials securely in `~/.kestra/config.yaml` for daily use
- Override with environment variables in CI/CD pipelines
- Override with flags for one-off commands

## Usage

All commands support global flags for connection and output configuration:
- `--host` - Kestra host URL
- `--token` / `-t` - API authentication token
- `--tenant` - Tenant name
- `--output` / `-o` - Output format (`table` or `json`)
- `--config` - Custom config file path (default: `~/.kestra/config.yaml`)

### Config Management

```bash
# Add a new context
kestra config add dev http://localhost:8080 main --token YOUR_TOKEN

# Add and set as default
kestra config add prod https://prod.kestra.io production --token PROD_TOKEN --default

# List all contexts
kestra config show

# Switch default context
kestra config use prod

# Remove a context
kestra config remove dev
```

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

The CLI uses a simple, direct architecture built on [Cobra](https://github.com/spf13/cobra), [Viper](https://github.com/spf13/viper), and the official Kestra Go SDK.

```
main.go → root.go → commands → Client → Kestra SDK → Kestra API
```

Configuration follows the [12-factor app](https://12factor.net/config) methodology:
- Viper handles configuration from multiple sources (flags, env vars, config file)
- Clear precedence order ensures predictable behavior
- Flags are bound to Viper automatically via `PersistentPreRunE`

### Project Structure

```
kestra-cli/
├── main.go                    # Entrypoint - calls cli.Execute()
├── go.mod                     # Dependencies: cobra, viper, kestra SDK, yaml
└── src/cli/
    ├── root.go                # Root command, global flags, Viper initialization
    ├── client.go              # Client wrapper for SDK with Viper config resolution
    ├── auth.go                # AuthManager - ~/.kestra/config.yaml persistence (YAML)
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
| 12-factor config with Viper | Viper handles flags > env vars > config file precedence automatically. Clean, predictable config resolution. |
| YAML config format | Human-readable, supports multiple contexts, industry standard (similar to kubectl, docker, etc.). |
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
