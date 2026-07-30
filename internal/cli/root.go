package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/example/agent-cli-starter/internal/buildinfo"
	"github.com/example/agent-cli-starter/internal/config"
	"github.com/example/agent-cli-starter/internal/output"
	"github.com/example/agent-cli-starter/internal/service"
	"github.com/spf13/cobra"
)

const (
	appName        = "agentctl"
	appDescription = "A production-minded starter for agent-facing CLI tools"
	skipConfig     = "skip-config"
)

type Options struct {
	Build     buildinfo.Info
	LookupEnv func(string) (string, bool)
	Echo      service.Echoer
}

type state struct {
	configPath string
	file       config.File
	values     config.Values
	entries    []config.Entry
	output     string
}

type flags struct {
	configPath string
	endpoint   string
	timeout    string
	output     string
	json       bool
}

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer, options Options) int {
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}
	if options.Echo == nil {
		options.Echo = service.EchoService{}
	}
	command, current := newRootCommand(options)
	command.SetArgs(args)
	command.SetOut(stdout)
	command.SetErr(stderr)

	err := command.ExecuteContext(ctx)
	if err == nil {
		return 0
	}

	result := &exitError{
		exitCode: 2,
		code:     "usage_error",
		message:  err.Error(),
		cause:    err,
	}
	var typed *exitError
	if errors.As(err, &typed) {
		result = typed
	}

	if current.output == "json" || wantsJSON(args) {
		_ = output.JSON(stderr, output.ErrorEnvelope{
			Error: output.ErrorBody{Code: result.code, Message: result.message},
		})
	} else {
		_, _ = fmt.Fprintf(stderr, "error: %s\n", result.message)
	}
	return result.exitCode
}

func newRootCommand(options Options) (*cobra.Command, *state) {
	current := &state{output: "text"}
	var rootFlags flags

	root := &cobra.Command{
		Use:           appName,
		Short:         appDescription,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Annotations[skipConfig] == "true" {
				mode, err := outputFromFlags(rootFlags)
				if err != nil {
					return usageError(err)
				}
				current.output = mode
				return nil
			}

			path := rootFlags.configPath
			if path == "" {
				var err error
				path, err = config.DefaultPath()
				if err != nil {
					return configError(err)
				}
			}
			file, err := (config.Store{Path: path}).Load()
			if err != nil {
				return configError(err)
			}

			overrides := make(map[string]string)
			if cmd.Flags().Changed("endpoint") {
				overrides[config.KeyEndpoint] = rootFlags.endpoint
			}
			if cmd.Flags().Changed("timeout") {
				overrides[config.KeyTimeout] = rootFlags.timeout
			}
			if cmd.Flags().Changed("output") {
				overrides[config.KeyOutput] = rootFlags.output
			}
			if rootFlags.json {
				if value, ok := overrides[config.KeyOutput]; ok && value != "json" {
					return usageError(fmt.Errorf("--json cannot be combined with --output=%s", value))
				}
				overrides[config.KeyOutput] = "json"
			}

			values, entries, err := config.Resolve(file, options.LookupEnv, overrides)
			if err != nil {
				return configError(err)
			}
			current.configPath = path
			current.file = file
			current.values = values
			current.entries = entries
			current.output = values.Output
			return nil
		},
	}
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(err)
	})

	root.PersistentFlags().StringVar(&rootFlags.configPath, "config", "", "configuration file path")
	root.PersistentFlags().StringVar(&rootFlags.endpoint, "endpoint", "", "API endpoint (overrides environment and file)")
	root.PersistentFlags().StringVar(&rootFlags.timeout, "timeout", "", "request timeout, for example 30s or 2m")
	root.PersistentFlags().StringVarP(&rootFlags.output, "output", "o", "", "output format: text or json")
	root.PersistentFlags().BoolVar(&rootFlags.json, "json", false, "shorthand for --output=json")

	root.AddCommand(
		newConfigCommand(current),
		newDoctorCommand(current, options),
		newExampleCommand(current, options),
		newVersionCommand(current, options.Build),
	)
	return root, current
}

func outputFromFlags(rootFlags flags) (string, error) {
	value := strings.ToLower(rootFlags.output)
	if rootFlags.json {
		if value != "" && value != "json" {
			return "", fmt.Errorf("--json cannot be combined with --output=%s", rootFlags.output)
		}
		return "json", nil
	}
	switch value {
	case "", "text":
		return "text", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("--output must be text or json")
	}
}

func wantsJSON(args []string) bool {
	for index, arg := range args {
		if arg == "--json" || arg == "--output=json" || arg == "-o=json" {
			return true
		}
		if (arg == "--output" || arg == "-o") && index+1 < len(args) && args[index+1] == "json" {
			return true
		}
	}
	return false
}

func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return usageError(err)
	}
	return nil
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(count)(cmd, args); err != nil {
			return usageError(err)
		}
		return nil
	}
}
