// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"time"
)

// Status returns memory graph health and statistics.
func Status(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	stats, err := client.GetStats(ctx)
	if err != nil {
		return NewError(fmt.Sprintf("Failed to get graph stats: %v", err)), nil
	}

	var sb string
	sb += "## MIE Memory Status\n\n"

	// Graph statistics
	sb += "### Graph Statistics\n"
	sb += fmt.Sprintf("- Facts: %d (%d valid, %d invalidated)\n", stats.TotalFacts, stats.ValidFacts, stats.InvalidatedFacts)
	sb += fmt.Sprintf("- Decisions: %d (%d active, %d other)\n", stats.TotalDecisions, stats.ActiveDecisions, stats.TotalDecisions-stats.ActiveDecisions)
	sb += fmt.Sprintf("- Entities: %d\n", stats.TotalEntities)
	sb += fmt.Sprintf("- Events: %d\n", stats.TotalEvents)
	sb += fmt.Sprintf("- Topics: %d\n", stats.TotalTopics)
	sb += fmt.Sprintf("- Relationships: %d edges total\n", stats.TotalEdges)

	// Configuration
	sb += "\n### Configuration\n"
	if stats.StorageEngine != "" {
		sb += fmt.Sprintf("- Storage: %s", stats.StorageEngine)
		if stats.StoragePath != "" {
			sb += fmt.Sprintf(" (%s)", stats.StoragePath)
		}
		sb += "\n"
	}
	if client.EmbeddingsEnabled() {
		sb += "- Embeddings: enabled\n"
	} else {
		sb += "- Embeddings: disabled\n"
	}
	if stats.SchemaVersion != "" {
		sb += fmt.Sprintf("- Schema version: %s\n", stats.SchemaVersion)
	}

	// Health checks
	sb += "\n### Health\n"
	totalNodes := stats.TotalFacts + stats.TotalDecisions + stats.TotalEntities + stats.TotalEvents + stats.TotalTopics
	if totalNodes > 0 {
		sb += fmt.Sprintf("- Database accessible (%d total nodes)\n", totalNodes)
	} else {
		sb += "- Database accessible (empty graph)\n"
	}
	if client.EmbeddingsEnabled() {
		sb += "- Embeddings enabled\n"
	} else {
		sb += "- Embeddings disabled (semantic search unavailable)\n"
	}

	// Usage metrics
	if stats.TotalQueries > 0 || stats.TotalStores > 0 {
		sb += "\n### Usage\n"
		sb += fmt.Sprintf("- Total queries: %d\n", stats.TotalQueries)
		sb += fmt.Sprintf("- Total stores: %d\n", stats.TotalStores)
		if stats.LastQueryAt > 0 {
			sb += fmt.Sprintf("- Last query: %s\n", time.Unix(stats.LastQueryAt, 0).UTC().Format("2006-01-02 15:04:05"))
		}
		if stats.LastStoreAt > 0 {
			sb += fmt.Sprintf("- Last store: %s\n", time.Unix(stats.LastStoreAt, 0).UTC().Format("2006-01-02 15:04:05"))
		}
	}

	return NewResult(sb), nil
}
