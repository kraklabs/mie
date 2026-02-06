// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import "context"

// MockQuerier is a mock implementation of the Querier interface for unit testing.
type MockQuerier struct {
	StoreFactFunc            func(ctx context.Context, req StoreFactRequest) (*Fact, error)
	StoreDecisionFunc        func(ctx context.Context, req StoreDecisionRequest) (*Decision, error)
	StoreEntityFunc          func(ctx context.Context, req StoreEntityRequest) (*Entity, error)
	StoreEventFunc           func(ctx context.Context, req StoreEventRequest) (*Event, error)
	StoreTopicFunc           func(ctx context.Context, req StoreTopicRequest) (*Topic, error)
	InvalidateFactFunc       func(ctx context.Context, oldFactID, newFactID, reason string) error
	AddRelationshipFunc      func(ctx context.Context, edgeType string, fields map[string]string) error
	SemanticSearchFunc       func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error)
	ExactSearchFunc          func(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error)
	GetNodeByIDFunc          func(ctx context.Context, nodeID string) (any, error)
	ListNodesFunc            func(ctx context.Context, opts ListOptions) ([]any, int, error)
	GetRelatedEntitiesFunc   func(ctx context.Context, factID string) ([]Entity, error)
	GetFactsAboutEntityFunc  func(ctx context.Context, entityID string) ([]Fact, error)
	GetDecisionEntitiesFunc  func(ctx context.Context, decisionID string) ([]EntityWithRole, error)
	GetInvalidationChainFunc func(ctx context.Context, factID string) ([]Invalidation, error)
	GetRelatedFactsFunc      func(ctx context.Context, entityID string) ([]Fact, error)
	GetEntityDecisionsFunc   func(ctx context.Context, entityID string) ([]Decision, error)
	UpdateDescriptionFunc    func(ctx context.Context, nodeID, newDescription string) error
	UpdateStatusFunc         func(ctx context.Context, nodeID, newStatus string) error
	DetectConflictsFunc      func(ctx context.Context, opts ConflictOptions) ([]Conflict, error)
	CheckNewFactConflictsFunc func(ctx context.Context, content, category string) ([]Conflict, error)
	GetStatsFunc             func(ctx context.Context) (*GraphStats, error)
	ExportGraphFunc          func(ctx context.Context, opts ExportOptions) (*ExportData, error)
	IncrementCounterFunc     func(ctx context.Context, key string) error
	EmbeddingsEnabledFunc    func() bool
}

func (m *MockQuerier) StoreFact(ctx context.Context, req StoreFactRequest) (*Fact, error) {
	if m.StoreFactFunc != nil {
		return m.StoreFactFunc(ctx, req)
	}
	return &Fact{ID: "fact:mock0001", Content: req.Content, Category: req.Category, Confidence: req.Confidence, Valid: true, SourceAgent: req.SourceAgent, CreatedAt: 1000, UpdatedAt: 1000}, nil
}

func (m *MockQuerier) StoreDecision(ctx context.Context, req StoreDecisionRequest) (*Decision, error) {
	if m.StoreDecisionFunc != nil {
		return m.StoreDecisionFunc(ctx, req)
	}
	return &Decision{ID: "dec:mock0001", Title: req.Title, Rationale: req.Rationale, Status: "active", SourceAgent: req.SourceAgent, CreatedAt: 1000, UpdatedAt: 1000}, nil
}

func (m *MockQuerier) StoreEntity(ctx context.Context, req StoreEntityRequest) (*Entity, error) {
	if m.StoreEntityFunc != nil {
		return m.StoreEntityFunc(ctx, req)
	}
	return &Entity{ID: "ent:mock0001", Name: req.Name, Kind: req.Kind, Description: req.Description, SourceAgent: req.SourceAgent, CreatedAt: 1000, UpdatedAt: 1000}, nil
}

func (m *MockQuerier) StoreEvent(ctx context.Context, req StoreEventRequest) (*Event, error) {
	if m.StoreEventFunc != nil {
		return m.StoreEventFunc(ctx, req)
	}
	return &Event{ID: "evt:mock0001", Title: req.Title, EventDate: req.EventDate, SourceAgent: req.SourceAgent, CreatedAt: 1000, UpdatedAt: 1000}, nil
}

func (m *MockQuerier) StoreTopic(ctx context.Context, req StoreTopicRequest) (*Topic, error) {
	if m.StoreTopicFunc != nil {
		return m.StoreTopicFunc(ctx, req)
	}
	return &Topic{ID: "top:mock0001", Name: req.Name, Description: req.Description, CreatedAt: 1000, UpdatedAt: 1000}, nil
}

func (m *MockQuerier) InvalidateFact(ctx context.Context, oldFactID, newFactID, reason string) error {
	if m.InvalidateFactFunc != nil {
		return m.InvalidateFactFunc(ctx, oldFactID, newFactID, reason)
	}
	return nil
}

func (m *MockQuerier) AddRelationship(ctx context.Context, edgeType string, fields map[string]string) error {
	if m.AddRelationshipFunc != nil {
		return m.AddRelationshipFunc(ctx, edgeType, fields)
	}
	return nil
}

func (m *MockQuerier) SemanticSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
	if m.SemanticSearchFunc != nil {
		return m.SemanticSearchFunc(ctx, query, nodeTypes, limit)
	}
	return []SearchResult{}, nil
}

func (m *MockQuerier) ExactSearch(ctx context.Context, query string, nodeTypes []string, limit int) ([]SearchResult, error) {
	if m.ExactSearchFunc != nil {
		return m.ExactSearchFunc(ctx, query, nodeTypes, limit)
	}
	return []SearchResult{}, nil
}

func (m *MockQuerier) GetNodeByID(ctx context.Context, nodeID string) (any, error) {
	if m.GetNodeByIDFunc != nil {
		return m.GetNodeByIDFunc(ctx, nodeID)
	}
	return nil, nil
}

func (m *MockQuerier) ListNodes(ctx context.Context, opts ListOptions) ([]any, int, error) {
	if m.ListNodesFunc != nil {
		return m.ListNodesFunc(ctx, opts)
	}
	return []any{}, 0, nil
}

func (m *MockQuerier) GetRelatedEntities(ctx context.Context, factID string) ([]Entity, error) {
	if m.GetRelatedEntitiesFunc != nil {
		return m.GetRelatedEntitiesFunc(ctx, factID)
	}
	return []Entity{}, nil
}

func (m *MockQuerier) GetFactsAboutEntity(ctx context.Context, entityID string) ([]Fact, error) {
	if m.GetFactsAboutEntityFunc != nil {
		return m.GetFactsAboutEntityFunc(ctx, entityID)
	}
	return []Fact{}, nil
}

func (m *MockQuerier) GetDecisionEntities(ctx context.Context, decisionID string) ([]EntityWithRole, error) {
	if m.GetDecisionEntitiesFunc != nil {
		return m.GetDecisionEntitiesFunc(ctx, decisionID)
	}
	return []EntityWithRole{}, nil
}

func (m *MockQuerier) GetInvalidationChain(ctx context.Context, factID string) ([]Invalidation, error) {
	if m.GetInvalidationChainFunc != nil {
		return m.GetInvalidationChainFunc(ctx, factID)
	}
	return []Invalidation{}, nil
}

func (m *MockQuerier) GetRelatedFacts(ctx context.Context, entityID string) ([]Fact, error) {
	if m.GetRelatedFactsFunc != nil {
		return m.GetRelatedFactsFunc(ctx, entityID)
	}
	return []Fact{}, nil
}

func (m *MockQuerier) GetEntityDecisions(ctx context.Context, entityID string) ([]Decision, error) {
	if m.GetEntityDecisionsFunc != nil {
		return m.GetEntityDecisionsFunc(ctx, entityID)
	}
	return []Decision{}, nil
}

func (m *MockQuerier) UpdateDescription(ctx context.Context, nodeID, newDescription string) error {
	if m.UpdateDescriptionFunc != nil {
		return m.UpdateDescriptionFunc(ctx, nodeID, newDescription)
	}
	return nil
}

func (m *MockQuerier) UpdateStatus(ctx context.Context, nodeID, newStatus string) error {
	if m.UpdateStatusFunc != nil {
		return m.UpdateStatusFunc(ctx, nodeID, newStatus)
	}
	return nil
}

func (m *MockQuerier) DetectConflicts(ctx context.Context, opts ConflictOptions) ([]Conflict, error) {
	if m.DetectConflictsFunc != nil {
		return m.DetectConflictsFunc(ctx, opts)
	}
	return []Conflict{}, nil
}

func (m *MockQuerier) CheckNewFactConflicts(ctx context.Context, content, category string) ([]Conflict, error) {
	if m.CheckNewFactConflictsFunc != nil {
		return m.CheckNewFactConflictsFunc(ctx, content, category)
	}
	return []Conflict{}, nil
}

func (m *MockQuerier) GetStats(ctx context.Context) (*GraphStats, error) {
	if m.GetStatsFunc != nil {
		return m.GetStatsFunc(ctx)
	}
	return &GraphStats{}, nil
}

func (m *MockQuerier) ExportGraph(ctx context.Context, opts ExportOptions) (*ExportData, error) {
	if m.ExportGraphFunc != nil {
		return m.ExportGraphFunc(ctx, opts)
	}
	return &ExportData{Version: "1", ExportedAt: "2026-02-05T00:00:00Z", Stats: map[string]int{}}, nil
}

func (m *MockQuerier) IncrementCounter(ctx context.Context, key string) error {
	if m.IncrementCounterFunc != nil {
		return m.IncrementCounterFunc(ctx, key)
	}
	return nil
}

func (m *MockQuerier) EmbeddingsEnabled() bool {
	if m.EmbeddingsEnabledFunc != nil {
		return m.EmbeddingsEnabledFunc()
	}
	return true
}
