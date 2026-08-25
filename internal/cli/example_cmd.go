package cli

import (
	"fmt"

	"github.com/pyronn/agent-cli-starter/internal/output"
	"github.com/spf13/cobra"
)

func newExampleCommand(current *state, options Options) *cobra.Command {
	command := &cobra.Command{
		Use:   "example",
		Short: "Example commands to replace with your domain",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	var upper bool
	echo := &cobra.Command{
		Use:   "echo MESSAGE",
		Short: "Demonstrate a testable service command",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := options.Echo.Echo(cmd.Context(), args[0], upper)
			if err != nil {
				return runtimeError(err)
			}
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: result})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return err
		},
	}
	echo.Flags().BoolVar(&upper, "upper", false, "convert the message to uppercase")
	command.AddCommand(echo)
	return command
}
