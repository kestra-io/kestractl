# Kestra CLI

A Go-based command-line interface for managing Kestra flows, executions, namespaces, and IAM users, groups, roles, and service accounts.

## Installation

### use convenience script installer
Install the latest release (macOS/Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/kestra-io/kestractl/main/install-scripts/install.sh | bash
```

Install a specific version or custom directory:

```bash
curl -fsSL https://raw.githubusercontent.com/kestra-io/kestractl/main/install-scripts/install.sh | VERSION=1.0.0 INSTALL_DIR=~/.local/bin bash
```

### or choose and download the proper binary for your OS/arch
download the compressed or plain binary for your platform at https://github.com/kestra-io/kestractl/releases

example:
```
curl -fsSL -o kestractl https://github.com/kestra-io/kestractl/releases/download/1.0.0/kestractl_1.0.0_linux_arm64
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
# Token auth
kestractl config add default http://localhost:8080 main --token YOUR_TOKEN --default

# Basic auth (username + password)
kestractl config add default http://localhost:8080 main --username you@example.com --password YOUR_PASSWORD --default
```

This creates a configuration file at `~/.kestractl/config.yaml`:

```yaml
contexts:
  default:
    host: http://localhost:8080
    tenant: main
    auth_method: token
    token: YOUR_TOKEN
    headers:                     # Optional: persisted extra HTTP headers
      - "X-Custom-Header:value"
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
export KESTRACTL_OUTPUT=json                       # Optional: table or json
export KESTRACTL_HEADER="X-Custom-Header:value"   # Optional: extra HTTP header
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
- `--username` - Basic auth username (alternative to `--token`)
- `--password` - Basic auth password (alternative to `--token`)
- `--tenant` - Tenant name
- `--header` - Extra HTTP header to include in all requests (format: `Key:Value`, repeatable)
- `--output` / `-o` - Output format (`table` or `json`)
- `--config` - Custom config file path (default: `~/.kestractl/config.yaml`)
- `--verbose` / `-v` - Verbose output (warning: prints credentials in HTTP requests)

### Config Management

```bash
# Add a new context
kestractl config add dev http://localhost:8080 main --token YOUR_TOKEN

# Add and set as default
kestractl config add prod https://prod.kestra.io production --token PROD_TOKEN --default

# Add with extra HTTP headers (persisted in the context)
kestractl config add dev http://localhost:8080 main --token YOUR_TOKEN \
  --header "X-Custom-Header:value" --header "Authorization:Bearer extra"

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

### Key-Value Store (kv)

Supported types: `STRING`, `NUMBER`, `BOOLEAN`, `DATETIME`, `DATE`, `DURATION`, `JSON`

```bash
# List all key-value entries
kestractl kv list

# List key-value entries in a namespace
kestractl kv list my.namespace

# Set a key — format: kv set <namespace> <type> <key> <value>
kestractl kv set my.namespace STRING api_key "my-secret"
kestractl kv set my.namespace NUMBER retries 3
kestractl kv set my.namespace BOOLEAN enabled true
kestractl kv set my.namespace JSON settings '{"feature":true}'

# Update an existing key (fails if key does not exist)
kestractl kv update my.namespace NUMBER retries 5

# Read a key (shows type and value)
kestractl kv get my.namespace api_key

# Delete a key (alias: rm)
kestractl kv delete my.namespace api_key
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

# Skip the pre-flight namespace existence check
kestractl nsfiles upload my.namespace ./assets resources --allow-missing-namespace

# Delete a file
kestractl nsfiles delete my.namespace workflows/example.yaml

# Delete a directory recursively
kestractl nsfiles delete my.namespace workflows --recursive

# Ignore missing targets
kestractl nsfiles delete my.namespace workflows/example.yaml --force
```

### Plugins

```bash
# Download every plugin JAR for a given Kestra version into ./plugins
kestractl plugins download 1.3.9

# Custom output directory
kestractl plugins download 1.3.9 --plugins-dir ./vendor/plugins

# Parallel downloads
kestractl plugins download 1.3.9 --concurrency 4

# `develop` and `latest` are aliases for the in-development version
kestractl plugins download develop
```

### Workers

```bash
# Generate a worker registration token (runs offline, no Kestra instance required)
kestractl workers registration-tokens generate
```

### Users (Enterprise Edition)

User management requires Kestra Enterprise Edition. Users are instance-level resources.

> Use `--user-password` to set a user's password — not `--password`, which is the
> global basic-auth flag used to authenticate the CLI itself.

```bash
# List / filter users (alias: ls)
kestractl users list
kestractl users list --query alice --output json

# Get user details (alias: show, describe)
kestractl users get <user_id>

# Create a user (--email is required)
kestractl users create --email alice@example.com --first-name Alice --user-password 'S3cret!'

# Create a super-admin
kestractl users create --email bob@example.com --superadmin

# Update a user — only the flags you pass change; other attributes are preserved
kestractl users update <user_id> --first-name Alicia
kestractl users update <user_id> --superadmin=false

# Set a user's password
kestractl users set-password <user_id> --user-password 'N3wPass!'

# Set the groups a user belongs to in the active tenant (no --group clears them)
kestractl users set-groups <user_id> --group <group_id>

# Delete a user (alias: rm) — prompts for confirmation unless --yes
kestractl users delete <user_id>
kestractl users delete <user_id> --yes

# Manage a user's API tokens (the full token is shown only once, at creation)
kestractl users tokens create <user_id> --name ci-token
kestractl users tokens list <user_id>
kestractl users tokens delete <user_id> <token_id>
```

### Groups (Enterprise Edition)

Group management requires Kestra Enterprise Edition. Groups are tenant-scoped resources.

```bash
# List / filter groups (alias: ls)
kestractl groups list
kestractl groups list --query admins --output json

# Get group details (alias: show, describe)
kestractl groups get <group_id>

# Create a group (--name is required; --member is repeatable for initial members)
kestractl groups create --name admins --description 'Platform admins'
kestractl groups create --name admins --member <user_id> --member <user_id>

# Update a group — only the flags you pass change; other attributes are preserved
kestractl groups update <group_id> --description 'Updated description'
kestractl groups update <group_id> --name platform-admins

# Delete a group (alias: rm) — prompts for confirmation unless --yes
kestractl groups delete <group_id>
kestractl groups delete <group_id> --yes

# Manage group members
kestractl groups members list <group_id>
kestractl groups members add <group_id> <user_id>
kestractl groups members remove <group_id> <user_id>
```

### Roles (Enterprise Edition)

Role management requires Kestra Enterprise Edition. Roles are tenant-scoped (the
active tenant is used).

A role carries a `permissions` payload: a map of resource type (e.g. `FLOW`,
`EXECUTION`, `NAMESPACE`, `SECRET`, `KVSTORE`, …) to a list of permission levels
(`READ`, `CREATE`, `UPDATE`, `DELETE`). You can provide it inline with the
repeatable `--permission TYPE:LEVEL[,LEVEL]` flag, or from a YAML/JSON file with
`--permissions-file` — but not both at once.

```bash
# List / filter roles (alias: ls)
kestractl roles list
kestractl roles list --query editor --output json
kestractl roles list --page 1 --size 50 --sort name:asc

# Get role details, including its permissions (aliases: show, describe)
kestractl roles get <role_id>

# Create a role with inline permissions (--name is required, plus at least one permission)
kestractl roles create --name editor \
  --description "Can edit flows and view executions" \
  --permission FLOW:READ,CREATE,UPDATE \
  --permission EXECUTION:READ

# Create a role from a permissions file (YAML or JSON)
kestractl roles create --name viewer --permissions-file perms.yaml

# perms.yaml
#   FLOW:
#     - READ
#   EXECUTION:
#     - READ

# Update a role — only the flags you pass change; other attributes are preserved.
# Passing --permission replaces the entire permissions block (it does not merge).
kestractl roles update <role_id> --description "Updated description"
kestractl roles update <role_id> --permission FLOW:READ,CREATE,UPDATE,DELETE
kestractl roles update <role_id> --default

# Delete a role (alias: rm) — prompts for confirmation unless --yes
kestractl roles delete <role_id>
kestractl roles delete <role_id> --yes
```

### Service Accounts (Enterprise Edition)

Service account management requires Kestra Enterprise Edition. Service accounts
are instance-level resources (command aliases: `service-account`, `sa`).

> `update` is a partial update of the name/description only. Tenant access,
> super-admin status and group membership are left untouched — set those at
> creation time.

```bash
# List service accounts (alias: ls)
kestractl service-accounts list
kestractl service-accounts list --output json
kestractl service-accounts list --page 1 --size 50 --sort name:asc

# Get service account details (aliases: show, describe)
kestractl service-accounts get <service_account_id>

# Create a service account (--name is required; lowercase alphanumeric and dashes)
kestractl service-accounts create --name ci-bot --description "CI pipeline"

# Create a super-admin service account with tenant access (--tenant-grant is repeatable)
kestractl service-accounts create --name ops-bot --superadmin --tenant-grant main

# Update name/description — other attributes are preserved
kestractl service-accounts update <service_account_id> --description "Updated description"
kestractl service-accounts update <service_account_id> --name new-bot-name

# Delete a service account (alias: rm) — prompts for confirmation unless --yes
kestractl service-accounts delete <service_account_id>
kestractl service-accounts delete <service_account_id> --yes

# Manage a service account's API tokens (the full token is shown only once, at creation)
kestractl service-accounts tokens create <service_account_id> --name deploy-token
kestractl service-accounts tokens create <service_account_id> --name short-lived --max-age P30D --extended
kestractl service-accounts tokens list <service_account_id>
kestractl service-accounts tokens delete <service_account_id> <token_id>
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
    ├── client_test.go         # Unit tests
    ├── auth.go                # AuthManager - ~/.kestractl/config.yaml persistence (YAML)
    ├── auth_test.go           # Unit tests
    ├── render.go              # Renderer: table or JSON output
    ├── render_test.go         # Unit tests
    ├── telemetry.go           # PostHog event per command (disable via env var)
    ├── telemetry_test.go      # Unit tests
    ├── config.go              # Config subcommands (add, show, use, remove)
    ├── flows.go               # Flows commands (list, get, deploy, validate)
    ├── flows_test.go          # Unit tests
    ├── executions.go          # Executions commands (run, get)
    ├── executions_test.go     # Unit tests
    ├── namespaces.go          # Namespaces commands (list)
    ├── namespaces_test.go     # Unit tests
    ├── kv.go                  # KV store commands (list, get, set, update, delete)
    ├── kv_test.go             # Unit tests
    ├── nsfiles.go             # Namespace files commands (list, get, upload, delete)
    ├── nsfiles_test.go        # Unit tests
    ├── plugins.go             # Plugins commands (download)
    ├── plugins_test.go        # Unit tests
    ├── workers.go             # Workers commands (registration-tokens generate)
    ├── workers_test.go        # Unit tests
    ├── users.go               # IAM users commands (list, get, create, update, delete, set-groups, set-password, tokens)
    ├── users_test.go          # Unit tests
    ├── groups.go              # IAM groups commands (list, get, create, update, delete, members)
    ├── groups_test.go         # Unit tests
    ├── roles.go               # IAM roles commands (list, get, create, update, delete)
    ├── roles_test.go          # Unit tests
    ├── service_accounts.go    # IAM service accounts commands (list, get, create, update, delete, tokens)
    ├── service_accounts_test.go # Unit tests
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
