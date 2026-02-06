// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

const maxBulkItems = 50

// bulkItem tracks the result of storing a single item in a bulk operation.
type bulkItem struct {
	nodeID   string
	nodeType string
	summary  string
}

// BulkStore writes multiple nodes and optional relationships to the memory graph in a single call.
func BulkStore(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	rawItems, ok := args["items"]
	if !ok || rawItems == nil {
		return NewError("Missing required parameter: items"), nil
	}
	itemSlice, ok := rawItems.([]any)
	if !ok || len(itemSlice) == 0 {
		return NewError("items must be a non-empty array"), nil
	}
	if len(itemSlice) > maxBulkItems {
		return NewError(fmt.Sprintf("Too many items: %d (max %d)", len(itemSlice), maxBulkItems)), nil
	}

	// Phase 1: Store all nodes and collect their IDs.
	stored := make([]bulkItem, len(itemSlice))
	var errors []string
	typeCounts := map[string]int{}

	for i, raw := range itemSlice {
		itemArgs, ok := raw.(map[string]any)
		if !ok {
			errors = append(errors, fmt.Sprintf("item[%d]: not a valid object", i))
			continue
		}
		nodeType := GetStringArg(itemArgs, "type", "")
		if nodeType == "" {
			errors = append(errors, fmt.Sprintf("item[%d]: missing required parameter: type", i))
			continue
		}

		nodeID, summary, err := storeNode(ctx, client, itemArgs, nodeType)
		if err != nil {
			errors = append(errors, fmt.Sprintf("item[%d] (%s): %v", i, nodeType, err))
			continue
		}
		if nodeID == "" {
			errors = append(errors, fmt.Sprintf("item[%d]: invalid type %q", i, nodeType))
			continue
		}

		stored[i] = bulkItem{nodeID: nodeID, nodeType: nodeType, summary: summary}
		typeCounts[nodeType]++
	}

	// Phase 2: Handle invalidations and relationships for successfully stored items.
	var relMessages []string
	for i, item := range stored {
		if item.nodeID == "" {
			continue
		}
		itemArgs, _ := itemSlice[i].(map[string]any)

		// Handle invalidation.
		toolErr, invalidationMsg := handleInvalidation(ctx, client, itemArgs, item.nodeID)
		if toolErr != nil {
			errors = append(errors, fmt.Sprintf("item[%d] invalidation: %s", i, toolErr.Text))
		} else if invalidationMsg != "" {
			relMessages = append(relMessages, fmt.Sprintf("item[%d]%s", i, invalidationMsg))
		}

		// Handle relationships, resolving cross-batch references.
		if rels, ok := itemArgs["relationships"]; ok && rels != nil {
			resolved := resolveBatchRefs(rels, stored)
			if msg := storeRelationships(ctx, client, item.nodeID, resolved); msg != "" {
				relMessages = append(relMessages, fmt.Sprintf("item[%d]:\n%s", i, msg))
			}
		}
	}

	// Phase 3: Build output.
	var sb strings.Builder

	// Summary line.
	var parts []string
	for _, nt := range []string{"fact", "decision", "entity", "event", "topic"} {
		if c := typeCounts[nt]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %ss", c, nt))
		}
	}
	totalStored := 0
	for _, c := range typeCounts {
		totalStored += c
	}
	sb.WriteString(fmt.Sprintf("Stored %d items: %s\n", totalStored, strings.Join(parts, ", ")))

	// Increment usage counters (never fail the main operation).
	for range totalStored {
		_ = client.IncrementCounter(ctx, "total_stores")
	}

	// Per-item IDs.
	sb.WriteString("\nIDs:\n")
	for i, item := range stored {
		if item.nodeID != "" {
			sb.WriteString(fmt.Sprintf("  [%d] %s [%s]\n", i, item.nodeType, item.nodeID))
		}
	}

	// Relationships.
	if len(relMessages) > 0 {
		sb.WriteString("\nRelationships:\n")
		for _, msg := range relMessages {
			sb.WriteString(msg)
		}
	}

	// Errors.
	if len(errors) > 0 {
		sb.WriteString(fmt.Sprintf("\nErrors (%d):\n", len(errors)))
		for _, e := range errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	return NewResult(sb.String()), nil
}

// resolveBatchRefs replaces target_ref index references in relationships with actual IDs
// from previously stored items in the same batch.
func resolveBatchRefs(rels any, stored []bulkItem) []any {
	relSlice, ok := rels.([]any)
	if !ok {
		return nil
	}
	resolved := make([]any, 0, len(relSlice))
	for _, rel := range relSlice {
		relMap, ok := rel.(map[string]any)
		if !ok {
			continue
		}
		// Check for target_ref (cross-batch index reference).
		if refIdx, hasRef := relMap["target_ref"]; hasRef {
			idx := toInt(refIdx)
			if idx < 0 || idx >= len(stored) || stored[idx].nodeID == "" {
				continue
			}
			// Copy the map and replace target_ref with the resolved target_id.
			resolved = append(resolved, map[string]any{
				"edge":      relMap["edge"],
				"target_id": stored[idx].nodeID,
				"role":      relMap["role"],
			})
		} else {
			resolved = append(resolved, relMap)
		}
	}
	return resolved
}

// toInt converts a JSON number to int. JSON numbers from map[string]any are float64.
func toInt(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	default:
		return -1
	}
}