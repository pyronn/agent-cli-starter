package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePrecedence(t *testing.T) {
	t.Parallel()

	file := File{}
	if err := file.Set(KeyEndpoint, "https://from-file.example/"); err != nil {
		t.Fatal(err)
	}
	environment := map[string]string{
		"AGENTCTL_ENDPOINT": "https://from-env.example",
		"AGENTCTL_TIMEOUT":  "45s",
	}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}
	values, entries, err := Resolve(file, lookup, map[string]string{
		KeyEndpoint: "https://from-flag.example/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if values.Endpoint != "https://from-flag.example" {
		t.Fatalf("Endpoint = %q, want flag value", values.Endpoint)
	}
	if values.Timeout != 45*time.Second {
		t.Fatalf("Timeout = %s, want 45s", values.Timeout)
	}
	if values.Output != "text" {
		t.Fatalf("Output = %q, want default text", values.Output)
	}

	sources := make(map[string]string)
	for _, entry := range entries {
		sources[entry.Key] = entry.Source
	}
	if sources[KeyEndpoint] != "flag" || sources[KeyTimeout] != "environment" || sources[KeyOutput] != "default" {
		t.Fatalf("unexpected sources: %#v", sources)
	}
}

func TestStoreRoundTripAndUnknownField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	store := Store{Path: path}
	file := File{}
	if err := file.Set(KeyTimeout, "2m"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(file); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := loaded.Get(KeyTimeout)
	if !ok || value != "2m0s" {
		t.Fatalf("loaded timeout = %q, %v; want 2m0s, true", value, ok)
	}

	if err := os.WriteFile(path, []byte("unknown: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load() accepted an unknown field, want error")
	}
}

func TestSetRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key   string
		value string
	}{
		{KeyEndpoint, "relative/path"},
		{KeyTimeout, "never"},
		{KeyOutput, "xml"},
		{"missing", "value"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.key, func(t *testing.T) {
			t.Parallel()
			file := File{}
			if err := file.Set(test.key, test.value); err == nil {
				t.Fatalf("Set(%q, %q) succeeded, want error", test.key, test.value)
			}
		})
	}
}
