# Contributing to Kestra CLI

This guide explains how to add new commands to the Kestra CLI.

## Architecture Overview

The CLI uses a simple, direct pattern:

1. **Commands** handle CLI parsing, validation, and output formatting
2. **Client** wraps the Kestra SDK with authentication
3. **Pure functions** contain business logic for testability

```
Command (Cobra) → NewClient() → client.API.* → Kestra SDK → API
                       ↓
              run*() function → output
```

## Adding a New Command

Follow these steps to add a new resource command (e.g., `kestra secrets list`).

### Step 1: Create the Command File

Create `src/cli/{resource}.go` (e.g., `src/cli/secrets.go`):

```go
package cli

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newSecretsCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "secrets",
        Short: "Manage secrets",
    }

    cmd.AddCommand(newSecretsListCommand())
    cmd.AddCommand(newSecretsGetCommand())

    return cmd
}
```

### Step 2: Implement Subcommands

Each subcommand follows this pattern:

```go
func newSecretsListCommand() *cobra.Command {
    return &cobra.Command{
        Use:   "list <namespace>",
        Short: "List secrets in a namespace.",
        Long: `List all secrets in the specified namespace.

Returns a table showing secret keys and their namespace.`,
        Example: `  # List all secrets in a namespace
  kestra secrets list my.namespace

  # List secrets with JSON output
  kestra secrets list my.namespace --output json`,
        Aliases: []string{"ls"},
        Args:    cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            // 1. Validate output format
            if err := validateOutputFormat(); err != nil {
                return err
            }

            // 2. Create client (handles config resolution)
            client, err := NewClient()
            if err != nil {
                return err
            }

            // 3. Call the pure function with business logic
            return runSecretsList(client, args[0])
        },
    }
}

// runSecretsList contains the business logic - separate for testability
func runSecretsList(client *Client, namespace string) error {
    // Call the SDK directly
    secrets, _, err := client.API.SecretsAPI.ListSecrets(client.Ctx, namespace, client.Tenant).Execute()
    if err != nil {
        return formatSDKError(err)
    }

    // Handle JSON output
    if globalFlags.Output == "json" {
        return printJSON(secrets)
    }

    // Table output
    w := tabWriter()
    fmt.Fprintln(w, "KEY\tNAMESPACE")
    for _, secret := range secrets {
        fmt.Fprintf(w, "%s\t%s\n", secret.GetKey(), secret.GetNamespace())
    }
    w.Flush()

    return nil
}
```

### Step 3: Register in root.go

Add your command to the root command in `src/cli/root.go`:

```go
func NewRootCommand() *cobra.Command {
    root := &cobra.Command{
        // ...
    }

    // Existing commands
    root.AddCommand(newVersionCommand())
    root.AddCommand(newConfigCommand())
    root.AddCommand(newFlowsCommand())
    root.AddCommand(newNamespacesCommand())
    root.AddCommand(newExecutionsCommand())
    
    // Add your new command
    root.AddCommand(newSecretsCommand())

    return root
}
```

### Step 4: Write Tests

Create `src/cli/{resource}_test.go`:

```go
package cli

import (
    "strings"
    "testing"
)

func TestSecretsListCommand_NoArgs(t *testing.T) {
    cmd := newSecretsListCommand()
    _, err := executeCommand(cmd)
    if err == nil {
        t.Fatal("expected error when no args provided")
    }
    if !strings.Contains(err.Error(), "accepts 1 arg") {
        t.Fatalf("expected args error, got: %v", err)
    }
}

func TestSecretsListCommand_Help(t *testing.T) {
    cmd := newSecretsListCommand()
    output, err := executeCommand(cmd, "--help")
    if err != nil {
        t.Fatalf("Execute() returned error: %v", err)
    }

    if !strings.Contains(output, "List all secrets") {
        t.Fatalf("expected help text, got: %s", output)
    }
}
```

**Test Strategy:**
- Test argument validation (no args, wrong number of args)
- Test pure functions directly when possible
- Test help output and flag presence
- Integration tests (optional) for SDK calls

## Command Conventions

### Use, Short, Long, Example

```go
&cobra.Command{
    Use:   "list <namespace>",           // Command syntax
    Short: "List secrets in a namespace.", // One-line description
    Long: `Detailed description with
formatting and usage guidelines.`,
    Example: `  # Example with comment
  kestra secrets list my.namespace`,
    Aliases: []string{"ls"},              // Common shortcuts
    Args:    cobra.ExactArgs(1),          // Argument validation
}
```

### Flags

```go
var query string

cmd := &cobra.Command{
    // ...
}

// Use short flags for common options
cmd.Flags().StringVarP(&query, "query", "q", "", "Filter by search query")

// Boolean flags
cmd.Flags().BoolVar(&override, "override", false, "Override existing resource")
```

### Output

Always support both table and JSON output:

```go
if globalFlags.Output == "json" {
    return printJSON(data)
}

// Table output
w := tabWriter()
fmt.Fprintln(w, "COLUMN1\tCOLUMN2")
for _, item := range items {
    fmt.Fprintf(w, "%s\t%s\n", item.Field1, item.Field2)
}
w.Flush()
```

## Helper Functions

### Output Helpers (helpers.go)

| Function | Purpose |
|----------|---------|
| `printJSON(value any)` | Pretty-prints any value as indented JSON |
| `tabWriter()` | Creates a `tabwriter.Writer` for aligned table output |
| `stringify(value any)` | Converts any value to string for table cells |
| `toPrettyString(value any)` | Like stringify but with indentation for complex types |
| `validateOutputFormat()` | Validates `globalFlags.Output` is "table" or "json" |

### Client (client.go)

| Function | Purpose |
|----------|---------|
| `NewClient()` | Creates SDK client with resolved config (flags > env > file) |
| `formatSDKError(err)` | Extracts meaningful message from SDK errors |

## Code Style

### Error Messages

Start with lowercase, no trailing period:

```go
return errors.New("namespace is required when flow-id is provided")
return fmt.Errorf("failed to read file '%s': %w", filepath, err)
```

### Success Messages

Confirm what happened:

```go
fmt.Println("Secret created successfully!")
fmt.Printf("Key: %s\n", key)
```

### Table Headers

Use uppercase:

```go
fmt.Fprintln(w, "KEY\tNAMESPACE\tCREATED")
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./src/cli/...

# Run specific test
go test -v ./src/cli/... -run TestSecretsListCommand
```

## Checklist for New Commands

- [ ] Command file created (`src/cli/{resource}.go`)
- [ ] Parent command with `Use` and `Short`
- [ ] Subcommands with `Use`, `Short`, `Long`, `Example`, `Aliases`
- [ ] Commands validate args with `cobra.ExactArgs()` or similar
- [ ] Commands call `validateOutputFormat()` first
- [ ] Commands create client with `NewClient()`
- [ ] Business logic in separate `run*()` functions
- [ ] Commands support both table and JSON output
- [ ] Command registered in `root.go`
- [ ] Unit tests for argument validation
- [ ] Tests for pure functions
