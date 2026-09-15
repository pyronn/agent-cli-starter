package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyronn/agent-cli-starter/internal/buildinfo"
	"github.com/pyronn/agent-cli-starter/internal/update"
)

func TestConfigLifecycle(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	lookup := denyEnv

	stdout, stderr, code := runCLI(t, lookup, "--config", configPath, "config", "set", "timeout", "45s")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Set timeout=45s") {
		t.Fatalf("set: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, lookup, "--config", configPath, "config", "get", "timeout")
	if code != 0 || stderr != "" || stdout != "45s\n" {
		t.Fatalf("get: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, lookup, "--config", configPath, "--json", "config", "list", "--effective")
	if code != 0 || stderr != "" {
		t.Fatalf("list: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var payload struct {
		Data struct {
			Items []struct {
				Key    string `json:"key"`
				Source string `json:"source"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode list JSON: %v\n%s", err, stdout)
	}
	if len(payload.Data.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(payload.Data.Items))
	}
}

func TestExampleJSONAndStructuredError(t *testing.T) {
	t.Parallel()

	lookup := denyEnv
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	stdout, stderr, code := runCLI(t, lookup, "--config", configPath, "--json", "example", "echo", "hello", "--upper")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"message": "HELLO"`) {
		t.Fatalf("echo: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, lookup, "--json", "config", "get", "missing")
	if code != 2 || stdout != "" || !strings.Contains(stderr, `"code": "usage_error"`) {
		t.Fatalf("error: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestVersionDoesNotRequireValidConfig(t *testing.T) {
	t.Parallel()

	stdout, stderr, code := runCLI(t, denyEnv, "--config", "does-not-matter", "--json", "version")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"version": "test"`) {
		t.Fatalf("version: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestUsageErrorsUseExitCodeTwo(t *testing.T) {
	t.Parallel()

	lookup := denyEnv
	tests := [][]string{
		{"--json", "config", "set", "only-one-argument"},
		{"--json", "unknown-command"},
		{"--json", "--output", "xml", "version"},
	}
	for _, args := range tests {
		_, stderr, code := runCLI(t, lookup, args...)
		if code != 2 || !strings.Contains(stderr, `"code": "usage_error"`) {
			t.Fatalf("%v: code=%d stderr=%q", args, code, stderr)
		}
	}
}

func runCLI(t *testing.T, lookup func(string) (string, bool), args ...string) (string, string, int) {
	t.Helper()
	return runCLIWithUpdater(t, lookup, &fakeUpdater{}, args...)
}

func runCLIWithUpdater(t *testing.T, lookup func(string) (string, bool), updater Updater, args ...string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute(context.Background(), args, &stdout, &stderr, Options{
		Build:     buildinfo.Info{Version: "test", Commit: "abc", Date: "today"},
		LookupEnv: lookup,
		Updater:   updater,
	})
	return stdout.String(), stderr.String(), code
}

type fakeUpdater struct {
	status     update.Status
	checkError error
	result     update.Result
	installErr error
	checks     []bool
	installs   []string
}

func (f *fakeUpdater) Check(ctx context.Context, useCache bool) (update.Status, error) {
	f.checks = append(f.checks, useCache)
	if f.checkError != nil {
		return update.Status{}, f.checkError
	}
	if f.status.Current == "" {
		f.status.Current = "test"
	}
	return f.status, nil
}

func (f *fakeUpdater) Install(ctx context.Context, version string, progress io.Writer) (update.Result, error) {
	f.installs = append(f.installs, version)
	if progress != nil {
		fmt.Fprintf(progress, "downloading %s\n", version)
	}
	if f.installErr != nil {
		return update.Result{}, f.installErr
	}
	if f.result.Version == "" {
		f.result = update.Result{
			Previous: f.status.Current,
			Version:  version,
			Path:     filepath.Join("tmp", appName),
			Updated:  true,
		}
	}
	return f.result, nil
}

func denyEnv(string) (string, bool) { return "", false }

func TestUpdateCheckReportsAvailableRelease(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{
		Current:    "v1.2.0",
		Latest:     "v1.3.0",
		Available:  true,
		ReleaseURL: "https://example.test/releases/tag/v1.3.0",
	}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--json", "update", "--check")
	if code != 0 || stderr != "" {
		t.Fatalf("update --check: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var payload struct {
		Data struct {
			Current    string `json:"current"`
			Latest     string `json:"latest"`
			Available  bool   `json:"available"`
			Updated    bool   `json:"updated"`
			ReleaseURL string `json:"release_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode update JSON: %v\n%s", err, stdout)
	}
	if !payload.Data.Available || payload.Data.Latest != "v1.3.0" || payload.Data.Current != "v1.2.0" {
		t.Fatalf("payload = %+v", payload.Data)
	}
	if payload.Data.Updated {
		t.Fatal("update --check must not install anything")
	}
	if payload.Data.ReleaseURL != "https://example.test/releases/tag/v1.3.0" {
		t.Fatalf("release_url = %q", payload.Data.ReleaseURL)
	}
	if len(updater.installs) != 0 {
		t.Fatalf("installs = %v, want none", updater.installs)
	}
	if len(updater.checks) != 1 || updater.checks[0] {
		t.Fatalf("checks = %v, want one uncached check", updater.checks)
	}
}

func TestUpdateInstallsLatestRelease(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "update")
	if code != 0 {
		t.Fatalf("update: code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, "Updated agentctl to v1.3.0 (was v1.2.0)") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "downloading v1.3.0") {
		t.Fatalf("stderr = %q, want download progress", stderr)
	}
	if strings.Contains(stderr, "A new version") {
		t.Fatalf("stderr = %q, want no duplicate notice for the update command", stderr)
	}
	if len(updater.installs) != 1 || updater.installs[0] != "v1.3.0" {
		t.Fatalf("installs = %v", updater.installs)
	}
	if len(updater.checks) != 1 {
		t.Fatalf("checks = %v, want the command's own check only", updater.checks)
	}
}

func TestUpdateInstallIsQuietInJSONMode(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--json", "update")
	if code != 0 || stderr != "" {
		t.Fatalf("update --json: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var payload struct {
		Data struct {
			Updated bool   `json:"updated"`
			Version string `json:"version"`
			Path    string `json:"path"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode update JSON: %v\n%s", err, stdout)
	}
	if !payload.Data.Updated || payload.Data.Version != "v1.3.0" || payload.Data.Path == "" {
		t.Fatalf("payload = %+v", payload.Data)
	}
}

func TestUpdateReportsUpToDate(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.3.0", Latest: "v1.3.0"}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "update")
	if code != 0 || stderr != "" {
		t.Fatalf("update: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "agentctl is already up to date (v1.3.0)") {
		t.Fatalf("stdout = %q", stdout)
	}
	if len(updater.installs) != 0 {
		t.Fatalf("installs = %v, want none", updater.installs)
	}
}

func TestUpdateInstallsExplicitVersion(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "update", "--version", "v1.0.0")
	if code != 0 {
		t.Fatalf("update --version: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Updated agentctl to v1.0.0") {
		t.Fatalf("stdout = %q", stdout)
	}
	if len(updater.installs) != 1 || updater.installs[0] != "v1.0.0" {
		t.Fatalf("installs = %v", updater.installs)
	}
	if len(updater.checks) != 0 {
		t.Fatalf("checks = %v, want no version check for an explicit version", updater.checks)
	}
}

func TestUpdateDoesNotRequireValidConfig(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--config", "does-not-matter", "--json", "update", "--check")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"available": true`) {
		t.Fatalf("update: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestUpdateRejectsCheckWithVersion(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{}
	_, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--json", "update", "--check", "--version", "v1.0.0")
	if code != 2 || !strings.Contains(stderr, `"code": "usage_error"`) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if len(updater.installs) != 0 || len(updater.checks) != 0 {
		t.Fatalf("checks = %v installs = %v, want no work", updater.checks, updater.installs)
	}
}

func TestUpdateFailureUsesRuntimeExitCode(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{checkError: errUpdateUnavailable}
	_, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--json", "update", "--check")
	if code != 1 || !strings.Contains(stderr, `"code": "runtime_error"`) {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

var errUpdateUnavailable = errors.New("release host unreachable")

func TestAutoCheckNotifiesInTextMode(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "version")
	if code != 0 || !strings.Contains(stdout, "version test") {
		t.Fatalf("version: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "A new version of agentctl is available: v1.3.0 (current v1.2.0)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if len(updater.checks) != 1 || !updater.checks[0] {
		t.Fatalf("checks = %v, want one cached check", updater.checks)
	}
}

func TestAutoCheckIsSilentForJSONOutput(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--json", "version")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"version": "test"`) {
		t.Fatalf("version: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(updater.checks) != 0 {
		t.Fatalf("checks = %v, want no update check in JSON mode", updater.checks)
	}
}

func TestAutoCheckIsSilentWhenUpToDate(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.3.0", Latest: "v1.3.0"}}
	_, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "version")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestAutoCheckSkippedForHelp(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	stdout, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage:") {
		t.Fatalf("--help: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(updater.checks) != 0 {
		t.Fatalf("checks = %v, want none for help output", updater.checks)
	}
}

func TestAutoCheckDisabledByEnvironment(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	lookup := func(name string) (string, bool) {
		if name == noUpdateCheckEnv {
			return "1", true
		}
		return "", false
	}
	_, stderr, code := runCLIWithUpdater(t, lookup, updater, "version")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if len(updater.checks) != 0 {
		t.Fatalf("checks = %v, want none", updater.checks)
	}
}

func TestAutoCheckDisabledByFlag(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	_, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--no-update-check", "version")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if len(updater.checks) != 0 {
		t.Fatalf("checks = %v, want none", updater.checks)
	}
}

func TestAutoCheckDoesNotRunAfterFailure(t *testing.T) {
	t.Parallel()

	updater := &fakeUpdater{status: update.Status{Current: "v1.2.0", Latest: "v1.3.0", Available: true}}
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLIWithUpdater(t, denyEnv, updater, "--config", configPath, "config", "get", "endpoint")
	if code != 1 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if len(updater.checks) != 0 {
		t.Fatalf("checks = %v, want none after a failed command", updater.checks)
	}
}
