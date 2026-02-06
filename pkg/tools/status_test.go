// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestStatus_PopulatedGraph(t *testing.T) {
	mock := &MockQuerier{
		GetStatsFunc: func(ctx context.Context) (*GraphStats, error) {
			return &GraphStats{
				TotalFacts:       47,
				ValidFacts:       42,
				InvalidatedFacts: 5,
				TotalDecisions:   12,
				ActiveDecisions:  10,
				TotalEntities:    23,
				TotalEvents:      8,
				TotalTopics:      5,
				TotalEdges:       89,
				TotalQueries:     42,
				TotalStores:      15,
				LastQueryAt:      1738853400,
				LastStoreAt:      1738848900,
				SchemaVersion:    "1",
				StorageEngine:    "sqlite",
				StoragePath:      "~/.mie/data/default/index.db",
			}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return true },
	}

	result, err := Status(context.Background(), mock, map[string]any{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Status() returned error result: %s", result.Text)
	}

	checks := []string{
		"MIE Memory Status",
		"Facts: 47 (42 valid, 5 invalidated)",
		"Decisions: 12 (10 active",
		"Entities: 23",
		"Events: 8",
		"Topics: 5",
		"89 edges",
		"sqlite",
		"Embeddings: enabled",
		"Schema version: 1",
		"### Usage",
		"Total queries: 42",
		"Total stores: 15",
		"Last query:",
		"Last store:",
	}
	for _, check := range checks {
		if !strings.Contains(result.Text, check) {
			t.Errorf("Status() output missing %q", check)
		}
	}
}

func TestStatus_EmptyGraph(t *testing.T) {
	mock := &MockQuerier{
		GetStatsFunc: func(ctx context.Context) (*GraphStats, error) {
			return &GraphStats{}, nil
		},
		EmbeddingsEnabledFunc: func() bool { return false },
	}

	result, err := Status(context.Background(), mock, map[string]any{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("Status() returned error: %s", result.Text)
	}

	if !strings.Contains(result.Text, "empty graph") {
		t.Error("Status() should mention empty graph")
	}
	if !strings.Contains(result.Text, "Embeddings: disabled") {
		t.Error("Status() should show embeddings disabled")
	}
	if strings.Contains(result.Text, "### Usage") {
		t.Error("Status() should not show Usage section when counters are zero")
	}
}