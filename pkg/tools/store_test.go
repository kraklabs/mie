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

func TestStore_Fact(t *testing.T) {
	mock := &MockQuerier{}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":         "fact",
		"content":      "User works at Kraklabs",
		"category":     "professional",
		"confidence":   0.95,
		"source_agent": "claude",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored fact") {
		t.Error("Store() should confirm fact storage")
	}
	if !strings.Contains(result.Text, "fact:mock0001") {
		t.Error("Store() should include fact ID")
	}
	if !strings.Contains(result.Text, "professional") {
		t.Error("Store() should include category")
	}
}

func TestStore_Decision(t *testing.T) {
	mock := &MockQuerier{}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":      "decision",
		"title":     "Chose Go for CIE",
		"rationale": "CGO CozoDB bindings only available in Go",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored decision") {
		t.Error("Store() should confirm decision storage")
	}
}

func TestStore_Entity(t *testing.T) {
	mock := &MockQuerier{}
	result, err := Store(context.Background(), mock, map[string]any{
		"type": "entity",
		"name": "Kraklabs",
		"kind": "company",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored entity") {
		t.Error("Store() should confirm entity storage")
	}
}

func TestStore_Event(t *testing.T) {
	mock := &MockQuerier{}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":       "event",
		"title":      "Launched CIE v0.4.0",
		"event_date": "2026-01-15",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored event") {
		t.Error("Store() should confirm event storage")
	}
}

func TestStore_Topic(t *testing.T) {
	mock := &MockQuerier{}
	result, err := Store(context.Background(), mock, map[string]any{
		"type": "topic",
		"name": "Architecture",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if !strings.Contains(result.Text, "Stored topic") {
		t.Error("Store() should confirm topic storage")
	}
}

func TestStore_MissingType(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{})
	if !result.IsError {
		t.Error("Store() should return error when type is missing")
	}
	if !strings.Contains(result.Text, "type") {
		t.Error("Error should mention 'type'")
	}
}

func TestStore_InvalidType(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type": "invalid",
	})
	if !result.IsError {
		t.Error("Store() should return error for invalid type")
	}
}

func TestStore_FactMissingContent(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type": "fact",
	})
	if !result.IsError {
		t.Error("Store() should return error when fact content is missing")
	}
}

func TestStore_DecisionMissingTitle(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type":      "decision",
		"rationale": "some reason",
	})
	if !result.IsError {
		t.Error("Store() should return error when decision title is missing")
	}
}

func TestStore_DecisionMissingRationale(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type":  "decision",
		"title": "some title",
	})
	if !result.IsError {
		t.Error("Store() should return error when decision rationale is missing")
	}
}

func TestStore_EntityMissingName(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type": "entity",
		"kind": "company",
	})
	if !result.IsError {
		t.Error("Store() should return error when entity name is missing")
	}
}

func TestStore_EntityInvalidKind(t *testing.T) {
	mock := &MockQuerier{}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type": "entity",
		"name": "Test",
		"kind": "invalid_kind",
	})
	if !result.IsError {
		t.Error("Store() should return error for invalid entity kind")
	}
}

func TestStore_WithInvalidation(t *testing.T) {
	invalidated := false
	mock := &MockQuerier{
		InvalidateFactFunc: func(ctx context.Context, oldFactID, newFactID, reason string) error {
			invalidated = true
			if oldFactID != "fact:old123" {
				t.Errorf("Expected oldFactID=fact:old123, got %s", oldFactID)
			}
			return nil
		},
	}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":        "fact",
		"content":     "User moved to NYC",
		"category":    "personal",
		"invalidates": "fact:old123",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if !invalidated {
		t.Error("Store() should have called InvalidateFact")
	}
	if !strings.Contains(result.Text, "Invalidated") {
		t.Error("Store() should mention invalidation in output")
	}
}

func TestStore_WithRelationships(t *testing.T) {
	relCount := 0
	mock := &MockQuerier{
		AddRelationshipFunc: func(ctx context.Context, edgeType string, fields map[string]string) error {
			relCount++
			return nil
		},
	}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":    "fact",
		"content": "User works at Kraklabs",
		"relationships": []any{
			map[string]any{
				"edge":      "fact_entity",
				"target_id": "ent:abc123",
			},
		},
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if relCount != 1 {
		t.Errorf("Expected 1 relationship created, got %d", relCount)
	}
	if !strings.Contains(result.Text, "fact_entity") {
		t.Error("Store() should mention relationship in output")
	}
}

func TestStore_FactDefaultCategory(t *testing.T) {
	var capturedReq StoreFactRequest
	mock := &MockQuerier{
		StoreFactFunc: func(ctx context.Context, req StoreFactRequest) (*Fact, error) {
			capturedReq = req
			return &Fact{ID: "fact:test", Content: req.Content, Category: req.Category, Confidence: req.Confidence, Valid: true}, nil
		},
	}
	_, err := Store(context.Background(), mock, map[string]any{
		"type":    "fact",
		"content": "Some fact",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if capturedReq.Category != "general" {
		t.Errorf("Default category should be 'general', got %q", capturedReq.Category)
	}
	if capturedReq.Confidence != 0.8 {
		t.Errorf("Default confidence should be 0.8, got %f", capturedReq.Confidence)
	}
	if capturedReq.SourceAgent != "unknown" {
		t.Errorf("Default source_agent should be 'unknown', got %q", capturedReq.SourceAgent)
	}
}

func TestStore_StorageError(t *testing.T) {
	mock := &MockQuerier{
		StoreFactFunc: func(ctx context.Context, req StoreFactRequest) (*Fact, error) {
			return nil, fmt.Errorf("database connection failed")
		},
	}
	result, _ := Store(context.Background(), mock, map[string]any{
		"type":    "fact",
		"content": "Test",
	})
	if !result.IsError {
		t.Error("Store() should return error when storage fails")
	}
	if !strings.Contains(result.Text, "database connection failed") {
		t.Error("Error should include underlying error message")
	}
}

func TestStore_IncrementsCounter(t *testing.T) {
	var counterKey string
	mock := &MockQuerier{
		IncrementCounterFunc: func(ctx context.Context, key string) error {
			counterKey = key
			return nil
		},
	}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":    "fact",
		"content": "Test counter increment",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Store() returned error: %s", result.Text)
	}
	if counterKey != "total_stores" {
		t.Errorf("Expected IncrementCounter called with %q, got %q", "total_stores", counterKey)
	}
}

func TestStore_CounterErrorDoesNotFailStore(t *testing.T) {
	mock := &MockQuerier{
		IncrementCounterFunc: func(ctx context.Context, key string) error {
			return fmt.Errorf("counter write failed")
		},
	}
	result, err := Store(context.Background(), mock, map[string]any{
		"type":    "fact",
		"content": "Test counter error ignored",
	})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if result.IsError {
		t.Error("Store() should succeed even when counter increment fails")
	}
}