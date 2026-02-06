//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "1", cfg.Version)
	assert.Equal(t, "rocksdb", cfg.Storage.Engine)
	assert.Empty(t, cfg.Storage.Path, "default path should be empty (resolved at runtime)")

	assert.True(t, cfg.Embedding.Enabled)
	assert.Equal(t, "ollama", cfg.Embedding.Provider)
	assert.Equal(t, 768, cfg.Embedding.Dimensions)
	assert.Equal(t, 4, cfg.Embedding.Workers)
	assert.NotEmpty(t, cfg.Embedding.BaseURL)
	assert.NotEmpty(t, cfg.Embedding.Model)

}

func TestConfigEnvOverrides(t *testing.T) {
	t.Setenv("MIE_STORAGE_ENGINE", "rocksdb")

	cfg := DefaultConfig()
	cfg.applyEnvOverrides()

	assert.Equal(t, "rocksdb", cfg.Storage.Engine)
}

func TestConfigEnvOverridesEmbedding(t *testing.T) {
	t.Setenv("MIE_EMBEDDING_ENABLED", "false")
	t.Setenv("MIE_EMBEDDING_PROVIDER", "openai")

	cfg := DefaultConfig()
	cfg.applyEnvOverrides()

	assert.False(t, cfg.Embedding.Enabled)
	assert.Equal(t, "openai", cfg.Embedding.Provider)
}

func TestConfigEnvOverridesStoragePath(t *testing.T) {
	t.Setenv("MIE_STORAGE_PATH", "/custom/path/data.db")

	cfg := DefaultConfig()
	cfg.applyEnvOverrides()

	assert.Equal(t, "/custom/path/data.db", cfg.Storage.Path)
}

func TestConfigYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	yaml := `version: "1"
storage:
  engine: rocksdb
  path: /tmp/test.db
embedding:
  enabled: false
  provider: openai
  base_url: https://api.openai.com
  model: text-embedding-3-small
  dimensions: 1536
  workers: 2
`
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0600))

	// Set the env var so LoadConfig finds our file
	t.Setenv("MIE_CONFIG_PATH", configPath)

	cfg, err := LoadConfig("")
	require.NoError(t, err)

	assert.Equal(t, "1", cfg.Version)
	assert.Equal(t, "rocksdb", cfg.Storage.Engine)
	assert.Equal(t, "/tmp/test.db", cfg.Storage.Path)

	assert.False(t, cfg.Embedding.Enabled)
	assert.Equal(t, "openai", cfg.Embedding.Provider)
	assert.Equal(t, "https://api.openai.com", cfg.Embedding.BaseURL)
	assert.Equal(t, "text-embedding-3-small", cfg.Embedding.Model)
	assert.Equal(t, 1536, cfg.Embedding.Dimensions)
	assert.Equal(t, 2, cfg.Embedding.Workers)

}

func TestConfigYAMLInvalidVersion(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	yaml := `version: "999"
storage:
  engine: sqlite
`
	require.NoError(t, os.WriteFile(configPath, []byte(yaml), 0600))

	t.Setenv("MIE_CONFIG_PATH", configPath)

	_, err := LoadConfig("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported config version")
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath("/home/user")
	assert.Equal(t, filepath.Join("/home/user", ".mie", "config.yaml"), path)
}

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".mie", "config.yaml")

	cfg := DefaultConfig()
	require.NoError(t, SaveConfig(cfg, configPath))

	// Verify file exists
	_, err := os.Stat(configPath)
	require.NoError(t, err)

	// Reload and verify
	t.Setenv("MIE_CONFIG_PATH", configPath)
	loaded, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, cfg.Version, loaded.Version)
	assert.Equal(t, cfg.Storage.Engine, loaded.Storage.Engine)
}