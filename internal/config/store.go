package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Store struct {
	Path string
}

func (s Store) Load() (File, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("read %s: %w", s.Path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return File{}, nil
	}

	var file File
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("parse %s: %w", s.Path, err)
	}
	return file, nil
}

func (s Store) Save(file File) error {
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create configuration directory %s: %w", dir, err)
	}
	if err := os.WriteFile(s.Path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", s.Path, err)
	}
	return nil
}
