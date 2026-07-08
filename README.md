# Kestra CLI

A Go-based command-line interface for managing Kestra flows, executions, triggers, namespaces, key-value store, namespace files, apps, dashboards, assets, blueprints, test suites, IAM users, groups, roles, service accounts, bindings, and invitations.

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

# List flows in a namespace with pagination
kestractl flows list-by-namespace my.namespace --page 1 --size 50

# List deprecated flows
kestractl flows list-deprecated

# Get a flow source (aliases: show, describe)
kestractl flows get my.namespace my-flow

# Get a specific task within a flow
kestractl flows task my.namespace my-flow my-task-id

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

# Validate a single flow or directory
kestractl flows validate path/to/flow.yaml
kestractl flows validate ./flows/

# Validate a task or trigger definition inline
kestractl flows validate-task --namespace my.namespace --flow my-flow --task-id my-task
kestractl flows validate-trigger --namespace my.namespace --flow my-flow --trigger-id my-trigger

# Search flows by source content
kestractl flows search-by-source --query "http.request"

# Bulk-update flows from YAML
kestractl flows bulk-update --file flows.yaml

# Generate the graph topology from a flow source file
kestractl flows generate-graph-from-source --file flow.yaml

# Show the graph topology of an existing flow (optionally a specific revision)
kestractl flows graph my.namespace my-flow --revision 3 --output json

# List available expressions for a flow
kestractl flows expressions --namespace my.namespace --flow my-flow

# Show namespace-level dependencies
kestractl flows namespace-dependencies my.namespace

# Show dependencies of a specific flow
kestractl flows dependencies my.namespace my-flow

# Enable / disable flows
kestractl flows enable my.namespace my-flow
kestractl flows disable my.namespace my-flow

# Delete a flow
kestractl flows delete my.namespace my-flow

# Delete / disable / enable matching a query
kestractl flows delete-by-query --namespace my.namespace --query old-
kestractl flows disable-by-query --namespace my.namespace
kestractl flows enable-by-query --namespace my.namespace

# Export flows
kestractl flows export --namespace my.namespace --output-file flows.zip
kestractl flows export-by-ids my.namespace/flow-a my.namespace/flow-b --output-file export.zip
kestractl flows export-by-query --namespace my.namespace --output-file export.zip

# Import flows from a ZIP archive
kestractl flows import flows.zip

# Sync an entire namespace from a YAML file (delete flows absent from the file)
kestractl flows namespace-sync my.namespace flows.yaml --delete --override

# View and manage flow revisions
kestractl flows revisions my.namespace my-flow
kestractl flows delete-revisions my.namespace my-flow --before-revision 5

# Manage concurrency limits
kestractl flows concurrency-limits my.namespace my-flow
kestractl flows update-concurrency my.namespace my-flow --limit 10
```

### Executions

```bash
# Trigger a flow execution (aliases: trigger, execute)
kestractl executions run my.namespace my-flow

# Trigger and wait for completion
kestractl executions run my.namespace my-flow --wait

# Get execution details (aliases: show, describe)
kestractl executions get 2TLGqHrXC9k8BczKJe5djX

# List executions
kestractl executions list --namespace my.namespace --flow my-flow

# Watch an execution in real time (alias: follow) — exits non-zero on failure
kestractl executions watch 2TLGqHrXC9k8BczKJe5djX

# Get the latest execution for each flow
kestractl executions latest --flow my.namespace:my-flow --flow my.namespace:other-flow

# Control execution state
kestractl executions kill   2TLGqHrXC9k8BczKJe5djX
kestractl executions pause  2TLGqHrXC9k8BczKJe5djX
kestractl executions resume 2TLGqHrXC9k8BczKJe5djX
kestractl executions restart 2TLGqHrXC9k8BczKJe5djX
kestractl executions force-run 2TLGqHrXC9k8BczKJe5djX

# Replay and unqueue
kestractl executions replay 2TLGqHrXC9k8BczKJe5djX
kestractl executions replay-with-inputs 2TLGqHrXC9k8BczKJe5djX --input key=value
kestractl executions unqueue 2TLGqHrXC9k8BczKJe5djX

# Labels
kestractl executions set-labels 2TLGqHrXC9k8BczKJe5djX env=prod team=platform

# Bulk operations by IDs
kestractl executions set-labels-bulk env=prod --ids id1 --ids id2
kestractl executions unqueue-bulk id1 id2 id3
kestractl executions change-status-by-ids --status SUCCESS id1 id2

# Bulk operations by query
kestractl executions kill-by-query    --namespace my.namespace --flow my-flow
kestractl executions pause-by-query   --namespace my.namespace
kestractl executions resume-by-query  --namespace my.namespace
kestractl executions restart-by-query --namespace my.namespace
kestractl executions replay-by-query  --namespace my.namespace --latest-revision
kestractl executions force-run-by-query --namespace my.namespace
kestractl executions delete-by-query  --namespace my.namespace --delete-logs --delete-storage
kestractl executions unqueue-by-query --namespace my.namespace
kestractl executions set-labels-by-query env=prod --namespace my.namespace
kestractl executions update-status-by-query --namespace my.namespace --new-status KILLED
# Filter by any field with --filter FIELD:OPERATION:VALUE (e.g. STATE:EQUALS:SUCCESS)
kestractl executions kill-by-query --filter STATE:EQUALS:RUNNING

# Trigger an execution via webhook (--method GET|POST|PUT, --path appends a URL suffix)
kestractl executions trigger-webhook my.namespace my-flow my-webhook-key
kestractl executions trigger-webhook my.namespace my-flow my-webhook-key --method POST
kestractl executions trigger-webhook my.namespace my-flow my-webhook-key --method PUT --path extra/segment

# Flow graph and info
kestractl executions flow-graph 2TLGqHrXC9k8BczKJe5djX
kestractl executions flow-info my.namespace my-flow
kestractl executions flow-info-by-id 2TLGqHrXC9k8BczKJe5djX

# Download execution output files
kestractl executions download-file 2TLGqHrXC9k8BczKJe5djX --path outputs/result.csv
kestractl executions file-metadata 2TLGqHrXC9k8BczKJe5djX --path outputs/result.csv

# Evaluate an expression against an execution
kestractl executions eval-expression 2TLGqHrXC9k8BczKJe5djX "{{ outputs.myTask.value }}"

# Force-change the status of an execution
kestractl executions change-status 2TLGqHrXC9k8BczKJe5djX SUCCESS

# Update a specific task run's state
kestractl executions update-taskrun 2TLGqHrXC9k8BczKJe5djX taskRunId SUCCESS

# Delete an execution
kestractl executions delete 2TLGqHrXC9k8BczKJe5djX
```

### Triggers

```bash
# List all triggers
kestractl triggers list

# List triggers for a specific flow
kestractl triggers search-for-flow my.namespace my-flow

# Enable / disable a trigger
kestractl triggers enable  my.namespace my-flow my-trigger
kestractl triggers disable my.namespace my-flow my-trigger

# Unlock a locked trigger
kestractl triggers unlock my.namespace my-flow my-trigger

# Restart a trigger
kestractl triggers restart my.namespace my-flow my-trigger

# Update a trigger (e.g. mark disabled)
kestractl triggers update my.namespace my-flow my-trigger --disabled

# Delete a trigger
kestractl triggers delete my.namespace my-flow my-trigger

# Bulk operations by IDs (format: namespace/flowId/triggerId)
kestractl triggers delete-by-ids  my.ns/my-flow/sched my.ns/my-flow/webhook
kestractl triggers unlock-by-ids  my.ns/my-flow/sched
kestractl triggers disable-by-ids my.ns/my-flow/sched
kestractl triggers enable-by-ids  my.ns/my-flow/sched

# Bulk operations by query
kestractl triggers delete-by-query  --namespace my.namespace
kestractl triggers unlock-by-query  --namespace my.namespace
kestractl triggers disable-by-query --namespace my.namespace
kestractl triggers enable-by-query  --namespace my.namespace

# Backfill management (single trigger)
kestractl triggers create-backfill my.namespace my-flow my-trigger \
  --start 2024-01-01T00:00:00Z --end 2024-02-01T00:00:00Z
kestractl triggers backfill-pause   my.namespace my-flow my-trigger
kestractl triggers backfill-unpause my.namespace my-flow my-trigger
kestractl triggers backfill-delete  my.namespace my-flow my-trigger

# Backfill management by IDs
kestractl triggers pause-backfill-by-ids   my.ns/my-flow/sched
kestractl triggers unpause-backfill-by-ids my.ns/my-flow/sched
kestractl triggers delete-backfill-by-ids  my.ns/my-flow/sched

# Backfill management by query
kestractl triggers pause-backfill-by-query   --namespace my.namespace
kestractl triggers unpause-backfill-by-query --namespace my.namespace
kestractl triggers delete-backfill-by-query  --namespace my.namespace

# Export all triggers as CSV
kestractl triggers export-csv
kestractl triggers export-csv --output-file triggers.csv
```

### Namespaces

```bash
# List all namespaces (alias: ls)
kestractl namespaces list

# Filter namespaces with query
kestractl namespaces list --query my.namespace

# Autocomplete namespace names
kestractl namespaces autocomplete --query my.

# Get namespace details
kestractl namespaces get my.namespace

# Create / update / delete a namespace
kestractl namespaces create my.namespace
kestractl namespaces update my.namespace --description "Production namespace"
kestractl namespaces delete my.namespace

# Set namespace variables (repeatable --variable, or a YAML/JSON file); replaces
# the full variable set on that namespace
kestractl namespaces create my.namespace --variable env=prod --variable region=eu
kestractl namespaces update my.namespace --variables-file variables.yml

# View inherited secrets and variables
kestractl namespaces inherited-secrets   my.namespace
kestractl namespaces inherited-variables my.namespace

# Plugin defaults for a namespace (inherited configuration)
kestractl namespaces plugin-defaults my.namespace

# Export / import plugin defaults
kestractl namespaces export-plugin-defaults my.namespace --output-file defaults.yaml
kestractl namespaces import-plugin-defaults my.namespace defaults.yaml
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

# Set a key with a TTL (ISO 8601 duration)
kestractl kv set my.namespace STRING session_token "abc" --ttl PT1H
kestractl kv set my.namespace STRING cache_key "value" --ttl P7D

# Update an existing key (fails if key does not exist)
kestractl kv update my.namespace NUMBER retries 5
kestractl kv update my.namespace STRING session_token "new" --ttl PT30M

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

# Bootstrap a standalone/remote worker: download only the "core" plugins required
# by a Kestra configuration — internal storage, secret manager, and the
# queue/repository backend. Bundled backends (local storage, JDBC, Kafka) are
# skipped; enterprise backends need --edition ALL (the default) or EE.
kestractl plugins download 1.3.9 --from-config /etc/kestra/application.yaml

# Preview the core plugins a config needs without downloading (pipes into download)
kestractl plugins list 1.3.9 --from-config /etc/kestra/application.yaml
```

### Workers

```bash
# Generate a worker registration token (runs offline, no Kestra instance required)
kestractl workers registration-tokens generate
```

### Dashboards (Enterprise Edition)

```bash
# List dashboards (alias: ls)
kestractl dashboards list
kestractl dashboards list --query my-dashboard --output json

# Get dashboard details (aliases: show, describe)
kestractl dashboards get <id>

# Create a dashboard from a YAML file
kestractl dashboards create --file my-dashboard.yaml

# Update an existing dashboard
kestractl dashboards update <id> --file my-dashboard.yaml

# Delete a dashboard (alias: rm)
kestractl dashboards delete <id>

# Show the tenant's default dashboard settings
kestractl dashboards defaults

# Validate a dashboard or a single chart definition
kestractl dashboards validate --file my-dashboard.yaml
kestractl dashboards validate-chart --file my-chart.yaml

# Preview a chart's data without saving it
kestractl dashboards preview-chart --file my-chart.yaml --output json

# Fetch the data for a chart of an existing dashboard
kestractl dashboards chart-data <dashboard-id> <chart-id>
kestractl dashboards chart-data <dashboard-id> <chart-id> --file filters.yaml --output json

# Export chart data as CSV (to stdout or --output-file)
kestractl dashboards export-chart-csv --file my-chart.yaml --output-file chart.csv
kestractl dashboards export-chart-data-csv <dashboard-id> <chart-id> --output-file chart.csv
```

### Apps (Enterprise Edition)

Apps are low-code interfaces built on top of flows.

```bash
# List apps (alias: ls)
kestractl apps list
kestractl apps list --namespace my.namespace --output json

# Get app details (aliases: show, describe)
kestractl apps get <uid>

# Deploy a new app from a YAML file
kestractl apps deploy --file my-app.yaml

# Update an existing app
kestractl apps update <uid> --file my-app.yaml

# Enable / disable an app
kestractl apps enable  <uid>
kestractl apps disable <uid>

# Delete an app (alias: rm)
kestractl apps delete <uid>

# Export all apps as a ZIP archive
kestractl apps export --output-file apps.zip

# Import apps from a ZIP archive
kestractl apps import apps.zip

# Bulk enable / disable / delete multiple apps
kestractl apps bulk-enable  uid-1 uid-2 uid-3
kestractl apps bulk-disable uid-1 uid-2 uid-3
kestractl apps bulk-delete  uid-1 uid-2 --yes

# List all tags used across apps
kestractl apps tags

# Search apps from the catalog
kestractl apps catalog
kestractl apps catalog --query reporting --output json

# Inspect files produced by an app execution view
kestractl apps file-meta    <view-id> --path /path/to/file
kestractl apps file-preview <view-id> --path /path/to/file --max-rows 50

# Download logs for an app execution
kestractl apps logs <view-id> --min-level ERROR --output-file app.log
```

### Assets (Enterprise Edition)

```bash
# List assets (alias: ls)
kestractl assets list
kestractl assets list --output json

# Get asset details (aliases: show, describe)
kestractl assets get <id>

# Create an asset from a file
kestractl assets create --name my-asset --file asset.csv

# Delete an asset (alias: rm)
kestractl assets delete <id>

# Show an asset's dependency graph (alias: deps)
kestractl assets dependencies <id> --expand-all --output json

# Bulk-delete assets by IDs or by query filters
kestractl assets delete-by-ids id1 id2 id3
kestractl assets delete-by-query --namespace my.namespace
kestractl assets delete-by-query --filter NAMESPACE:EQUALS:my.namespace --purge

# Inspect and manage lineage events (alias: lineage)
kestractl assets lineage-events list --output json
kestractl assets lineage-events delete-by-query --namespace my.namespace

# Inspect and manage asset usages (alias: usage)
kestractl assets usages list --output json
kestractl assets usages delete-by-query --namespace my.namespace
```

### Blueprints

```bash
# Search community blueprints
kestractl blueprints community search --query "kafka"
kestractl blueprints community search --query "etl" --output json

# Get a community blueprint
kestractl blueprints community get <id>

# Get the flow source of a community blueprint
kestractl blueprints community source <id>

# Get the topology graph of a community blueprint
kestractl blueprints community graph <id> --output json

# Manage internal flow blueprints (Enterprise Edition)
kestractl blueprints flow list
kestractl blueprints flow get <id>
kestractl blueprints flow get <id> --legacy   # use the legacy /blueprints/flow/{id} endpoint
kestractl blueprints flow create --title "My Blueprint" --source-file blueprint.yaml --tag etl
kestractl blueprints flow update <id> --title "My Blueprint" --source-file blueprint.yaml
kestractl blueprints flow delete <id>

# Generate a flow source from a flow blueprint template
kestractl blueprints flow use-template <id> --input env=prod --input region=eu

# Manage internal/custom blueprints (Enterprise Edition)
kestractl blueprints custom get <id>
kestractl blueprints custom source <id>
kestractl blueprints custom create --title "My Blueprint" --source-file blueprint.yaml
kestractl blueprints custom update <id> --title "My Blueprint" --source-file blueprint.yaml
kestractl blueprints custom delete <id>
```

### Test Suites (Enterprise Edition)

```bash
# List test suites (alias: ls)
kestractl test-suites list
kestractl test-suites list --namespace my.namespace

# Get a test suite
kestractl test-suites get my.namespace my-test-suite

# Create / update a test suite from a YAML file
kestractl test-suites create --file suite.yaml
kestractl test-suites update my.namespace my-test-suite --file suite.yaml

# Validate a test suite YAML definition without creating it
kestractl test-suites validate --file suite.yaml

# Run a test suite
kestractl test-suites run my.namespace my-test-suite

# Run test suites matching a query
kestractl test-suites run-by-query --namespace my.namespace

# Bulk enable / disable / delete
kestractl test-suites delete-bulk my.namespace/suite-a my.namespace/suite-b
kestractl test-suites disable-bulk my.namespace/suite-a
kestractl test-suites enable-bulk  my.namespace/suite-a

# Query and retrieve results
kestractl test-suites search-results --namespace my.namespace
kestractl test-suites last-result --ids my.namespace/suite-a --ids my.namespace/suite-b
kestractl test-suites get-result <result_id>

# Delete a test suite
kestractl test-suites delete my.namespace my-test-suite
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

# Autocomplete user names
kestractl users autocomplete --query ali

# Create a user (--email is required)
kestractl users create --email alice@example.com --first-name Alice --user-password 'S3cret!'

# Create a super-admin
kestractl users create --email bob@example.com --superadmin

# Update a user — only the flags you pass change; other attributes are preserved
kestractl users update <user_id> --first-name Alicia
kestractl users update <user_id> --superadmin=false

# Set a user's password
kestractl users set-password <user_id> --user-password 'N3wPass!'

# Change your own password
kestractl users change-my-password --old-password 'OldPass!' --new-password 'N3wPass!'

# Grant / revoke super-admin status
kestractl users set-super-admin <user_id> --superadmin
kestractl users set-super-admin <user_id> --superadmin=false

# Mark a user as restricted, or lift the restriction (targeted PATCH)
kestractl users set-restricted <user_id> --restricted=true
kestractl users set-restricted <user_id> --restricted=false

# Delete an auth method for a user
kestractl users delete-auth-method <user_id> BASIC_AUTH

# Set the groups a user belongs to in the active tenant (no --group clears them)
kestractl users set-groups <user_id> --group <group_id>

# Impersonate a user (returns an impersonation token)
kestractl users impersonate <user_id>

# Revoke all refresh tokens for a user
kestractl users revoke-refresh-token <user_id>

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

# Autocomplete group names
kestractl groups autocomplete --query adm

# Look up multiple groups by IDs
kestractl groups list-by-ids <id1> <id2>

# Create a group (--name is required; --member is repeatable for initial members)
kestractl groups create --name admins --description 'Platform admins'
kestractl groups create --name admins --member <user_id> --member <user_id>

# Update a group — only the flags you pass change; other attributes are preserved
kestractl groups update <group_id> --description 'Updated description'
kestractl groups update <group_id> --name platform-admins

# Set a user's group membership (replaces all current memberships in this group)
kestractl groups set-membership <group_id> <user_id>

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

# Autocomplete role names
kestractl roles autocomplete --query edi

# Look up multiple roles by IDs
kestractl roles list-from-ids <id1> <id2>

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

# Grant / revoke super-admin status
kestractl service-accounts set-super-admin <service_account_id> --superadmin
kestractl service-accounts set-super-admin <service_account_id> --superadmin=false

# Delete a service account (alias: rm) — prompts for confirmation unless --yes
kestractl service-accounts delete <service_account_id>
kestractl service-accounts delete <service_account_id> --yes

# Manage a service account's API tokens (the full token is shown only once, at creation)
kestractl service-accounts tokens create <service_account_id> --name deploy-token
kestractl service-accounts tokens create <service_account_id> --name short-lived --max-age P30D --extended
kestractl service-accounts tokens list <service_account_id>
kestractl service-accounts tokens delete <service_account_id> <token_id>
```

### Bindings (Enterprise Edition)

IAM role bindings assign a role to a user, group, or service account within a tenant.

```bash
# List bindings (alias: ls)
kestractl bindings list
kestractl bindings list --output json

# Get binding details (aliases: show, describe)
kestractl bindings get <binding_id>

# Create a binding
kestractl bindings create --role <role_id> --user <user_id>
kestractl bindings create --role <role_id> --group <group_id>

# Create multiple bindings from a JSON file
kestractl bindings bulk-create --file bindings.json

# Delete a binding (alias: rm)
kestractl bindings delete <binding_id>
```

### Invitations (Enterprise Edition)

```bash
# List all invitations
kestractl invitations list
kestractl invitations list --output json

# List invitations for a specific email address
kestractl invitations list-by-email user@example.com

# Get an invitation
kestractl invitations get <invitation_id>

# Create an invitation
kestractl invitations create --email user@example.com --role <role_id>

# Delete an invitation (alias: rm)
kestractl invitations delete <invitation_id>
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
├── main.go                        # Entrypoint - calls cli.Execute()
├── go.mod                         # Dependencies: cobra, viper, kestra SDK, yaml
└── src/cli/
    ├── root.go                    # Root command, global flags, Viper initialization
    ├── client.go                  # Client wrapper for SDK with Viper config resolution
    ├── client_test.go
    ├── auth.go                    # AuthManager - ~/.kestractl/config.yaml persistence
    ├── auth_test.go
    ├── render.go                  # Renderer: table or JSON output
    ├── render_test.go
    ├── telemetry.go               # PostHog event per command (disable via env var)
    ├── telemetry_test.go
    ├── config.go                  # Config subcommands (add, show, use, remove)
    ├── flows.go                   # Flows commands
    ├── flows_test.go
    ├── executions.go              # Executions commands
    ├── executions_test.go
    ├── triggers.go                # Triggers commands
    ├── triggers_test.go
    ├── namespaces.go              # Namespaces commands
    ├── namespaces_test.go
    ├── kv.go                      # KV store commands (list, get, set, update, delete)
    ├── kv_test.go
    ├── nsfiles.go                 # Namespace files commands
    ├── nsfiles_test.go
    ├── plugins.go                 # Plugins commands (download, list)
    ├── plugins_test.go
    ├── workers.go                 # Workers commands (registration-tokens generate)
    ├── workers_test.go
    ├── dashboards.go              # Dashboards commands (EE)
    ├── dashboards_test.go
    ├── apps.go                    # Apps commands (EE)
    ├── apps_test.go
    ├── assets.go                  # Assets commands (EE)
    ├── assets_test.go
    ├── blueprints.go              # Blueprints commands (community + internal EE)
    ├── blueprints_test.go
    ├── test_suites.go             # Test suites commands (EE)
    ├── test_suites_test.go
    ├── users.go                   # IAM users commands (EE)
    ├── users_test.go
    ├── groups.go                  # IAM groups commands (EE)
    ├── groups_test.go
    ├── roles.go                   # IAM roles commands (EE)
    ├── roles_test.go
    ├── service_accounts.go        # IAM service accounts commands (EE)
    ├── service_accounts_test.go
    ├── bindings.go                # IAM role bindings commands (EE)
    ├── bindings_test.go
    ├── invitations.go             # IAM invitations commands (EE)
    ├── invitations_test.go
    └── testdata/                  # Test fixtures
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
