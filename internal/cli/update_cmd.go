package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pyronn/agent-cli-starter/internal/output"
	"github.com/pyronn/agent-cli-starter/internal/update"
	"github.com/spf13/cobra"
)

const (
	// skipUpdateCheck marks commands that should not trigger the automatic
	// update check, either because they perform their own check or because
	// their output must stay clean.
	skipUpdateCheck = "skip-update-check"
	// noUpdateCheckEnv disables the automatic update check for every command.
	noUpdateCheckEnv = "AGENTCTL_NO_UPDATE_CHECK"
	// autoCheckTimeout bounds the automatic update check so a slow or
	// unreachable release host cannot stall a command.
	autoCheckTimeout = 3 * time.Second
)

// Updater is the subset of the update service the CLI depends on. Tests
// substitute it to avoid network access.
type Updater interface {
	Check(ctx context.Context, useCache bool) (update.Status, error)
	Install(ctx context.Context, version string, progress io.Writer) (update.Result, error)
}

// updateReport is the stable JSON payload for `update` and `update --check`.
type updateReport struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Available  bool   `json:"available"`
	Updated    bool   `json:"updated"`
	Previous   string `json:"previous,omitempty"`
	Version    string `json:"version,omitempty"`
	Path       string `json:"path,omitempty"`
	ReleaseURL string `json:"release_url,omitempty"`
}

func newUpdateCommand(current *state, options Options) *cobra.Command {
	var (
		checkOnly bool
		version   string
	)
	command := &cobra.Command{
		Use:         "update",
		Short:       "Update agentctl to the latest release",
		Args:        noArgs,
		Annotations: map[string]string{skipConfig: "true", skipUpdateCheck: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if checkOnly && version != "" {
				return usageError(fmt.Errorf("--check cannot be combined with --version"))
			}
			jsonMode := current.output == "json"
			// Progress would corrupt the structured stderr contract, so JSON
			// mode stays silent and reports the outcome in stdout only.
			progress := io.Writer(io.Discard)
			if !jsonMode {
				progress = cmd.ErrOrStderr()
			}
			if version != "" {
				result, err := options.Updater.Install(cmd.Context(), version, progress)
				if err != nil {
					return runtimeError(fmt.Errorf("install %s: %w", version, err))
				}
				return writeUpdateReport(cmd, jsonMode, updateReport{
					Current:   result.Previous,
					Latest:    result.Version,
					Available: true,
					Updated:   result.Updated,
					Previous:  result.Previous,
					Version:   result.Version,
					Path:      result.Path,
				}, false)
			}

			status, err := options.Updater.Check(cmd.Context(), false)
			if err != nil {
				return runtimeError(fmt.Errorf("check latest version: %w", err))
			}
			report := updateReport{
				Current:    status.Current,
				Latest:     status.Latest,
				Available:  status.Available,
				ReleaseURL: status.ReleaseURL,
			}
			if checkOnly || !status.Available {
				return writeUpdateReport(cmd, jsonMode, report, checkOnly)
			}

			result, err := options.Updater.Install(cmd.Context(), status.Latest, progress)
			if err != nil {
				return runtimeError(fmt.Errorf("install %s: %w", status.Latest, err))
			}
			report.Updated = result.Updated
			report.Previous = result.Previous
			report.Version = result.Version
			report.Path = result.Path
			return writeUpdateReport(cmd, jsonMode, report, false)
		},
	}
	command.Flags().BoolVar(&checkOnly, "check", false, "only report whether a newer release exists")
	command.Flags().StringVar(&version, "version", "", "install a specific release tag, for example v1.2.3")
	return command
}

func writeUpdateReport(cmd *cobra.Command, jsonMode bool, report updateReport, checkOnly bool) error {
	if jsonMode {
		return output.JSON(cmd.OutOrStdout(), output.Envelope{Data: report})
	}
	writer := cmd.OutOrStdout()
	switch {
	case report.Updated:
		_, err := fmt.Fprintf(writer, "Updated %s to %s (was %s): %s\n",
			appName, report.Version, report.Previous, report.Path)
		return err
	case report.Available && checkOnly:
		if _, err := fmt.Fprintf(writer, "A new version of %s is available: %s (current %s).\n",
			appName, report.Latest, report.Current); err != nil {
			return err
		}
		_, err := fmt.Fprintf(writer, "Run \"%s update\" to install it.\n", appName)
		return err
	default:
		installed := report.Latest
		if installed == "" {
			installed = report.Current
		}
		_, err := fmt.Fprintf(writer, "%s is already up to date (%s).\n", appName, installed)
		return err
	}
}

// notifyAvailableUpdate prints a short notice to stderr when a newer release
// exists. It never runs in JSON mode so structured stdout and stderr stay
// machine-readable, and it never fails the command it follows.
func notifyAvailableUpdate(ctx context.Context, options Options, current *state, stderr io.Writer) {
	if !current.ran || current.skipUpdateCheck || current.output == "json" {
		return
	}
	if envTruthy(options.LookupEnv, noUpdateCheckEnv) {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, autoCheckTimeout)
	defer cancel()
	status, err := options.Updater.Check(checkCtx, true)
	if err != nil || !status.Available {
		return
	}
	fmt.Fprintf(stderr, "\nA new version of %s is available: %s (current %s).\nRun \"%s update\" to install it.\n",
		appName, status.Latest, status.Current, appName)
}

func envTruthy(lookup func(string) (string, bool), name string) bool {
	if lookup == nil {
		return false
	}
	value, ok := lookup(name)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
