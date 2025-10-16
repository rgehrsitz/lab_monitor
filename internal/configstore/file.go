package configstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"labmonitor/internal/config"
)

// FileStore persists configuration to a YAML file on disk.
// It is intended for local development and backwards compatibility
// with the Raspberry Pi deployment.
type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore {
	return &FileStore{path: path}
}

func (s *FileStore) Get(ctx context.Context) (config.Config, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reuse existing loader to validate configuration.
	cfg, err := config.Load(s.path)
	if err != nil {
		return config.Config{}, "", err
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("read config: %w", err)
	}
	return cfg, computeVersion(data), nil
}

func (s *FileStore) Put(ctx context.Context, cfg config.Config, version string) error {
	if version == "" {
		return errors.New("version required for config update")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Read existing file to verify version.
	data, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read config for update: %w", err)
	}
	currentVersion := computeVersion(data)
	if currentVersion != version {
		return ErrConflict{Message: "configuration updated by another process"}
	}

	// Validate configuration before writing.
	if err := cfg.Validate(); err != nil {
		return err
	}

	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(s.path, encoded, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// computeVersion returns a SHA-256 hex digest for optimistic concurrency.
func computeVersion(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
