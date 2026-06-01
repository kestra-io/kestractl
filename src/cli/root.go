package cli

import (
	"errors"
	"fmt"
	"github.com/spf13/pflag"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

// Flag names
const (
	FlagHost     = "host"
	FlagToken    = "token"
	FlagUsername = "username"
	FlagPassword = "password"
	FlagTenant   = "tenant"
	FlagOutput   = "output"
	FlagConfig   = "config"
	FlagVerbose  = "verbose"
	FlagHeader   = "header"
)

// AnnotationOffline can be set in a command's Annotations to indicate that it
// runs entirely offline and does not require a running Kestra instance.
// Commands with this annotation skip config initialization in PersistentPreRunE.
//
// Usage:
//
//	Annotations: map[string]string{AnnotationOffline: "true"}
const AnnotationOffline = "offline"

// GlobalFlags holds flags that are available to all commands
type GlobalFlags struct {
	Host     string
	Token    string
	Username string
	Password string
	Tenant   string
	Output   string
	Headers  []string
}

var globalFlags GlobalFlags

// NewRootCommand builds the root CLI command.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "kestractl",
		Short: "kestractl - Manage flows, namespaces, and executions",
		Long: `kestractl is a command-line tool for managing Kestra workflows.

It provides commands to manage flows, namespaces, and executions,
with support for multiple authentication contexts and output formats.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Annotations[AnnotationOffline] == "true" {
				return nil
			}
			if err := initializeConfig(cmd); err != nil {
				return err
			}
			initializeTelemetry()
			if verbose, _ := cmd.Flags().GetBool(FlagVerbose); verbose == true {
				if viper.ConfigFileUsed() != "" {
					fmt.Printf("config location: %s\n", viper.ConfigFileUsed())
				}
				fmt.Printf("resolved params: \n")
				cmd.Flags().VisitAll(func(flag *pflag.Flag) {
					v := flag.Value.String()
					// Mask any credential-bearing flag, not just the global
					// token/password ones (e.g. users --user-password).
					name := strings.ToLower(flag.Name)
					if v != "" && (strings.Contains(name, "token") || strings.Contains(name, "password")) {
						v = "XXX"
					}
					fmt.Printf("\t%s: %s\n", flag.Name, v)
				})
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Add persistent flags available to all subcommands
	root.PersistentFlags().StringVar(&globalFlags.Host, FlagHost, "", "Kestra host URL")
	root.PersistentFlags().StringVarP(&globalFlags.Token, FlagToken, "t", "", "API token")
	root.PersistentFlags().StringVarP(&globalFlags.Username, FlagUsername, "", "", "basic auth username")
	root.PersistentFlags().StringVarP(&globalFlags.Password, FlagPassword, "", "", "basic auth password")
	root.PersistentFlags().StringVar(&globalFlags.Tenant, FlagTenant, "", "Tenant name")
	root.PersistentFlags().StringVarP(&globalFlags.Output, FlagOutput, "o", "table", "Output format (table or json)")
	root.PersistentFlags().String(FlagConfig, "", "config file (default is $HOME/.kestractl/config.yaml)")
	root.PersistentFlags().BoolP(FlagVerbose, "v", false, "verbose output (warning: it will print credentials in http requests")
	root.PersistentFlags().StringArray(FlagHeader, nil, "Extra HTTP header to include in all requests (format: 'Key:Value', repeatable)")

	root.AddCommand(newVersionCommand())
	root.AddCommand(newConfigCommand())
	root.AddCommand(newFlowsCommand())
	root.AddCommand(newNamespacesCommand())
	root.AddCommand(newNamespaceFilesCommand())
	root.AddCommand(newKVCommand())
	root.AddCommand(newExecutionsCommand())
	root.AddCommand(newWorkersCommand())
	root.AddCommand(newPluginsCommand())
	root.AddCommand(newUsersCommand())
	root.AddCommand(newGroupsCommand())

	return root
}

// initializeConfig sets up Viper to handle configuration from multiple sources.
func initializeConfig(cmd *cobra.Command) error {
	// 1. Set up Viper to use environment variables
	viper.SetEnvPrefix("KESTRACTL")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// 2. Handle the configuration file
	cfgFile := cmd.Flag(FlagConfig).Value.String()
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for a config file in default locations
		home, err := os.UserHomeDir()
		if err != nil {
			// If we can't get home dir, only search current directory
			viper.AddConfigPath(".")
		} else {
			viper.AddConfigPath(home + "/.kestractl")
			viper.AddConfigPath(".")
		}
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	// 3. Read the configuration file
	// It's okay if the config file doesn't exist
	if err := viper.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return fmt.Errorf("error reading config file: %w", err)
		}
	}

	// 4. Bind Cobra flags to Viper
	// This ensures flags have the highest priority
	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	// 5. Read from the default context if config file was loaded
	// Map nested context values to top-level for Viper
	if viper.ConfigFileUsed() != "" {
		defaultContext := viper.GetString("default_context")
		if defaultContext != "" {
			// Set defaults from the context (lowest priority)
			if !viper.IsSet(FlagHost) {
				viper.SetDefault(FlagHost, viper.GetString("contexts."+defaultContext+".host"))
			}
			if !viper.IsSet(FlagTenant) {
				viper.SetDefault(FlagTenant, viper.GetString("contexts."+defaultContext+".tenant"))
			}
			if !viper.IsSet(FlagToken) {
				viper.SetDefault(FlagToken, viper.GetString("contexts."+defaultContext+".token"))
			}
			if !viper.IsSet(FlagUsername) {
				viper.SetDefault(FlagUsername, viper.GetString("contexts."+defaultContext+".username"))
			}
			if !viper.IsSet(FlagPassword) {
				viper.SetDefault(FlagPassword, viper.GetString("contexts."+defaultContext+".password"))
			}
			if !viper.IsSet(FlagHeader) {
				viper.SetDefault(FlagHeader, viper.GetStringSlice("contexts."+defaultContext+".headers"))
			}
		}
	}

	// 6. Sync Viper values back to globalFlags for backward compatibility
	globalFlags.Host = viper.GetString(FlagHost)
	globalFlags.Token = viper.GetString(FlagToken)
	globalFlags.Username = viper.GetString(FlagUsername)
	globalFlags.Password = viper.GetString(FlagPassword)
	globalFlags.Tenant = viper.GetString(FlagTenant)
	globalFlags.Output = viper.GetString(FlagOutput)
	// cmd.Flags() excludes root's own persistent flags, so BindPFlags misses FlagHeader.
	// Read directly from the persistent flagset when explicitly set by the user.
	if f := cmd.Root().PersistentFlags().Lookup(FlagHeader); f != nil && f.Changed {
		globalFlags.Headers, _ = cmd.Root().PersistentFlags().GetStringArray(FlagHeader)
	} else {
		globalFlags.Headers = viper.GetStringSlice(FlagHeader)
	}

	return nil
}

// Execute runs the CLI.
func Execute() error {
	root := NewRootCommand()
	start := time.Now()

	executedCommand, err := root.ExecuteC()

	commandPath := root.CommandPath()
	if executedCommand != nil {
		commandPath = executedCommand.CommandPath()
	}

	activeTelemetry.CaptureCommand(commandPath, err, time.Since(start))
	activeTelemetry.Close()
	activeTelemetry = noopTelemetry{}

	return err
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show CLI version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("kestractl v%s\n", version)
			if commit != "none" {
				fmt.Printf("commit: %s\n", commit)
			}
			if buildDate != "unknown" {
				fmt.Printf("built: %s\n", buildDate)
			}
		},
	}
}
