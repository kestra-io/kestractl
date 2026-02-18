# Kestra CLI

A Go-based command-line interface for managing Kestra flows, executions, and namespaces.

## Installation

### use convenience script installer
Install the latest release (macOS/Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/kestra-io/kestractl/main/install-scripts/install.sh | VERSION=1.0.0-alpha.5 bash
```

Install a specific version or custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/kestra-io/kestractl/main/install-scripts/install.sh | VERSION=1.0.0-alpha.5 INSTALL_DIR=~/.local/bin bash
```

### or choose and download the proper binary for your OS/arch
download the compressed or plain binary for your platform at https://github.com/kestra-io/kestractl/releases

example:
```
curl -fsSL -o kestractl https://github.com/kestra-io/kestractl/releases/download/1.0.0-alpha.4/kestractl_1.0.0-alpha.4_linux_arm64
chmod +x kestractl
```

### or compile it
```bash
git clone git@github.com:kestra-io/kestractl.git

# Download dependencies (requires Go 1.25+)
go mod download

# Build the binary
go build -o kestractl
```

You can also install it into your `$GOBIN`:

```bash
go install ./...
```

## Quick Setup

Configure your Kestra instance and credentials:

```bash
kestractl config add default http://localhost:8080 main --token YOUR_TOKEN --default
```

This creates a configuration file at `~/.kestractl/config.yaml`:

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
kestractl config add dev http://localhost:8080 main --token DEV_TOKEN
kestractl config add prod https://prod.kestra.io production --token PROD_TOKEN

# List all contexts
kestractl config show

# Switch between contexts
kestractl config use prod
```

### Environment Variables

You can also configure the CLI using environment variables, which override config file settings:

```bash
export KESTRACTL_HOST=http://localhost:8080
export KESTRACTL_TENANT=main
export KESTRACTL_TOKEN=YOUR_TOKEN
export KESTRACTL_OUTPUT=json  # Optional: table or json
```

### Configuration Precedence

Following the [12-factor app](https://12factor.net/config) methodology, configuration is resolved in this order (highest to lowest):

1. **Command-line flags** (`--host`, `--token`, etc.) - Highest priority
2. **Environment variables** (`KESTRACTL_HOST`, `KESTRACTL_TOKEN`, etc.)
3. **Config file** (`~/.kestractl/config.yaml` or custom via `--config`)
4. **Default values** - Lowest priority

This allows you to:
- Store credentials securely in `~/.kestractl/config.yaml` for daily use
- Override with environment variables in CI/CD pipelines
- Override with flags for one-off commands

### Telemetry

The CLI sends anonymous telemetry to help the Kestra team understand real usage and improve the product over time, and it never blocks command execution.

Set `KESTRACTL_TELEMETRY_DISABLED=true` to disable telemetry.

## Usage

All commands support global flags for connection and output configuration:
- `--host` - Kestra host URL
- `--token` / `-t` - API authentication token
- `--tenant` - Tenant name
- `--output` / `-o` - Output format (`table` or `json`)
- `--config` - Custom config file path (default: `~/.kestractl/config.yaml`)
- `--verbose` / `-v` - Verbose output (warning: prints credentials in HTTP requests)

### Config Management

```bash
# Add a new context
kestractl config add dev http://localhost:8080 main --token YOUR_TOKEN

# Add and set as default
kestractl config add prod https://prod.kestra.io production --token PROD_TOKEN --default

# List all contexts
kestractl config show

# Switch default context
kestractl config use prod

# Remove a context
kestractl config remove dev
```

### Flows

```bash
# List flows in a namespace (alias: ls)
kestractl flows list my.namespace

# List flows across all namespaces
kestractl flows list

# Get a flow source (aliases: show, describe)
kestractl flows get my.namespace my-flow

# Deploy a single flow from YAML (aliases: create, apply)
kestractl flows deploy path/to/flow.yaml

# Deploy all flows in a directory (recursive)
kestractl flows deploy ./flows/

# Deploy with namespace override (all flows go to specified namespace)
kestractl flows deploy ./flows/ --namespace prod.namespace

# Override existing flows
kestractl flows deploy ./flows/ --override

# Stop on first error (fail-fast)
kestractl flows deploy ./flows/ --fail-fast

# Combine flags
kestractl flows deploy ./flows/ --namespace prod --override --fail-fast

# Validate a single flow
kestractl flows validate path/to/flow.yaml

# Validate all flows in a directory (recursive)
kestractl flows validate ./flows/
```

### Executions

```bash
# Trigger a flow execution (aliases: trigger, execute)
kestractl executions run my.namespace my-flow

# Trigger and wait for completion
kestractl executions run my.namespace my-flow --wait

# Get execution details (aliases: show, describe)
kestractl executions get 2TLGqHrXC9k8BczKJe5djX

```

### Namespaces

```bash
# List all namespaces (alias: ls)
kestractl namespaces list

# Filter namespaces with query
kestractl namespaces list --query my.namespace
```

### Namespace Files (nsfiles)

```bash
# List files at the namespace root (alias: ls)
kestractl nsfiles list my.namespace

# List files in a directory
kestractl nsfiles list my.namespace --path workflows/

# List files recursively
kestractl nsfiles list my.namespace --path workflows/ --recursive

# Get a file's raw content (alias: cat)
kestractl nsfiles get my.namespace workflows/example.yaml

# Get a specific revision
kestractl nsfiles get my.namespace workflows/example.yaml --revision 3

# Upload a single file
kestractl nsfiles upload my.namespace ./local.txt workflows/local.txt

# Upload a directory (recursive)
kestractl nsfiles upload my.namespace ./assets resources

# Override existing files
kestractl nsfiles upload my.namespace ./assets resources --override

# Stop on the first error
kestractl nsfiles upload my.namespace ./assets resources --fail-fast

# Delete a file
kestractl nsfiles delete my.namespace workflows/example.yaml

# Delete a directory recursively
kestractl nsfiles delete my.namespace workflows --recursive

# Ignore missing targets
kestractl nsfiles delete my.namespace workflows/example.yaml --force
```

### Output Formats

```bash
# Table output (default, human-readable)
kestractl flows list my.namespace

# JSON output (for scripting)
kestractl flows list my.namespace --output json
```

### Overriding Configuration

```bash
# Override config settings with flags
kestractl flows get my.namespace my-flow \
  --host https://kestra.example.com \
  --tenant production \
  --token YOUR_TOKEN

# Or use environment variables
KESTRACTL_HOST=https://kestra.example.com \
KESTRACTL_TENANT=production \
KESTRACTL_TOKEN=YOUR_TOKEN \
  kestractl flows list my.namespace
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
kestractl/
├── main.go                    # Entrypoint - calls cli.Execute()
├── go.mod                     # Dependencies: cobra, viper, kestra SDK, yaml
└── src/cli/
    ├── root.go                # Root command, global flags, Viper initialization
    ├── client.go              # Client wrapper for SDK with Viper config resolution
	    ├── auth.go                # AuthManager - ~/.kestractl/config.yaml persistence (YAML)
    ├── helpers.go             # Output formatting utilities
    ├── config.go              # Config subcommands (add, show, use, remove)
    ├── flows.go               # Flows commands (list, get, deploy)
    ├── flows_test.go          # Unit tests
    ├── executions.go          # Executions commands (run, get)
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
./kestractl --help
```

### Testing

```bash
go test ./src/...
```

### Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed instructions on adding new commands.

## Requirements

- Go 1.25 or newer
- Access to a Kestra instance and API token
