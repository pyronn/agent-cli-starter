package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	KeyEndpoint = "endpoint"
	KeyTimeout  = "timeout"
	KeyOutput   = "output"
)

var keys = []string{KeyEndpoint, KeyTimeout, KeyOutput}

type spec struct {
	Default string
	Env     string
}

var specs = map[string]spec{
	KeyEndpoint: {Default: "https://api.example.com", Env: "AGENTCTL_ENDPOINT"},
	KeyTimeout:  {Default: "30s", Env: "AGENTCTL_TIMEOUT"},
	KeyOutput:   {Default: "text", Env: "AGENTCTL_OUTPUT"},
}

// File is the persisted user configuration. Pointers distinguish an unset
// value from a value explicitly stored by the user.
type File struct {
	Endpoint *string `yaml:"endpoint,omitempty"`
	Timeout  *string `yaml:"timeout,omitempty"`
	Output   *string `yaml:"output,omitempty"`
}

// Values is the typed, effective configuration used by commands.
type Values struct {
	Endpoint string
	Timeout  time.Duration
	Output   string
}

// Entry describes a configuration value and where it came from.
type Entry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// DefaultPath returns the platform-native configuration path.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(dir, "agentctl", "config.yaml"), nil
}

func Keys() []string {
	return append([]string(nil), keys...)
}

func IsKey(key string) bool {
	_, ok := specs[key]
	return ok
}

func EnvName(key string) string {
	if item, ok := specs[key]; ok {
		return item.Env
	}
	return ""
}

func (f File) Get(key string) (string, bool) {
	var value *string
	switch key {
	case KeyEndpoint:
		value = f.Endpoint
	case KeyTimeout:
		value = f.Timeout
	case KeyOutput:
		value = f.Output
	default:
		return "", false
	}
	if value == nil {
		return "", false
	}
	return *value, true
}

func (f *File) Set(key, value string) error {
	normalized, err := normalize(key, value)
	if err != nil {
		return err
	}
	switch key {
	case KeyEndpoint:
		f.Endpoint = &normalized
	case KeyTimeout:
		f.Timeout = &normalized
	case KeyOutput:
		f.Output = &normalized
	default:
		return unknownKeyError(key)
	}
	return nil
}

func (f *File) Unset(key string) error {
	switch key {
	case KeyEndpoint:
		f.Endpoint = nil
	case KeyTimeout:
		f.Timeout = nil
	case KeyOutput:
		f.Output = nil
	default:
		return unknownKeyError(key)
	}
	return nil
}

func (f File) Entries() []Entry {
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		if value, ok := f.Get(key); ok {
			entries = append(entries, Entry{Key: key, Value: value, Source: "config"})
		}
	}
	return entries
}

// Resolve applies defaults, file values, environment variables, and flag
// overrides in increasing priority order.
func Resolve(file File, lookupEnv func(string) (string, bool), overrides map[string]string) (Values, []Entry, error) {
	resolved := make(map[string]Entry, len(keys))
	for _, key := range keys {
		resolved[key] = Entry{Key: key, Value: specs[key].Default, Source: "default"}
	}

	for _, key := range keys {
		if value, ok := file.Get(key); ok {
			normalized, err := normalize(key, value)
			if err != nil {
				return Values{}, nil, fmt.Errorf("%s from config file: %w", key, err)
			}
			resolved[key] = Entry{Key: key, Value: normalized, Source: "config"}
		}
	}

	if lookupEnv != nil {
		for _, key := range keys {
			if value, ok := lookupEnv(specs[key].Env); ok {
				normalized, err := normalize(key, value)
				if err != nil {
					return Values{}, nil, fmt.Errorf("%s from %s: %w", key, specs[key].Env, err)
				}
				resolved[key] = Entry{Key: key, Value: normalized, Source: "environment"}
			}
		}
	}

	for key, value := range overrides {
		if !IsKey(key) {
			return Values{}, nil, unknownKeyError(key)
		}
		normalized, err := normalize(key, value)
		if err != nil {
			return Values{}, nil, fmt.Errorf("%s from command line: %w", key, err)
		}
		resolved[key] = Entry{Key: key, Value: normalized, Source: "flag"}
	}

	timeout, _ := time.ParseDuration(resolved[KeyTimeout].Value)
	values := Values{
		Endpoint: resolved[KeyEndpoint].Value,
		Timeout:  timeout,
		Output:   resolved[KeyOutput].Value,
	}
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, resolved[key])
	}
	return values, entries, nil
}

func normalize(key, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch key {
	case KeyEndpoint:
		parsed, err := url.ParseRequestURI(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", fmt.Errorf("must be an absolute URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return "", fmt.Errorf("URL scheme must be http or https")
		}
		return strings.TrimRight(value, "/"), nil
	case KeyTimeout:
		duration, err := time.ParseDuration(value)
		if err != nil {
			return "", fmt.Errorf("must be a Go duration such as 30s or 2m: %w", err)
		}
		if duration <= 0 {
			return "", fmt.Errorf("must be greater than zero")
		}
		return duration.String(), nil
	case KeyOutput:
		switch strings.ToLower(value) {
		case "text", "json":
			return strings.ToLower(value), nil
		default:
			return "", fmt.Errorf("must be text or json")
		}
	default:
		return "", unknownKeyError(key)
	}
}

func unknownKeyError(key string) error {
	return fmt.Errorf("unknown configuration key %q (valid keys: %s)", key, strings.Join(keys, ", "))
}
