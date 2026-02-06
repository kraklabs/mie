// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// Query reads from the memory graph. Supports semantic search, exact lookup, and graph traversal.
func Query(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	query := GetStringArg(args, "query", "")
	if query == "" {
		return NewError("Missing required parameter: query"), nil
	}

	mode := GetStringArg(args, "mode", "semantic")
	nodeTypes := GetStringSliceArg(args, "node_types", []string{"fact", "decision", "entity", "event"})
	limit := GetIntArg(args, "limit", 10)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	var result *ToolResult
	var err error
	switch mode {
	case "semantic":
		result, err = querySemanticMode(ctx, client, query, nodeTypes, limit)
	case "exact":
		result, err = queryExactMode(ctx, client, query, nodeTypes, limit)
	case "graph":
		result, err = queryGraphMode(ctx, client, args)
	default:
		return NewError(fmt.Sprintf("Invalid mode %q. Must be one of: semantic, exact, graph", mode)), nil
	}

	// Increment usage counter on success (never fail the main operation).
	if err == nil && result != nil && !result.IsError {
		_ = client.IncrementCounter(ctx, "total_queries")
	}

	return result, err
}

func querySemanticMode(ctx context.Context, client Querier, query string, nodeTypes []string, limit int) (*ToolResult, error) {
	if !client.EmbeddingsEnabled() {
		return NewError("Semantic search requires embeddings to be enabled. Enable in config or use mode=exact."), nil
	}

	results, err := client.SemanticSearch(ctx, query, nodeTypes, limit)
	if err != nil {
		return NewError(fmt.Sprintf("Semantic search failed: %v", err)), nil
	}

	if len(results) == 0 {
		return NewResult(fmt.Sprintf("## Memory Search Results for: %q\n\n_No results found._\n", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Memory Search Results for: %q\n\n", query))

	// Group results by type
	grouped := map[string][]SearchResult{}
	for _, r := range results {
		grouped[r.NodeType] = append(grouped[r.NodeType], r)
	}

	typeLabels := map[string]string{
		"fact": "Facts", "decision": "Decisions", "entity": "Entities", "event": "Events",
	}

	for _, nt := range nodeTypes {
		items, ok := grouped[nt]
		if !ok || len(items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%d results)\n", typeLabels[nt], len(items)))
		for i, item := range items {
			pct := SimilarityPercent(item.Distance)
			indicator := SimilarityIndicator(item.Distance)
			sb.WriteString(fmt.Sprintf("%d. %s %d%% [%s] %q\n", i+1, indicator, pct, item.ID, Truncate(item.Content, 100)))
			if item.Detail != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", item.Detail))
			}
		}
		sb.WriteString("\n")
	}

	return NewResult(sb.String()), nil
}

func queryExactMode(ctx context.Context, client Querier, query string, nodeTypes []string, limit int) (*ToolResult, error) {
	results, err := client.ExactSearch(ctx, query, nodeTypes, limit)
	if err != nil {
		return NewError(fmt.Sprintf("Exact search failed: %v", err)), nil
	}

	if len(results) == 0 {
		return NewResult(fmt.Sprintf("## Exact Search Results for: %q\n\n_No results found._\n", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Exact Search Results for: %q\n\n", query))

	grouped := map[string][]SearchResult{}
	for _, r := range results {
		grouped[r.NodeType] = append(grouped[r.NodeType], r)
	}

	typeLabels := map[string]string{
		"fact": "Facts", "decision": "Decisions", "entity": "Entities", "event": "Events",
	}

	for _, nt := range nodeTypes {
		items, ok := grouped[nt]
		if !ok || len(items) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%d results)\n", typeLabels[nt], len(items)))
		for i, item := range items {
			sb.WriteString(fmt.Sprintf("%d. [%s] %q\n", i+1, item.ID, Truncate(item.Content, 100)))
			if item.Detail != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", item.Detail))
			}
		}
		sb.WriteString("\n")
	}

	return NewResult(sb.String()), nil
}

func queryGraphMode(ctx context.Context, client Querier, args map[string]any) (*ToolResult, error) {
	nodeID := GetStringArg(args, "node_id", "")
	if nodeID == "" {
		return NewError("node_id is required for graph mode"), nil
	}

	traversal := GetStringArg(args, "traversal", "")
	if traversal == "" {
		return NewError("traversal is required for graph mode"), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Graph Traversal: %s from [%s]\n\n", traversal, nodeID)

	var err error
	switch traversal {
	case "related_entities":
		err = traverseRelatedEntities(ctx, client, &sb, nodeID)
	case "related_facts", "facts_about_entity":
		err = traverseRelatedFacts(ctx, client, &sb, nodeID)
	case "invalidation_chain":
		err = traverseInvalidationChain(ctx, client, &sb, nodeID)
	case "decision_entities":
		err = traverseDecisionEntities(ctx, client, &sb, nodeID)
	case "entity_decisions":
		err = traverseEntityDecisions(ctx, client, &sb, nodeID)
	default:
		return NewError(fmt.Sprintf("Invalid traversal type %q. Must be one of: related_entities, related_facts, invalidation_chain, decision_entities, facts_about_entity, entity_decisions", traversal)), nil
	}

	if err != nil {
		return NewError(fmt.Sprintf("Traversal failed: %v", err)), nil
	}

	return NewResult(sb.String()), nil
}

func traverseRelatedEntities(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	entities, err := client.GetRelatedEntities(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		sb.WriteString("_No related entities found._\n")
		return nil
	}
	for i, e := range entities {
		fmt.Fprintf(sb, "%d. [%s] %q (kind: %s)\n", i+1, e.ID, e.Name, e.Kind)
		if e.Description != "" {
			fmt.Fprintf(sb, "   %s\n", Truncate(e.Description, 100))
		}
	}
	return nil
}

func traverseRelatedFacts(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	facts, err := client.GetFactsAboutEntity(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		sb.WriteString("_No related facts found._\n")
		return nil
	}
	for i, f := range facts {
		validStr := "valid"
		if !f.Valid {
			validStr = "invalidated"
		}
		fmt.Fprintf(sb, "%d. [%s] %q (category: %s, confidence: %.1f, %s)\n",
			i+1, f.ID, Truncate(f.Content, 100), f.Category, f.Confidence, validStr)
	}
	return nil
}

func traverseInvalidationChain(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	chain, err := client.GetInvalidationChain(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(chain) == 0 {
		sb.WriteString("_No invalidation chain found._\n")
		return nil
	}
	for i, inv := range chain {
		fmt.Fprintf(sb, "%d. [%s] -> [%s]\n", i+1, inv.NewFactID, inv.OldFactID)
		fmt.Fprintf(sb, "   Reason: %s\n", inv.Reason)
		if inv.OldContent != "" {
			fmt.Fprintf(sb, "   Old: %q\n", Truncate(inv.OldContent, 80))
		}
		if inv.NewContent != "" {
			fmt.Fprintf(sb, "   New: %q\n", Truncate(inv.NewContent, 80))
		}
	}
	return nil
}

func traverseDecisionEntities(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	entities, err := client.GetDecisionEntities(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		sb.WriteString("_No related entities found for this decision._\n")
		return nil
	}
	for i, e := range entities {
		fmt.Fprintf(sb, "%d. [%s] %q (kind: %s, role: %s)\n",
			i+1, e.ID, e.Name, e.Kind, e.Role)
	}
	return nil
}

func traverseEntityDecisions(ctx context.Context, client Querier, sb *strings.Builder, nodeID string) error {
	decisions, err := client.GetEntityDecisions(ctx, nodeID)
	if err != nil {
		return err
	}
	if len(decisions) == 0 {
		sb.WriteString("_No related decisions found for this entity._\n")
		return nil
	}
	for i, d := range decisions {
		fmt.Fprintf(sb, "%d. [%s] %q (status: %s)\n",
			i+1, d.ID, Truncate(d.Title, 100), d.Status)
	}
	return nil
}