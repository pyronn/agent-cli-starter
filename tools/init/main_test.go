package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveEnvPrefix(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"acmectl":    "ACMECTL",
		"acme-agent": "ACME_AGENT",
		"a1-cli":     "A1_CLI",
	}
	for input, expected := range tests {
		if actual := deriveEnvPrefix(input); actual != expected {
			t.Errorf("deriveEnvPrefix(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestBuildAndApplyPlan(t *testing.T) {
	t.Parallel()
	root := createFixture(t)
	current, err := discoverProject(root)
	if err != nil {
		t.Fatal(err)
	}
	options := settings{
		root:        root,
		name:        "acme-cli",
		module:      "github.com/acme/acme-cli",
		envPrefix:   "ACME_CLI",
		description: `Manage Acme "$5" resources`,
	}
	if err := validateSettings(options); err != nil {
		t.Fatal(err)
	}

	plan, err := buildPlan(root, current, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.edits) == 0 {
		t.Fatal("buildPlan returned no edits")
	}
	if err := applyPlan(plan); err != nil {
		t.Fatal(err)
	}

	assertContains(t, filepath.Join(root, "go.mod"), "module github.com/acme/acme-cli")
	assertContains(t, filepath.Join(root, "internal", "config", "config.go"), `"ACME_CLI_ENDPOINT"`)
	assertContains(t, filepath.Join(root, "internal", "cli", "root.go"), `appName = "acme-cli"`)
	assertContains(t, filepath.Join(root, "internal", "cli", "root.go"), `appDescription = "Manage Acme \"$5\" resources"`)
	assertContains(t, filepath.Join(root, "README.md"), "# acme-cli\n\nManage Acme \"$5\" resources")
	if _, err := os.Stat(filepath.Join(root, "cmd", "acme-cli", "main.go")); err != nil {
		t.Fatalf("renamed command entry does not exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "agentctl")); !os.IsNotExist(err) {
		t.Fatalf("old command directory still exists or stat failed: %v", err)
	}
}

func TestValidateSettings(t *testing.T) {
	t.Parallel()
	valid := settings{
		name:        "my-cli",
		module:      "github.com/acme/my-cli",
		envPrefix:   "MY_CLI",
		description: "My CLI",
	}
	if err := validateSettings(valid); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	for name, mutate := range map[string]func(*settings){
		"name":        func(value *settings) { value.name = "Bad Name" },
		"module":      func(value *settings) { value.module = "github.com/acme/my cli" },
		"env-prefix":  func(value *settings) { value.envPrefix = "my-cli" },
		"description": func(value *settings) { value.description = "line one\nline two" },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if err := validateSettings(value); err == nil {
				t.Fatalf("invalid %s accepted", name)
			}
		})
	}
}

func TestReplaceIdentityPreservesRequestedModule(t *testing.T) {
	t.Parallel()
	current := projectState{
		name:      "oldctl",
		module:    "github.com/acme/oldctl",
		envPrefix: "OLDCTL",
	}
	options := settings{
		name:      "newctl",
		module:    "github.com/acme/oldctl-compatible",
		envPrefix: "NEWCTL",
	}
	input := []byte("github.com/acme/oldctl/internal/cli OLDCTL_ENDPOINT oldctl")
	actual := string(replaceIdentity(input, current, options))
	expected := "github.com/acme/oldctl-compatible/internal/cli NEWCTL_ENDPOINT newctl"
	if actual != expected {
		t.Fatalf("replaceIdentity() = %q, want %q", actual, expected)
	}
}

func createFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module github.com/example/agent-cli-starter\n\ngo 1.26.0\n",
		filepath.Join("cmd", "agentctl", "main.go"): `package main

import "github.com/example/agent-cli-starter/internal/cli"

func main() { _ = cli.Execute }
`,
		filepath.Join("internal", "cli", "root.go"): `package cli

const (
	appName = "agentctl"
	appDescription = "A production-minded starter for agent-facing CLI tools"
)
`,
		filepath.Join("internal", "config", "config.go"): `package config

const endpoint = "AGENTCTL_ENDPOINT"
`,
		"README.md": `# Agent CLI Starter

Template description.

Run agentctl.
`,
	}
	for path, contents := range files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func assertContains(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), expected) {
		t.Fatalf("%s does not contain %q:\n%s", path, expected, data)
	}
}
