package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyronn/agent-cli-starter/internal/buildinfo"
)

func TestConfigLifecycle(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	lookup := func(string) (string, bool) { return "", false }

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

	lookup := func(string) (string, bool) { return "", false }
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

	stdout, stderr, code := runCLI(t, func(string) (string, bool) { return "", false }, "--config", "does-not-matter", "--json", "version")
	if code != 0 || stderr != "" || !strings.Contains(stdout, `"version": "test"`) {
		t.Fatalf("version: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestUsageErrorsUseExitCodeTwo(t *testing.T) {
	t.Parallel()

	lookup := func(string) (string, bool) { return "", false }
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
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute(context.Background(), args, &stdout, &stderr, Options{
		Build:     buildinfo.Info{Version: "test", Commit: "abc", Date: "today"},
		LookupEnv: lookup,
	})
	return stdout.String(), stderr.String(), code
}
