package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration and authentication",
	}

	cmd.AddCommand(newConfigShowCommand())
	cmd.AddCommand(newConfigAddCommand())
	cmd.AddCommand(newConfigRemoveCommand())
	cmd.AddCommand(newConfigUseCommand())

	return cmd
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := NewAuthManager("")
			contexts, defaultName, err := manager.ListContexts()
			if err != nil {
				return err
			}

			if len(contexts) == 0 {
				fmt.Println("No authentication contexts configured.")
				fmt.Println("Use 'kestractl config add' to add a new context.")
				return nil
			}

			fmt.Println("Current Configuration:")
			if defaultName == "" {
				fmt.Println("Default context: None")
			} else {
				fmt.Printf("Default context: %s\n", defaultName)
			}
			fmt.Println()

			names := make([]string, 0, len(contexts))
			for name := range contexts {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				ctx := contexts[name]
				status := " "
				if name == defaultName {
					status = "*"
				}
				fmt.Printf("%s %s: %s (tenant: %s)\n", status, name, ctx.Host, ctx.Tenant)
			}

			return nil
		},
	}
}

func newConfigAddCommand() *cobra.Command {
	var token string
	var username string
	var password string
	var setDefault bool

	cmd := &cobra.Command{
		Use:   "add <name> <host> <tenant>",
		Short: "Add a new authentication context.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 3 {
				return errors.New("requires name, host, and tenant arguments")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name, host, tenant := args[0], normalizeHost(args[1]), args[2]

			var authMethod string
			if strings.TrimSpace(token) != "" {
				authMethod = "token"
			} else if strings.TrimSpace(username) != "" && strings.TrimSpace(password) != "" {
				authMethod = "basicAuth"
			} else {
				return errors.New("at least token or username+password is required (use --token or --username and --password)")
			}

			manager := NewAuthManager("")
			ctx := AuthContext{
				Name:       name,
				Host:       host,
				Tenant:     tenant,
				AuthMethod: authMethod,
				Token:      token,
				Username:   username,
				Password:   password,
			}

			if err := manager.AddContext(ctx); err != nil {
				return err
			}

			if setDefault {
				if err := manager.SetDefaultContext(name); err != nil {
					return err
				}
				fmt.Printf("Context '%s' added and set as default.\n", name)
			} else {
				fmt.Printf("Context '%s' added.\n", name)
			}
			fmt.Printf("Host: %s\n", host)
			fmt.Printf("Tenant: %s\n", tenant)
			fmt.Println("Token: [REDACTED]")

			return nil
		},
	}

	cmd.Flags().StringVarP(&token, FlagToken, "t", "", "API token")
	cmd.Flags().StringVarP(&username, FlagUsername, "", "", "basic auth username")
	cmd.Flags().StringVarP(&password, FlagPassword, "", "", "basic auth password")
	cmd.Flags().BoolVar(&setDefault, "default", false, "Set as default context")

	return cmd
}

func newConfigRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an authentication context.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			manager := NewAuthManager("")
			if err := manager.DeleteContext(name); err != nil {
				return err
			}

			fmt.Printf("Context '%s' removed.\n", name)
			return nil
		},
	}
}

func newConfigUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Set a context as the default.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			manager := NewAuthManager("")
			if err := manager.SetDefaultContext(name); err != nil {
				return err
			}

			fmt.Printf("Default context set to '%s'.\n", name)
			return nil
		},
	}
}
