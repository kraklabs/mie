//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/kraklabs/mie/pkg/tools"
)

const (
	mcpVersion    = "0.1.0"
	mcpServerName = "mie"
)

// mieInstructions is the MCP instructions text sent to agents on initialize.
// It guides AI agents on how to use MIE effectively.
const mieInstructions = `MIE (Memory Intelligence Engine) gives you persistent memory across conversations. Use it to remember facts, decisions, entities, events, and topics about the user and their projects.

## When to capture memory

At the end of meaningful conversations, call mie_analyze with a summary of what was discussed. It will identify what is worth storing and return related existing memories. Then use mie_store or mie_bulk_store to persist the information.

## When to query memory

Before answering questions about past decisions, user preferences, project context, or previously discussed topics, query MIE first using mie_query. This lets you give informed, consistent responses grounded in what you actually know about the user.

## What to store

- Architecture and design decisions (with rationale and alternatives considered)
- Technology choices and preferences
- Project facts (team structure, conventions, deployment targets)
- Important events (releases, migrations, incidents)
- Entities (people, companies, projects, technologies the user works with)
- User preferences and working style

## What NOT to store

- Transient debugging details or one-off troubleshooting steps
- Code snippets or file contents (store the decision, not the code)
- Temporary task context that won't matter next conversation
- Information the user explicitly asks you not to remember

## Self-import from files

When the user asks to "import" knowledge from files (markdown, ADRs, READMEs, docs), you ARE the LLM — read the files directly using your file-reading tools and extract the knowledge yourself. No external processing is needed. Use mie_bulk_store (preferred for multiple items) or mie_store to persist what you extract.

### Importing ADRs (Architecture Decision Records)

Map ADR fields to MIE decision nodes:
- ADR title -> decision title
- ADR status (accepted/deprecated/superseded) -> decision status
- ADR context section -> decision context field
- ADR decision section -> decision rationale field
- ADR consequences -> store as related facts linked to the decision
- Mentioned technologies -> store as entities (kind: technology)

### Importing general markdown

- Headings suggest topics — create topic nodes for major themes
- "We decided" / "We chose" / "We use" language suggests decisions
- Technical tool and framework names suggest entities (kind: technology)
- People and team names suggest entities (kind: person, company)
- Factual statements about the project suggest facts
- Dates and milestones suggest events

## Self-import from git history

When the user asks to "import from git" or "learn from this repo's history", read the git log and extract implicit knowledge. Use your shell/command tools to run git commands and then store findings via mie_bulk_store.

### Step-by-step approach

1. Run 'git log --oneline -50' (or similar) to get recent commits
2. For deeper analysis, use 'git log --format="%H|%an|%ad|%s" --date=short -50'
3. Scan commit messages for patterns:
   - "feat:", "feature:" -> potential entities (features/products) and decisions
   - "fix:", "bugfix:" -> facts about past issues, potential event nodes
   - "refactor:", "chore:" -> technical decisions about code structure
   - "migrate", "upgrade", "switch to" -> technology decisions with alternatives
   - "add <tool/lib>" -> entity nodes (kind: technology)
   - PR merge commits -> decisions (the PR title is often the decision title)
4. For significant commits, use 'git show --stat <hash>' to see what files changed
5. Use 'git log --all --oneline --graph' to understand branching strategy (fact about workflow)

### What to extract

- **Entities**: Technologies, libraries, frameworks mentioned in commits (kind: technology)
- **Decisions**: Architecture changes, library adoptions, migrations (with commit as context)
- **Events**: Major releases (tags), migrations, incidents (with event_date from commit date)
- **Facts**: Team conventions visible in commit patterns (conventional commits, PR workflow, etc.)
- **Topics**: Recurring themes across commits (e.g., "performance", "security", "testing")

### Example bulk_store from git history

Given commits like:
- "feat: migrate from REST to GraphQL"
- "chore: upgrade React 17 -> 18"

Store as:
items: [
  {type: "entity", name: "GraphQL", kind: "technology", description: "API query language"},
  {type: "decision", title: "Migrate from REST to GraphQL", rationale: "Extracted from git commit history", context: "commit: abc123", relationships: [{edge: "decision_entity", target_ref: 0}]},
  {type: "entity", name: "React", kind: "technology", description: "Frontend UI framework"},
  {type: "event", title: "Upgrade React 17 to 18", event_date: "2025-06-15", description: "Major framework version upgrade", relationships: [{edge: "event_decision", target_ref: 3}]},
  {type: "decision", title: "Upgrade React to v18", rationale: "Version upgrade found in git history", context: "commit: def456"}
]

### Git tags as events

Run 'git tag -l --sort=-creatordate --format="%(creatordate:short) %(refname:short)"' to find releases. Each tag is a natural event node with the tag date as event_date.

### Cross-referencing

Use mie_bulk_store with the target_ref field in relationships to link items within the same batch by their array index (0-based). This avoids needing to know IDs ahead of time.`

// JSON-RPC 2.0 types for MCP protocol.

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpCapabilities struct {
	Tools     map[string]any `json:"tools,omitempty"`
	Resources map[string]any `json:"resources,omitempty"`
}

type mcpInitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    mcpCapabilities `json:"capabilities"`
	ServerInfo      mcpServerInfo   `json:"serverInfo"`
	Instructions    string          `json:"instructions,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpToolResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCP resource types.

type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type mcpResourcesListResult struct {
	Resources []mcpResource `json:"resources"`
}

type mcpResourceReadParams struct {
	URI string `json:"uri"`
}

type mcpResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type mcpResourceReadResult struct {
	Contents []mcpResourceContent `json:"contents"`
}

// mcpServer maintains state for the running MCP server instance.
type mcpServer struct {
	client tools.Querier
	config *Config
}

// toolHandler is the signature for MCP tool handlers.
type toolHandler func(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error)

// toolHandlers maps tool names to their handler functions.
var toolHandlers = map[string]toolHandler{
	"mie_analyze":    handleAnalyze,
	"mie_store":      handleStore,
	"mie_bulk_store": handleBulkStore,
	"mie_query":      handleQuery,
	"mie_update":     handleUpdate,
	"mie_list":       handleList,
	"mie_conflicts":  handleConflicts,
	"mie_export":     handleExport,
	"mie_status":     handleMIEStatus,
}

// runMCPServer starts the MIE MCP server on stdin/stdout.
func runMCPServer(configPath string) {
	var cfg *Config
	var err error

	cfg, err = LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using default configuration with environment variable overrides\n")
		cfg = DefaultConfig()
		cfg.applyEnvOverrides()
	}

	if cfg.Storage.Engine == "sqlite" {
		fmt.Fprintf(os.Stderr, "Warning: sqlite engine may not be available in pre-built binaries; consider using \"rocksdb\"\n")
	}

	// Resolve storage path
	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create data directory %s: %v\n", dataDir, err)
		os.Exit(ExitDatabase)
	}

	// Create the memory client (implements tools.Querier)
	// This opens CozoDB, ensures schema, and sets up embeddings.
	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:            dataDir,
		StorageEngine:      cfg.Storage.Engine,
		EmbeddingEnabled:   cfg.Embedding.Enabled,
		EmbeddingProvider:  cfg.Embedding.Provider,
		EmbeddingBaseURL:   cfg.Embedding.BaseURL,
		EmbeddingModel:     cfg.Embedding.Model,
		EmbeddingAPIKey:    cfg.Embedding.APIKey,
		EmbeddingDimensions: cfg.Embedding.Dimensions,
		EmbeddingWorkers:   cfg.Embedding.Workers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot initialize MIE: %v\n", err)
		os.Exit(ExitDatabase)
	}
	defer func() { _ = client.Close() }()

	server := &mcpServer{
		client: client,
		config: cfg,
	}

	fmt.Fprintf(os.Stderr, "MIE MCP Server v%s starting...\n", mcpVersion)
	fmt.Fprintf(os.Stderr, "  Storage: %s (%s)\n", cfg.Storage.Engine, dataDir)
	if cfg.Embedding.Enabled {
		fmt.Fprintf(os.Stderr, "  Embeddings: %s (%s, %dd)\n", cfg.Embedding.Provider, cfg.Embedding.Model, cfg.Embedding.Dimensions)
	}

	if err := server.serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: stdin read error: %v\n", err)
		os.Exit(ExitGeneral)
	}
}

// serve runs the JSON-RPC read loop, reading requests from r and writing responses to w.
func (s *mcpServer) serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid JSON-RPC request: %v\n", err)
			continue
		}

		fmt.Fprintf(os.Stderr, "-> %s\n", req.Method)

		ctx := context.Background()
		resp := s.handleRequest(ctx, req)

		if resp.ID == nil && resp.Result == nil && resp.Error == nil {
			continue
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot encode response: %v\n", err)
			continue
		}

		_, _ = fmt.Fprintf(w, "%s\n", respBytes)

		fmt.Fprintf(os.Stderr, "<- response sent for %s\n", req.Method)
	}

	return scanner.Err()
}

// handleRequest dispatches a JSON-RPC request to the appropriate handler.
func (s *mcpServer) handleRequest(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpInitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities: mcpCapabilities{
					Tools:     map[string]any{"listChanged": true},
					Resources: map[string]any{"listChanged": false},
				},
				ServerInfo: mcpServerInfo{
					Name:    mcpServerName,
					Version: mcpVersion,
				},
				Instructions: mieInstructions,
			},
		}

	case "notifications/initialized":
		return jsonRPCResponse{}

	case "tools/list":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpToolsListResult{
				Tools: s.getTools(),
			},
		}

	case "tools/call":
		var params mcpToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32602,
					Message: "Invalid params",
					Data:    err.Error(),
				},
			}
		}

		result, err := s.handleToolCall(ctx, params)
		if err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32603,
					Message: "Internal error",
					Data:    err.Error(),
				},
			}
		}

		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	case "resources/list":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpResourcesListResult{
				Resources: []mcpResource{
					{
						URI:         "mie://context/recent",
						Name:        "Recent memory context",
						Description: "Latest facts, decisions, and entities from the memory graph",
						MimeType:    "text/plain",
					},
				},
			},
		}

	case "resources/read":
		var params mcpResourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32602,
					Message: "Invalid params",
					Data:    err.Error(),
				},
			}
		}

		if params.URI != "mie://context/recent" {
			return jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &rpcError{
					Code:    -32602,
					Message: "Unknown resource",
					Data:    params.URI,
				},
			}
		}

		text := s.buildRecentContext(ctx)
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpResourceReadResult{
				Contents: []mcpResourceContent{
					{
						URI:      params.URI,
						MimeType: "text/plain",
						Text:     text,
					},
				},
			},
		}

	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32601,
				Message: "Method not found",
				Data:    req.Method,
			},
		}
	}
}

// handleToolCall dispatches a tool call to the registered handler.
func (s *mcpServer) handleToolCall(ctx context.Context, params mcpToolCallParams) (*mcpToolResult, error) {
	handler, ok := toolHandlers[params.Name]
	if !ok {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", params.Name)}},
			IsError: true,
		}, nil
	}

	result, err := handler(ctx, s, params.Arguments)
	if err != nil {
		return &mcpToolResult{
			Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("Error in %s: %v", params.Name, err)}},
			IsError: true,
		}, nil
	}

	return &mcpToolResult{
		Content: []mcpContent{{Type: "text", Text: result.Text}},
		IsError: result.IsError,
	}, nil
}

// getTools returns the list of all MIE MCP tool definitions.
func (s *mcpServer) getTools() []mcpTool {
	return []mcpTool{
		{
			Name:        "mie_analyze",
			Description: "Analyze a conversation fragment for potential memory storage. Returns related existing memory and an evaluation guide for the agent to decide what to persist. Call this at the end of meaningful conversations or when noticing something worth remembering.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":        "string",
						"description": "Conversation fragment or information to analyze for potential memory storage",
					},
					"content_type": map[string]any{
						"type":        "string",
						"enum":        []string{"conversation", "statement", "decision", "event"},
						"description": "Type of content being analyzed. Helps focus the search.",
						"default":     "conversation",
					},
				},
				"required": []string{"content"},
			},
		},
		{
			Name:        "mie_store",
			Description: "Store a new memory node (fact, decision, entity, event, or topic) in the memory graph. Use after mie_analyze confirms something is worth persisting.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"fact", "decision", "entity", "event", "topic"},
						"description": "Type of memory node to store",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Fact content text (required for type=fact)",
					},
					"category": map[string]any{
						"type":        "string",
						"enum":        []string{"personal", "professional", "preference", "technical", "relationship", "general"},
						"description": "Fact category",
						"default":     "general",
					},
					"confidence": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "Confidence level (0.0-1.0)",
						"default":     0.8,
					},
					"title": map[string]any{
						"type":        "string",
						"description": "Decision or event title (required for type=decision, type=event)",
					},
					"rationale": map[string]any{
						"type":        "string",
						"description": "Decision rationale (required for type=decision)",
					},
					"alternatives": map[string]any{
						"type":        "string",
						"description": "JSON array of alternatives considered (for decisions)",
					},
					"context": map[string]any{
						"type":        "string",
						"description": "Decision context",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "Entity or topic name (required for type=entity, type=topic)",
					},
					"kind": map[string]any{
						"type":        "string",
						"enum":        []string{"person", "company", "project", "product", "technology", "place", "other"},
						"description": "Entity kind (required for type=entity)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Description for entity, event, or topic",
					},
					"event_date": map[string]any{
						"type":        "string",
						"description": "Event date in ISO format (e.g., 2026-02-05). Required for type=event.",
					},
					"source_agent": map[string]any{
						"type":        "string",
						"description": "Agent identifier (e.g., 'claude', 'cursor')",
						"default":     "unknown",
					},
					"source_conversation": map[string]any{
						"type":        "string",
						"description": "Conversation reference or identifier",
					},
					"relationships": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"edge": map[string]any{
									"type":        "string",
									"enum":        []string{"fact_entity", "fact_topic", "decision_topic", "decision_entity", "event_decision", "entity_topic"},
									"description": "Relationship type",
								},
								"target_id": map[string]any{
									"type":        "string",
									"description": "Target node ID",
								},
								"role": map[string]any{
									"type":        "string",
									"description": "Role description (for decision_entity edges)",
								},
							},
							"required": []string{"edge", "target_id"},
						},
						"description": "Relationships to create after storing",
					},
					"invalidates": map[string]any{
						"type":        "string",
						"description": "ID of a fact to invalidate (marks it as invalid and creates invalidation edge)",
					},
				},
				"required": []string{"type"},
			},
		},
		{
			Name:        "mie_bulk_store",
			Description: "Store multiple memory nodes in a single call. Preferred over repeated mie_store calls when importing or capturing multiple items. Supports cross-batch relationships via target_ref (0-based index into the items array).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type": map[string]any{
									"type":        "string",
									"enum":        []string{"fact", "decision", "entity", "event", "topic"},
									"description": "Type of memory node to store",
								},
								"content": map[string]any{
									"type":        "string",
									"description": "Fact content text (required for type=fact)",
								},
								"category": map[string]any{
									"type":        "string",
									"enum":        []string{"personal", "professional", "preference", "technical", "relationship", "general"},
									"description": "Fact category",
									"default":     "general",
								},
								"confidence": map[string]any{
									"type":        "number",
									"minimum":     0,
									"maximum":     1,
									"description": "Confidence level (0.0-1.0)",
									"default":     0.8,
								},
								"title": map[string]any{
									"type":        "string",
									"description": "Decision or event title (required for type=decision, type=event)",
								},
								"rationale": map[string]any{
									"type":        "string",
									"description": "Decision rationale (required for type=decision)",
								},
								"alternatives": map[string]any{
									"type":        "string",
									"description": "JSON array of alternatives considered (for decisions)",
								},
								"context": map[string]any{
									"type":        "string",
									"description": "Decision context",
								},
								"name": map[string]any{
									"type":        "string",
									"description": "Entity or topic name (required for type=entity, type=topic)",
								},
								"kind": map[string]any{
									"type":        "string",
									"enum":        []string{"person", "company", "project", "product", "technology", "place", "other"},
									"description": "Entity kind (required for type=entity)",
								},
								"description": map[string]any{
									"type":        "string",
									"description": "Description for entity, event, or topic",
								},
								"event_date": map[string]any{
									"type":        "string",
									"description": "Event date in ISO format (e.g., 2026-02-05). Required for type=event.",
								},
								"source_agent": map[string]any{
									"type":        "string",
									"description": "Agent identifier (e.g., 'claude', 'cursor')",
									"default":     "unknown",
								},
								"source_conversation": map[string]any{
									"type":        "string",
									"description": "Conversation reference or identifier",
								},
								"relationships": map[string]any{
									"type": "array",
									"items": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"edge": map[string]any{
												"type":        "string",
												"enum":        []string{"fact_entity", "fact_topic", "decision_topic", "decision_entity", "event_decision", "entity_topic"},
												"description": "Relationship type",
											},
											"target_id": map[string]any{
												"type":        "string",
												"description": "Target node ID (use target_ref for cross-batch references)",
											},
											"target_ref": map[string]any{
												"type":        "number",
												"description": "0-based index of another item in this batch to link to (alternative to target_id)",
											},
											"role": map[string]any{
												"type":        "string",
												"description": "Role description (for decision_entity edges)",
											},
										},
										"required": []string{"edge"},
									},
									"description": "Relationships to create after storing",
								},
								"invalidates": map[string]any{
									"type":        "string",
									"description": "ID of a fact to invalidate (marks it as invalid and creates invalidation edge)",
								},
							},
							"required": []string{"type"},
						},
						"description": "Array of memory nodes to store (max 50)",
					},
				},
				"required": []string{"items"},
			},
		},
		{
			Name:        "mie_query",
			Description: "Search the memory graph. Supports three modes: 'semantic' (natural language similarity search), 'exact' (substring match), and 'graph' (traverse relationships from a node).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query. Natural language for semantic mode, exact text for exact mode, or node ID for graph mode.",
					},
					"mode": map[string]any{
						"type":        "string",
						"enum":        []string{"semantic", "exact", "graph"},
						"description": "Search mode",
						"default":     "semantic",
					},
					"node_types": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": []string{"fact", "decision", "entity", "event"}},
						"description": "Node types to search (default: all)",
					},
					"limit": map[string]any{
						"type":    "number",
						"minimum": 1,
						"maximum": 50,
						"default": 10,
					},
					"category": map[string]any{
						"type":        "string",
						"description": "Filter facts by category",
					},
					"kind": map[string]any{
						"type":        "string",
						"description": "Filter entities by kind",
					},
					"valid_only": map[string]any{
						"type":    "boolean",
						"default": true,
					},
					"node_id": map[string]any{
						"type":        "string",
						"description": "Node ID for graph traversal mode",
					},
					"traversal": map[string]any{
						"type":        "string",
						"enum":        []string{"related_entities", "related_facts", "invalidation_chain", "decision_entities", "facts_about_entity", "entity_decisions"},
						"description": "Traversal type for graph mode",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "mie_update",
			Description: "Update or invalidate existing memory nodes. For facts, invalidation creates a chain (old fact marked invalid, linked to new). For entities, update description. For decisions, change status.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_id": map[string]any{
						"type":        "string",
						"description": "ID of the node to modify",
					},
					"action": map[string]any{
						"type":        "string",
						"enum":        []string{"invalidate", "update_description", "update_status"},
						"description": "Action: invalidate a fact, update an entity description, or change a decision status",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Why this change is being made (required for invalidation)",
					},
					"replacement_id": map[string]any{
						"type":        "string",
						"description": "ID of the new fact that replaces the invalidated one",
					},
					"new_value": map[string]any{
						"type":        "string",
						"description": "New value for update_description or update_status actions",
					},
				},
				"required": []string{"node_id", "action"},
			},
		},
		{
			Name:        "mie_list",
			Description: "List memory nodes with filtering, pagination, and sorting. Returns a formatted table of results.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"node_type": map[string]any{
						"type":        "string",
						"enum":        []string{"fact", "decision", "entity", "event", "topic"},
						"description": "Type of memory nodes to list",
					},
					"category": map[string]any{
						"type":        "string",
						"description": "Filter facts by category",
					},
					"kind": map[string]any{
						"type":        "string",
						"description": "Filter entities by kind",
					},
					"status": map[string]any{
						"type":        "string",
						"description": "Filter decisions by status (active, superseded, reversed)",
					},
					"topic": map[string]any{
						"type":        "string",
						"description": "Filter by topic name",
					},
					"valid_only": map[string]any{
						"type":    "boolean",
						"default": true,
					},
					"limit": map[string]any{
						"type":    "number",
						"minimum": 1,
						"maximum": 100,
						"default": 20,
					},
					"offset": map[string]any{
						"type":    "number",
						"minimum": 0,
						"default": 0,
					},
					"sort_by": map[string]any{
						"type":        "string",
						"description": "Sort field (created_at, updated_at, name)",
						"default":     "created_at",
					},
					"sort_order": map[string]any{
						"type":        "string",
						"enum":        []string{"asc", "desc"},
						"default":     "desc",
					},
				},
				"required": []string{"node_type"},
			},
		},
		{
			Name:        "mie_conflicts",
			Description: "Detect potentially contradicting facts in the memory graph. Returns pairs of facts that are semantically similar but may contain conflicting information. Use this to maintain memory consistency.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category": map[string]any{
						"type":        "string",
						"description": "Limit conflict scan to a specific category",
					},
					"threshold": map[string]any{
						"type":        "number",
						"minimum":     0,
						"maximum":     1,
						"description": "Similarity threshold (0.0-1.0). Higher = stricter matching.",
						"default":     0.85,
					},
					"limit": map[string]any{
						"type":    "number",
						"minimum": 1,
						"maximum": 50,
						"default": 10,
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "mie_export",
			Description: "Export the complete memory graph for backup or migration. Returns all nodes and relationships in structured format.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": map[string]any{
						"type":        "string",
						"enum":        []string{"json", "datalog"},
						"description": "Export format",
						"default":     "json",
					},
					"include_embeddings": map[string]any{
						"type":        "boolean",
						"description": "Include embedding vectors (can be very large)",
						"default":     false,
					},
					"node_types": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": []string{"fact", "decision", "entity", "event", "topic"}},
						"description": "Types to export (default: all)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "mie_status",
			Description: "Display memory graph health and statistics. Shows counts of all node types, configuration details, and health checks.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
		},
	}
}

// Tool handler implementations — each delegates to the corresponding pkg/tools function
// passing the Querier client and the raw arguments map.

func handleAnalyze(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Analyze(ctx, s.client, args)
}

func handleStore(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Store(ctx, s.client, args)
}

func handleQuery(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Query(ctx, s.client, args)
}

func handleUpdate(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Update(ctx, s.client, args)
}

func handleList(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.List(ctx, s.client, args)
}

func handleConflicts(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Conflicts(ctx, s.client, args)
}

func handleExport(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Export(ctx, s.client, args)
}

func handleBulkStore(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.BulkStore(ctx, s.client, args)
}

func handleMIEStatus(ctx context.Context, s *mcpServer, args map[string]any) (*tools.ToolResult, error) {
	return tools.Status(ctx, s.client, args)
}

// buildRecentContext queries the memory graph for recent facts, decisions, and entities,
// and formats them as a concise markdown summary for the mie://context/recent resource.
func (s *mcpServer) buildRecentContext(ctx context.Context) string {
	var sb strings.Builder
	sb.WriteString("# Recent Memory Context\n\n")

	// Recent facts.
	facts, _, err := s.client.ListNodes(ctx, tools.ListOptions{
		NodeType:  "fact",
		ValidOnly: true,
		Limit:     5,
		SortBy:    "created_at",
		SortOrder: "desc",
	})
	if err == nil && len(facts) > 0 {
		sb.WriteString("## Recent Facts\n")
		for _, node := range facts {
			if f, ok := node.(*tools.Fact); ok {
				sb.WriteString(fmt.Sprintf("- [%s] %s\n", f.Category, f.Content))
			}
		}
		sb.WriteString("\n")
	}

	// Recent decisions.
	decisions, _, err := s.client.ListNodes(ctx, tools.ListOptions{
		NodeType:  "decision",
		Limit:     3,
		SortBy:    "created_at",
		SortOrder: "desc",
	})
	if err == nil && len(decisions) > 0 {
		sb.WriteString("## Recent Decisions\n")
		for _, node := range decisions {
			if d, ok := node.(*tools.Decision); ok {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", d.Title, d.Rationale))
			}
		}
		sb.WriteString("\n")
	}

	// Recent entities.
	entities, _, err := s.client.ListNodes(ctx, tools.ListOptions{
		NodeType:  "entity",
		Limit:     5,
		SortBy:    "created_at",
		SortOrder: "desc",
	})
	if err == nil && len(entities) > 0 {
		sb.WriteString("## Known Entities\n")
		for _, node := range entities {
			if e, ok := node.(*tools.Entity); ok {
				desc := e.Description
				if desc == "" {
					desc = e.Kind
				}
				sb.WriteString(fmt.Sprintf("- %s (%s)\n", e.Name, desc))
			}
		}
		sb.WriteString("\n")
	}

	if sb.Len() == len("# Recent Memory Context\n\n") {
		sb.WriteString("No memories stored yet.\n")
	}

	return sb.String()
}