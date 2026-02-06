// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

//go:build cozodb

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/kraklabs/mie/pkg/storage"
	"github.com/kraklabs/mie/pkg/tools"
)

// ClientConfig holds configuration for creating a memory Client.
type ClientConfig struct {
	DataDir             string
	StorageEngine       string
	EmbeddingEnabled    bool
	EmbeddingProvider   string
	EmbeddingBaseURL    string
	EmbeddingModel      string
	EmbeddingAPIKey     string
	EmbeddingDimensions int
	EmbeddingWorkers    int
}

// Client provides access to the MIE memory graph.
// It implements tools.Querier so it can be used by MCP tool handlers.
type Client struct {
	backend  storage.Backend
	config   ClientConfig
	writer   *Writer
	reader   *Reader
	detector *ConflictDetector
	embedder *EmbeddingGenerator
	logger   *slog.Logger
}

// Ensure Client implements tools.Querier at compile time.
var _ tools.Querier = (*Client)(nil)

// NewClient creates a new memory Client backed by CozoDB.
func NewClient(cfg ClientConfig) (*Client, error) {
	return NewClientWithLogger(cfg, nil)
}

// NewClientWithLogger creates a new memory Client with a custom logger.
func NewClientWithLogger(cfg ClientConfig, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	backend, err := storage.NewEmbeddedBackend(storage.EmbeddedConfig{
		DataDir:             cfg.DataDir,
		Engine:              cfg.StorageEngine,
		EmbeddingDimensions: cfg.EmbeddingDimensions,
	})
	if err != nil {
		return nil, err
	}

	// Apply storage-level schema (mie_meta only)
	if err := backend.EnsureSchema(); err != nil {
		_ = backend.Close()
		return nil, err
	}

	// Apply full MIE memory schema
	dim := cfg.EmbeddingDimensions
	if dim <= 0 {
		dim = 768
	}
	if err := EnsureSchema(backend, dim); err != nil {
		_ = backend.Close()
		return nil, err
	}

	// Create HNSW indexes for semantic search if embeddings are enabled
	if cfg.EmbeddingEnabled {
		if err := EnsureHNSWIndexes(backend, dim); err != nil {
			_ = backend.Close()
			return nil, err
		}
	}

	// Set up embedding provider if enabled
	var embedder *EmbeddingGenerator
	if cfg.EmbeddingEnabled && cfg.EmbeddingProvider != "" {
		provider, err := CreateEmbeddingProvider(
			cfg.EmbeddingProvider,
			cfg.EmbeddingAPIKey,
			cfg.EmbeddingBaseURL,
			cfg.EmbeddingModel,
			logger,
		)
		if err != nil {
			logger.Warn("failed to create embedding provider, continuing without embeddings", "error", err)
		} else {
			embedder = NewEmbeddingGenerator(provider, logger)
		}
	}

	writer := NewWriter(backend, embedder, logger)
	reader := NewReader(backend, embedder, logger)
	detector := NewConflictDetector(backend, embedder, logger)

	return &Client{
		backend:  backend,
		config:   cfg,
		writer:   writer,
		reader:   reader,
		detector: detector,
		embedder: embedder,
		logger:   logger,
	}, nil
}

// Close releases resources held by the Client.
func (c *Client) Close() error {
	return c.backend.Close()
}

// RawQuery executes a raw CozoScript query against the database.
func (c *Client) RawQuery(ctx context.Context, script string) (*storage.QueryResult, error) {
	return c.backend.Query(ctx, script)
}

// EmbeddingsEnabled reports whether embedding support is configured.
func (c *Client) EmbeddingsEnabled() bool {
	return c.config.EmbeddingEnabled && c.embedder != nil
}

// --- tools.Querier write operations ---

func (c *Client) StoreFact(ctx context.Context, req tools.StoreFactRequest) (*tools.Fact, error) {
	return c.writer.StoreFact(ctx, req)
}

func (c *Client) StoreDecision(ctx context.Context, req tools.StoreDecisionRequest) (*tools.Decision, error) {
	return c.writer.StoreDecision(ctx, req)
}

func (c *Client) StoreEntity(ctx context.Context, req tools.StoreEntityRequest) (*tools.Entity, error) {
	return c.writer.StoreEntity(ctx, req)
}

func (c *Client) StoreEvent(ctx context.Context, req tools.StoreEventRequest) (*tools.Event, error) {
	return c.writer.StoreEvent(ctx, req)
}

func (c *Client) StoreTopic(ctx context.Context, req tools.StoreTopicRequest) (*tools.Topic, error) {
	return c.writer.StoreTopic(ctx, req)
}

func (c *Client) InvalidateFact(ctx context.Context, oldFactID, newFactID, reason string) error {
	return c.writer.InvalidateFact(ctx, oldFactID, newFactID, reason)
}

func (c *Client) AddRelationship(ctx context.Context, edgeType string, fields map[string]string) error {
	return c.writer.AddRelationship(ctx, edgeType, fields)
}

// --- tools.Querier read operations ---

func (c *Client) SemanticSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]tools.SearchResult, error) {
	return c.reader.SemanticSearch(ctx, query, nodeTypes, limit)
}

func (c *Client) ExactSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]tools.SearchResult, error) {
	return c.reader.ExactSearch(ctx, query, nodeTypes, limit)
}

func (c *Client) GetNodeByID(ctx context.Context, nodeID string) (any, error) {
	return c.reader.GetNodeByID(ctx, nodeID)
}

func (c *Client) ListNodes(ctx context.Context, opts tools.ListOptions) ([]any, int, error) {
	return c.reader.ListNodes(ctx, opts)
}

// --- tools.Querier graph traversal ---

func (c *Client) GetRelatedEntities(ctx context.Context, factID string) ([]tools.Entity, error) {
	return c.reader.GetRelatedEntities(ctx, factID)
}

func (c *Client) GetFactsAboutEntity(ctx context.Context, entityID string) ([]tools.Fact, error) {
	return c.reader.GetFactsAboutEntity(ctx, entityID)
}

func (c *Client) GetDecisionEntities(ctx context.Context, decisionID string) ([]tools.EntityWithRole, error) {
	return c.reader.GetDecisionEntities(ctx, decisionID)
}

func (c *Client) GetInvalidationChain(ctx context.Context, factID string) ([]tools.Invalidation, error) {
	return c.reader.GetInvalidationChain(ctx, factID)
}

func (c *Client) GetRelatedFacts(ctx context.Context, entityID string) ([]tools.Fact, error) {
	return c.reader.GetRelatedFacts(ctx, entityID)
}

func (c *Client) GetEntityDecisions(ctx context.Context, entityID string) ([]tools.Decision, error) {
	return c.reader.GetEntityDecisions(ctx, entityID)
}

// --- tools.Querier update operations ---

func (c *Client) UpdateDescription(ctx context.Context, nodeID, newDescription string) error {
	return c.writer.UpdateDescription(ctx, nodeID, newDescription)
}

func (c *Client) UpdateStatus(ctx context.Context, nodeID, newStatus string) error {
	return c.writer.UpdateStatus(ctx, nodeID, newStatus)
}

// --- tools.Querier conflict detection ---

func (c *Client) DetectConflicts(ctx context.Context, opts tools.ConflictOptions) ([]tools.Conflict, error) {
	return c.detector.DetectConflicts(ctx, opts)
}

func (c *Client) CheckNewFactConflicts(ctx context.Context, content, category string) ([]tools.Conflict, error) {
	return c.detector.CheckNewFactConflicts(ctx, content, category)
}

// --- tools.Querier stats and export ---

func (c *Client) GetStats(ctx context.Context) (*tools.GraphStats, error) {
	stats, err := c.reader.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	stats.StorageEngine = c.config.StorageEngine
	stats.StoragePath = c.config.DataDir
	return stats, nil
}

func (c *Client) ExportGraph(ctx context.Context, opts tools.ExportOptions) (*tools.ExportData, error) {
	return c.reader.ExportGraph(ctx, opts)
}

// IncrementCounter atomically increments a counter in mie_meta and updates
// the corresponding last_*_at timestamp.
func (c *Client) IncrementCounter(ctx context.Context, key string) error {
	// Read current value.
	readScript := fmt.Sprintf(`?[value] := *mie_meta{key: '%s', value}`, escapeDatalog(key))
	result, err := c.backend.Query(ctx, readScript)

	current := 0
	if err == nil && len(result.Rows) > 0 {
		if v, parseErr := strconv.Atoi(toString(result.Rows[0][0])); parseErr == nil {
			current = v
		}
	}

	// Write incremented value.
	next := strconv.Itoa(current + 1)
	writeScript := fmt.Sprintf(
		`?[key, value] <- [['%s', '%s']] :put mie_meta {key => value}`,
		escapeDatalog(key), next,
	)
	if err := c.backend.Execute(ctx, writeScript); err != nil {
		return fmt.Errorf("increment counter %s: %w", key, err)
	}

	// Update the corresponding timestamp.
	tsKey := ""
	switch key {
	case "total_queries":
		tsKey = "last_query_at"
	case "total_stores":
		tsKey = "last_store_at"
	}
	if tsKey != "" {
		now := strconv.FormatInt(time.Now().Unix(), 10)
		tsScript := fmt.Sprintf(
			`?[key, value] <- [['%s', '%s']] :put mie_meta {key => value}`,
			tsKey, now,
		)
		// Best-effort: ignore timestamp write errors.
		_ = c.backend.Execute(ctx, tsScript)
	}

	return nil
}