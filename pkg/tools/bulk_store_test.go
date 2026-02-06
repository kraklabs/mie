// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestBulkStore_MixedTypes(t *testing.T) {
	callCount := 0
	mock := &MockQuerier{
		StoreFactFunc: func(ctx context.Context, req StoreFactRequest) (*Fact, error) {
			callCount++
			return &Fact{ID: fmt.Sprintf("fact:bulk%04d", callCount), Content: req.Content, Category: req.Category, Confidence: req.Confidence, Valid: true, SourceAgent: req.SourceAgent}, nil
		},
		StoreEntityFunc: func(ctx context.Context, req StoreEntityRequest) (*Entity, error) {
			callCount++
			return &Entity{ID: fmt.Sprintf("ent:bulk%04d", callCount), Name: req.Name, Kind: req.Kind, SourceAgent: req.SourceAgent}, nil
		},
	}

	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"type": "fact", "content": "User likes Go", "category": "preference"},
			map[string]any{"type": "entity", "name": "Go", "kind": "technology"},
			map[string]any{"type": "fact", "content": "User uses CozoDB", "category": "technical"},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored 3 items") {
		t.Errorf("expected 'Stored 3 items' in output, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "2 facts") {
		t.Errorf("expected '2 facts' in output, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "1 entitys") {
		t.Errorf("expected '1 entitys' in output, got: %s", result.Text)
	}
}

func TestBulkStore_MissingItems(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := BulkStore(context.Background(), mock, map[string]any{})
	if !result.IsError {
		t.Error("BulkStore() should return error when items is missing")
	}
	if !strings.Contains(result.Text, "items") {
		t.Error("error should mention 'items'")
	}
}

func TestBulkStore_EmptyItems(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{},
	})
	if !result.IsError {
		t.Error("BulkStore() should return error for empty items array")
	}
}

func TestBulkStore_TooManyItems(t *testing.T) {
	mock := &MockQuerier{}
	items := make([]any, 51)
	for i := range items {
		items[i] = map[string]any{"type": "fact", "content": "test"}
	}
	result, _ := BulkStore(context.Background(), mock, map[string]any{
		"items": items,
	})
	if !result.IsError {
		t.Error("BulkStore() should return error when items exceed max")
	}
	if !strings.Contains(result.Text, "51") {
		t.Error("error should mention the count")
	}
}

func TestBulkStore_CrossBatchReferences(t *testing.T) {
	var relCalls []map[string]string
	entityID := "ent:ref0001"
	mock := &MockQuerier{
		StoreEntityFunc: func(ctx context.Context, req StoreEntityRequest) (*Entity, error) {
			return &Entity{ID: entityID, Name: req.Name, Kind: req.Kind, SourceAgent: req.SourceAgent}, nil
		},
		AddRelationshipFunc: func(ctx context.Context, edgeType string, fields map[string]string) error {
			relCalls = append(relCalls, fields)
			return nil
		},
	}

	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"type": "entity", "name": "Kraklabs", "kind": "company"},
			map[string]any{
				"type":    "fact",
				"content": "User works at Kraklabs",
				"relationships": []any{
					map[string]any{
						"edge":       "fact_entity",
						"target_ref": float64(0), // reference item[0]
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	if len(relCalls) != 1 {
		t.Fatalf("expected 1 relationship call, got %d", len(relCalls))
	}
	if relCalls[0]["entity_id"] != entityID {
		t.Errorf("expected entity_id=%s, got %s", entityID, relCalls[0]["entity_id"])
	}
}

func TestBulkStore_CrossBatchRefOutOfBounds(t *testing.T) {
	mock := &MockQuerier{}

	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{
				"type":    "fact",
				"content": "Some fact",
				"relationships": []any{
					map[string]any{
						"edge":       "fact_entity",
						"target_ref": float64(99), // out of bounds
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	// Should store the fact but skip the unresolvable relationship.
	if !strings.Contains(result.Text, "Stored 1 items") {
		t.Errorf("expected 'Stored 1 items', got: %s", result.Text)
	}
}

func TestBulkStore_PartialFailure(t *testing.T) {
	mock := &MockQuerier{
		StoreFactFunc: func(ctx context.Context, req StoreFactRequest) (*Fact, error) {
			return nil, fmt.Errorf("storage error")
		},
	}

	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"type": "entity", "name": "Go", "kind": "technology"},
			map[string]any{"type": "fact", "content": "will fail"},
			map[string]any{"type": "topic", "name": "testing"},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() should not be an error result for partial failures")
	}
	if !strings.Contains(result.Text, "Stored 2 items") {
		t.Errorf("expected 'Stored 2 items', got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Errors (1)") {
		t.Errorf("expected 'Errors (1)' in output, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "storage error") {
		t.Errorf("expected underlying error message in output, got: %s", result.Text)
	}
}

func TestBulkStore_InvalidItemType(t *testing.T) {
	mock := &MockQuerier{}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"type": "invalid_type"},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() should not be error result for invalid item type")
	}
	if !strings.Contains(result.Text, "Stored 0 items") {
		t.Errorf("expected 'Stored 0 items', got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "invalid type") {
		t.Errorf("expected 'invalid type' in errors, got: %s", result.Text)
	}
}

func TestBulkStore_ItemMissingType(t *testing.T) {
	mock := &MockQuerier{}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"content": "no type field"},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if !strings.Contains(result.Text, "missing required parameter: type") {
		t.Errorf("expected type error, got: %s", result.Text)
	}
}

func TestBulkStore_WithInvalidation(t *testing.T) {
	invalidated := false
	mock := &MockQuerier{
		InvalidateFactFunc: func(ctx context.Context, oldFactID, newFactID, reason string) error {
			invalidated = true
			if oldFactID != "fact:old123" {
				t.Errorf("expected oldFactID=fact:old123, got %s", oldFactID)
			}
			return nil
		},
	}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{
				"type":        "fact",
				"content":     "User moved to NYC",
				"category":    "personal",
				"invalidates": "fact:old123",
			},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	if !invalidated {
		t.Error("BulkStore() should have called InvalidateFact")
	}
	if !strings.Contains(result.Text, "Invalidated") {
		t.Error("output should mention invalidation")
	}
}

func TestBulkStore_WithDirectRelationship(t *testing.T) {
	relCount := 0
	mock := &MockQuerier{
		AddRelationshipFunc: func(ctx context.Context, edgeType string, fields map[string]string) error {
			relCount++
			return nil
		},
	}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{
				"type":    "fact",
				"content": "User works at Kraklabs",
				"relationships": []any{
					map[string]any{
						"edge":      "fact_entity",
						"target_id": "ent:existing123",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	if relCount != 1 {
		t.Errorf("expected 1 relationship, got %d", relCount)
	}
}

func TestBulkStore_NonObjectItem(t *testing.T) {
	mock := &MockQuerier{}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			"not a map",
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if !strings.Contains(result.Text, "not a valid object") {
		t.Errorf("expected 'not a valid object' in errors, got: %s", result.Text)
	}
}

func TestBulkStore_SingleFact(t *testing.T) {
	mock := &MockQuerier{}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"type": "fact", "content": "Single fact", "source_agent": "claude"},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored 1 items: 1 facts") {
		t.Errorf("expected single fact summary, got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "fact:mock0001") {
		t.Errorf("expected mock fact ID in output, got: %s", result.Text)
	}
}

func TestBulkStore_AllFiveTypes(t *testing.T) {
	mock := &MockQuerier{}
	result, err := BulkStore(context.Background(), mock, map[string]any{
		"items": []any{
			map[string]any{"type": "fact", "content": "A fact"},
			map[string]any{"type": "decision", "title": "A decision", "rationale": "Because"},
			map[string]any{"type": "entity", "name": "An entity", "kind": "project"},
			map[string]any{"type": "event", "title": "An event", "event_date": "2026-01-01"},
			map[string]any{"type": "topic", "name": "A topic"},
		},
	})
	if err != nil {
		t.Fatalf("BulkStore() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("BulkStore() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored 5 items") {
		t.Errorf("expected 'Stored 5 items', got: %s", result.Text)
	}
	for _, typ := range []string{"1 facts", "1 decisions", "1 entitys", "1 events", "1 topics"} {
		if !strings.Contains(result.Text, typ) {
			t.Errorf("expected %q in output, got: %s", typ, result.Text)
		}
	}
}