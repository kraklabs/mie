// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import "context"

// Querier is the interface that MIE tools use to interact with the memory graph.
// It abstracts over the memory.Writer, memory.Reader, and memory.ConflictDetector
// so that tools can be tested with a mock implementation.
type Querier interface {
	// Write operations
	StoreFact(ctx context.Context, req StoreFactRequest) (*Fact, error)
	StoreDecision(ctx context.Context, req StoreDecisionRequest) (*Decision, error)
	StoreEntity(ctx context.Context, req StoreEntityRequest) (*Entity, error)
	StoreEvent(ctx context.Context, req StoreEventRequest) (*Event, error)
	StoreTopic(ctx context.Context, req StoreTopicRequest) (*Topic, error)
	InvalidateFact(ctx context.Context, oldFactID, newFactID, reason string) error
	AddRelationship(ctx context.Context, edgeType string, fields map[string]string) error

	// Read operations
	SemanticSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error)
	ExactSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error)
	GetNodeByID(ctx context.Context, nodeID string) (any, error)
	ListNodes(ctx context.Context, opts ListOptions) ([]any, int, error)

	// Graph traversal
	GetRelatedEntities(ctx context.Context, factID string) ([]Entity, error)
	GetFactsAboutEntity(ctx context.Context, entityID string) ([]Fact, error)
	GetDecisionEntities(ctx context.Context, decisionID string) ([]EntityWithRole, error)
	GetInvalidationChain(ctx context.Context, factID string) ([]Invalidation, error)
	GetRelatedFacts(ctx context.Context, entityID string) ([]Fact, error)
	GetEntityDecisions(ctx context.Context, entityID string) ([]Decision, error)

	// Update operations
	UpdateDescription(ctx context.Context, nodeID, newDescription string) error
	UpdateStatus(ctx context.Context, nodeID, newStatus string) error

	// Conflict detection
	DetectConflicts(ctx context.Context, opts ConflictOptions) ([]Conflict, error)
	CheckNewFactConflicts(ctx context.Context, content, category string) ([]Conflict, error)

	// Stats and export
	GetStats(ctx context.Context) (*GraphStats, error)
	ExportGraph(ctx context.Context, opts ExportOptions) (*ExportData, error)

	// Metrics
	IncrementCounter(ctx context.Context, key string) error

	// Configuration
	EmbeddingsEnabled() bool
}

// --- Request types ---

// StoreFactRequest contains parameters for storing a fact.
type StoreFactRequest struct {
	Content            string  `json:"content"`
	Category           string  `json:"category"`
	Confidence         float64 `json:"confidence"`
	SourceAgent        string  `json:"source_agent"`
	SourceConversation string  `json:"source_conversation"`
}

// StoreDecisionRequest contains parameters for storing a decision.
type StoreDecisionRequest struct {
	Title              string `json:"title"`
	Rationale          string `json:"rationale"`
	Alternatives       string `json:"alternatives"`
	Context            string `json:"context"`
	SourceAgent        string `json:"source_agent"`
	SourceConversation string `json:"source_conversation"`
}

// StoreEntityRequest contains parameters for storing an entity.
type StoreEntityRequest struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	SourceAgent string `json:"source_agent"`
}

// StoreEventRequest contains parameters for storing an event.
type StoreEventRequest struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	EventDate          string `json:"event_date"`
	SourceAgent        string `json:"source_agent"`
	SourceConversation string `json:"source_conversation"`
}

// StoreTopicRequest contains parameters for storing a topic.
type StoreTopicRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// --- Node types ---

// Fact represents a personal truth or piece of knowledge.
type Fact struct {
	ID                 string  `json:"id"`
	Content            string  `json:"content"`
	Category           string  `json:"category"`
	Confidence         float64 `json:"confidence"`
	SourceAgent        string  `json:"source_agent"`
	SourceConversation string  `json:"source_conversation"`
	Valid              bool    `json:"valid"`
	CreatedAt          int64   `json:"created_at"`
	UpdatedAt          int64   `json:"updated_at"`
}

// Decision represents a choice with rationale.
type Decision struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Rationale          string `json:"rationale"`
	Alternatives       string `json:"alternatives"`
	Context            string `json:"context"`
	SourceAgent        string `json:"source_agent"`
	SourceConversation string `json:"source_conversation"`
	Status             string `json:"status"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

// Entity represents a person, company, project, or technology.
type Entity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	SourceAgent string `json:"source_agent"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// Event represents a timestamped occurrence.
type Event struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	EventDate          string `json:"event_date"`
	SourceAgent        string `json:"source_agent"`
	SourceConversation string `json:"source_conversation"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

// Topic represents a recurring theme.
type Topic struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// EntityWithRole is an entity with its role in a decision.
type EntityWithRole struct {
	Entity
	Role string `json:"role"`
}

// Invalidation tracks when a fact supersedes another.
type Invalidation struct {
	NewFactID  string `json:"new_fact_id"`
	OldFactID  string `json:"old_fact_id"`
	Reason     string `json:"reason"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}

// --- Search and query types ---

// SearchResult represents a single result from semantic or exact search.
type SearchResult struct {
	NodeType string      `json:"node_type"`
	ID       string      `json:"id"`
	Content  string      `json:"content"`
	Detail   string      `json:"detail"`
	Distance float64     `json:"distance"`
	Metadata any `json:"metadata"`
}

// ListOptions configures listing of nodes.
type ListOptions struct {
	NodeType  string `json:"node_type"`
	Category  string `json:"category"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	TopicName string `json:"topic_name"`
	ValidOnly bool   `json:"valid_only"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// --- Conflict types ---

// Conflict represents two potentially contradicting facts.
type Conflict struct {
	FactA      Fact    `json:"fact_a"`
	FactB      Fact    `json:"fact_b"`
	Similarity float64 `json:"similarity"`
}

// ConflictOptions configures conflict detection.
type ConflictOptions struct {
	Category  string  `json:"category"`
	Threshold float64 `json:"threshold"`
	Limit     int     `json:"limit"`
}

// --- Stats and export types ---

// GraphStats contains memory graph statistics.
type GraphStats struct {
	TotalFacts       int    `json:"total_facts"`
	ValidFacts       int    `json:"valid_facts"`
	InvalidatedFacts int    `json:"invalidated_facts"`
	TotalDecisions   int    `json:"total_decisions"`
	ActiveDecisions  int    `json:"active_decisions"`
	TotalEntities    int    `json:"total_entities"`
	TotalEvents      int    `json:"total_events"`
	TotalTopics      int    `json:"total_topics"`
	TotalEdges       int    `json:"total_edges"`
	TotalQueries     int    `json:"total_queries"`
	TotalStores      int    `json:"total_stores"`
	LastQueryAt      int64  `json:"last_query_at,omitempty"`
	LastStoreAt      int64  `json:"last_store_at,omitempty"`
	SchemaVersion    string `json:"schema_version"`
	StorageEngine    string `json:"storage_engine"`
	StoragePath      string `json:"storage_path"`
}

// ExportOptions configures graph export.
type ExportOptions struct {
	Format            string   `json:"format"`
	IncludeEmbeddings bool     `json:"include_embeddings"`
	NodeTypes         []string `json:"node_types"`
}

// ExportData contains the full graph export.
type ExportData struct {
	Version    string                 `json:"version"`
	ExportedAt string                 `json:"exported_at"`
	Stats      map[string]int         `json:"stats"`
	Facts      []Fact                 `json:"facts,omitempty"`
	Decisions  []Decision             `json:"decisions,omitempty"`
	Entities   []Entity               `json:"entities,omitempty"`
	Events     []Event                `json:"events,omitempty"`
	Topics     []Topic                `json:"topics,omitempty"`
	Edges      map[string]any `json:"relationships,omitempty"`
}
