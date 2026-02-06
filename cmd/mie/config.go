//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultConfigDir  = ".mie"
	defaultConfigFile = "config.yaml"
	configVersion     = "1"
)

// Config represents the .mie/config.yaml configuration file.
type Config struct {
	Version   string          `yaml:"version"`
	Storage   StorageConfig   `yaml:"storage"`
	Embedding EmbeddingConfig `yaml:"embedding"`
}

// StorageConfig contains storage backend configuration.
type StorageConfig struct {
	Engine string `yaml:"engine"` // mem, sqlite, rocksdb
	Path   string `yaml:"path"`   // Auto: ~/.mie/data/default/
}

// EmbeddingConfig contains embedding provider configuration.
type EmbeddingConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Provider   string `yaml:"provider"`   // ollama, openai, nomic, mock
	BaseURL    string `yaml:"base_url"`
	Model      string `yaml:"model"`
	Dimensions int    `yaml:"dimensions"` // 768 for nomic, 1536 for openai
	APIKey     string `yaml:"api_key,omitempty"`
	Workers    int    `yaml:"workers"`
}

// DefaultConfig returns a config with sensible defaults for local development.
func DefaultConfig() *Config {
	return &Config{
		Version: configVersion,
		Storage: StorageConfig{
			Engine: "rocksdb",
			Path:   "", // resolved at runtime to ~/.mie/data/default/
		},
		Embedding: EmbeddingConfig{
			Enabled:    true,
			Provider:   "ollama",
			BaseURL:    getEnv("OLLAMA_HOST", "http://localhost:11434"),
			Model:      getEnv("OLLAMA_EMBED_MODEL", "nomic-embed-text"),
			Dimensions: 768,
			Workers:    4,
		},
	}
}

// LoadConfig loads configuration from the specified path or finds it automatically.
//
// If configPath is empty, it searches for .mie/config.yaml in the current directory
// and parent directories. The MIE_CONFIG_PATH environment variable can override the
// search path.
//
// After loading, environment variables are applied to override file-based configuration.
func LoadConfig(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = os.Getenv("MIE_CONFIG_PATH")
	}
	if configPath == "" {
		var err error
		configPath, err = findConfigFile()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(configPath) //nolint:gosec // G304: Path comes from user config or discovery
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", configPath, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config format in %s: %w", configPath, err)
	}

	if cfg.Version != configVersion {
		return nil, fmt.Errorf("unsupported config version %q (expected %q), run 'mie init --force' to regenerate", cfg.Version, configVersion)
	}

	cfg.applyEnvOverrides()

	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ValidateConfig checks that the configuration values are valid.
func ValidateConfig(cfg *Config) error {
	switch cfg.Storage.Engine {
	case "mem", "sqlite", "rocksdb":
		// valid
	default:
		return fmt.Errorf("unsupported storage engine %q (supported: mem, sqlite, rocksdb)", cfg.Storage.Engine)
	}
	return nil
}

// SaveConfig writes the configuration to the specified path as YAML.
func SaveConfig(cfg *Config, configPath string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("cannot encode config: %w", err)
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("cannot create config directory %s: %w", dir, err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("cannot write config file %s: %w", configPath, err)
	}

	return nil
}

// ConfigPath returns the path to the config file in the given directory.
func ConfigPath(dir string) string {
	return filepath.Join(dir, defaultConfigDir, defaultConfigFile)
}

// DefaultDataDir returns the default data directory for MIE storage.
func DefaultDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(homeDir, ".mie", "data", "default"), nil
}

// ResolveDataDir returns the effective data directory from config.
// If config path is empty, uses the default ~/.mie/data/default/.
func ResolveDataDir(cfg *Config) (string, error) {
	if cfg.Storage.Path != "" {
		return filepath.Dir(cfg.Storage.Path), nil
	}
	return DefaultDataDir()
}

// ResolveStoragePath returns the effective storage path from config.
// For sqlite, appends "index.db" to the data directory.
// For rocksdb and mem, the data directory itself is the path.
func ResolveStoragePath(cfg *Config) (string, error) {
	if cfg.Storage.Path != "" {
		return cfg.Storage.Path, nil
	}
	dataDir, err := DefaultDataDir()
	if err != nil {
		return "", err
	}
	if cfg.Storage.Engine == "sqlite" {
		return filepath.Join(dataDir, "index.db"), nil
	}
	return dataDir, nil
}

// findConfigFile searches for .mie/config.yaml in current and parent directories.
func findConfigFile() (string, error) {
	if configPath := os.Getenv("MIE_CONFIG_PATH"); configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
		return "", fmt.Errorf("MIE_CONFIG_PATH is set to %q but the file does not exist", configPath)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot access working directory: %w", err)
	}

	for {
		configPath := ConfigPath(dir)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("no .mie/config.yaml found in current directory or any parent directory; run 'mie init' to create one")
}

// applyEnvOverrides applies environment variable overrides to the configuration.
func (c *Config) applyEnvOverrides() {
	// Storage overrides
	if v := os.Getenv("MIE_STORAGE_ENGINE"); v != "" {
		c.Storage.Engine = v
	}
	if v := os.Getenv("MIE_STORAGE_PATH"); v != "" {
		c.Storage.Path = v
	}

	// Embedding overrides
	if v := os.Getenv("MIE_EMBEDDING_ENABLED"); v != "" {
		c.Embedding.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("MIE_EMBEDDING_PROVIDER"); v != "" {
		c.Embedding.Provider = v
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		c.Embedding.BaseURL = v
	}
	if v := os.Getenv("OLLAMA_EMBED_MODEL"); v != "" {
		c.Embedding.Model = v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		c.Embedding.APIKey = v
		if c.Embedding.Provider == "ollama" {
			c.Embedding.Provider = "openai"
		}
	}
	if v := os.Getenv("NOMIC_API_KEY"); v != "" {
		c.Embedding.APIKey = v
		if c.Embedding.Provider == "ollama" {
			c.Embedding.Provider = "nomic"
		}
	}

}

// getEnv retrieves an environment variable or returns a fallback value if not set.
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
