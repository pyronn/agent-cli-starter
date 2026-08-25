package cli

import (
	"fmt"

	"github.com/pyronn/agent-cli-starter/internal/buildinfo"
	"github.com/pyronn/agent-cli-starter/internal/output"
	"github.com/spf13/cobra"
)

func newVersionCommand(current *state, info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:         "version",
		Short:       "Print version and build metadata",
		Args:        noArgs,
		Annotations: map[string]string{skipConfig: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: info})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s version %s (commit %s, built %s)\n", appName, info.Version, info.Commit, info.Date)
			return err
		},
	}
}
