package cli

import (
	"fmt"

	"github.com/pyronn/agent-cli-starter/internal/config"
	"github.com/pyronn/agent-cli-starter/internal/output"
	"github.com/spf13/cobra"
)

func newConfigCommand(current *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Read and write agentctl configuration",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(
		newConfigListCommand(current),
		newConfigGetCommand(current),
		newConfigSetCommand(current),
		newConfigUnsetCommand(current),
		newConfigPathCommand(current),
	)
	return command
}

func newConfigListCommand(current *state) *cobra.Command {
	var effective bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List stored configuration values",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := current.file.Entries()
			if effective {
				entries = current.entries
			}
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: map[string]any{
					"config_path": current.configPath,
					"effective":   effective,
					"items":       entries,
				}})
			}
			if len(entries) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No configuration values are stored.")
				return err
			}
			for _, entry := range entries {
				if effective {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\t(source: %s)\n", entry.Key, entry.Value, entry.Source); err != nil {
						return err
					}
				} else if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", entry.Key, entry.Value); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&effective, "effective", false, "show effective values and their sources")
	return command
}

func newConfigGetCommand(current *state) *cobra.Command {
	var effective bool
	command := &cobra.Command{
		Use:   "get KEY",
		Short: "Get one configuration value",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsKey(key) {
				return usageError(fmt.Errorf("unknown configuration key %q", key))
			}

			var entry config.Entry
			found := false
			if effective {
				for _, item := range current.entries {
					if item.Key == key {
						entry, found = item, true
						break
					}
				}
			} else if value, ok := current.file.Get(key); ok {
				entry, found = config.Entry{Key: key, Value: value, Source: "config"}, true
			}
			if !found {
				return newExitError(1, "not_found", fmt.Sprintf("configuration key %q is not set", key), nil)
			}

			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: entry})
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), entry.Value)
			return err
		},
	}
	command.Flags().BoolVar(&effective, "effective", false, "include environment, flags, and defaults")
	return command
}

func newConfigSetCommand(current *state) *cobra.Command {
	return &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Persist one configuration value",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			file := current.file
			if err := file.Set(key, value); err != nil {
				return usageError(err)
			}
			if err := (config.Store{Path: current.configPath}).Save(file); err != nil {
				return configError(err)
			}
			stored, _ := file.Get(key)
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: config.Entry{
					Key: key, Value: stored, Source: "config",
				}})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Set %s=%s\n", key, stored)
			return err
		},
	}
}

func newConfigUnsetCommand(current *state) *cobra.Command {
	return &cobra.Command{
		Use:   "unset KEY",
		Short: "Remove one stored configuration value",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !config.IsKey(key) {
				return usageError(fmt.Errorf("unknown configuration key %q", key))
			}
			if _, ok := current.file.Get(key); !ok {
				return newExitError(1, "not_found", fmt.Sprintf("configuration key %q is not set", key), nil)
			}
			file := current.file
			if err := file.Unset(key); err != nil {
				return usageError(err)
			}
			if err := (config.Store{Path: current.configPath}).Save(file); err != nil {
				return configError(err)
			}
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: map[string]string{
					"key": key, "status": "unset",
				}})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Unset %s\n", key)
			return err
		},
	}
}

func newConfigPathCommand(current *state) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the active configuration file path",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: map[string]string{
					"path": current.configPath,
				}})
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), current.configPath)
			return err
		},
	}
}
