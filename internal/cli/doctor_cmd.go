package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/example/agent-cli-starter/internal/output"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func newDoctorCommand(current *state, options Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Inspect the local agentctl setup without exposing secrets",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			configStatus := "ok"
			configDetail := current.configPath
			if _, err := os.Stat(current.configPath); err != nil {
				if os.IsNotExist(err) {
					configStatus = "info"
					configDetail += " (not created; defaults are active)"
				} else {
					configStatus = "warn"
					configDetail += fmt.Sprintf(" (%v)", err)
				}
			}
			tokenStatus := "info"
			tokenDetail := "AGENTCTL_TOKEN is not set"
			if value, ok := options.LookupEnv("AGENTCTL_TOKEN"); ok && value != "" {
				tokenStatus = "ok"
				tokenDetail = "AGENTCTL_TOKEN is set (value redacted)"
			}

			checks := []doctorCheck{
				{Name: "platform", Status: "ok", Detail: runtime.GOOS + "/" + runtime.GOARCH},
				{Name: "config", Status: configStatus, Detail: configDetail},
				{Name: "endpoint", Status: "ok", Detail: current.values.Endpoint},
				{Name: "credentials", Status: tokenStatus, Detail: tokenDetail},
			}
			if current.output == "json" {
				return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: map[string]any{
					"checks": checks,
				}})
			}
			for _, check := range checks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-12s %-5s %s\n", check.Name, check.Status, check.Detail); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
