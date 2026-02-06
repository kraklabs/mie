//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestServer creates an MCP server backed by an in-memory CozoDB and
// returns a writer for sending requests and a reader for reading responses.
// The server runs in a background goroutine and stops when the writer is closed.
func startTestServer(t *testing.T) (io.WriteCloser, *bufio.Reader) {
	t.Helper()

	dir := t.TempDir()
	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:             dir,
		StorageEngine:       "mem",
		EmbeddingEnabled:    false,
		EmbeddingDimensions: 768,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	cfg := DefaultConfig()
	cfg.Storage.Engine = "mem"
	cfg.Embedding.Enabled = false

	server := &mcpServer{
		client: client,
		config: cfg,
	}

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	go func() {
		_ = server.serve(stdinReader, stdoutWriter)
		_ = stdoutWriter.Close()
	}()

	return stdinWriter, bufio.NewReader(stdoutReader)
}

// sendRequest writes a JSON-RPC request and reads the response line.
func sendRequest(t *testing.T, w io.Writer, r *bufio.Reader, id any, method string, params any) map[string]any {
	t.Helper()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		req["params"] = json.RawMessage(raw)
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)
	_, err = w.Write(append(data, '\n'))
	require.NoError(t, err)

	line, err := r.ReadString('\n')
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &resp))
	return resp
}

// sendNotification writes a JSON-RPC notification (no id, no response expected).
func sendNotification(t *testing.T, w io.Writer, method string, params any) {
	t.Helper()

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		req["params"] = json.RawMessage(raw)
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)
	_, err = w.Write(append(data, '\n'))
	require.NoError(t, err)
}

// initSession sends the initialize request and notifications/initialized
// notification, returning the initialize response.
func initSession(t *testing.T, w io.Writer, r *bufio.Reader) map[string]any {
	t.Helper()
	resp := sendRequest(t, w, r, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0.0.1"},
	})
	sendNotification(t, w, "notifications/initialized", nil)
	return resp
}

// callTool sends a tools/call request and returns the parsed response.
func callTool(t *testing.T, w io.Writer, r *bufio.Reader, id any, name string, args map[string]any) map[string]any {
	t.Helper()
	return sendRequest(t, w, r, id, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
}

func TestMCPInitialize(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	resp := sendRequest(t, w, r, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "0.0.1"},
	})

	assert.Equal(t, "2.0", resp["jsonrpc"])
	assert.Equal(t, float64(1), resp["id"])
	assert.Nil(t, resp["error"])

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "result should be a map")

	assert.Equal(t, "2024-11-05", result["protocolVersion"])

	serverInfo, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "mie", serverInfo["name"])
	assert.Equal(t, "0.1.0", serverInfo["version"])

	caps, ok := result["capabilities"].(map[string]any)
	require.True(t, ok)
	toolsCap, ok := caps["tools"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, toolsCap["listChanged"])
}

func TestMCPToolsList(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	resp := sendRequest(t, w, r, 2, "tools/list", nil)
	assert.Nil(t, resp["error"])

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)

	toolsList, ok := result["tools"].([]any)
	require.True(t, ok)
	assert.Len(t, toolsList, 9)

	expectedNames := map[string]bool{
		"mie_analyze":    false,
		"mie_store":      false,
		"mie_bulk_store": false,
		"mie_query":      false,
		"mie_update":     false,
		"mie_list":       false,
		"mie_conflicts":  false,
		"mie_export":     false,
		"mie_status":     false,
	}

	for _, tool := range toolsList {
		toolMap, ok := tool.(map[string]any)
		require.True(t, ok)

		name, ok := toolMap["name"].(string)
		require.True(t, ok)

		_, exists := expectedNames[name]
		assert.True(t, exists, "unexpected tool name: %s", name)
		expectedNames[name] = true

		// Verify each tool has a description and inputSchema
		assert.NotEmpty(t, toolMap["description"], "tool %s should have description", name)
		schema, ok := toolMap["inputSchema"].(map[string]any)
		require.True(t, ok, "tool %s should have inputSchema", name)
		assert.Equal(t, "object", schema["type"], "tool %s schema type should be object", name)
	}

	// Verify all expected tools were found
	for name, found := range expectedNames {
		assert.True(t, found, "expected tool %s not found", name)
	}
}

func TestMCPStatusEmptyDB(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_status", map[string]any{})
	assert.Nil(t, resp["error"])

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)

	content, ok := result["content"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, content)

	firstContent, ok := content[0].(map[string]any)
	require.True(t, ok)
	text, ok := firstContent["text"].(string)
	require.True(t, ok)

	// Verify zero counts in the status output
	assert.Contains(t, text, "Facts: 0")
	assert.Contains(t, text, "Decisions: 0")
	assert.Contains(t, text, "Entities: 0")
	assert.Contains(t, text, "Events: 0")
	assert.Contains(t, text, "Topics: 0")
	assert.Contains(t, text, "empty graph")
}

func TestMCPStoreAndList(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Store a fact
	storeResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "The sky is blue",
		"category":     "general",
		"confidence":   0.9,
		"source_agent": "test",
	})
	assert.Nil(t, storeResp["error"])

	storeResult := extractToolText(t, storeResp)
	assert.Contains(t, storeResult, "Stored fact")
	assert.Contains(t, storeResult, "The sky is blue")

	// List facts
	listResp := callTool(t, w, r, 3, "mie_list", map[string]any{
		"node_type": "fact",
	})
	assert.Nil(t, listResp["error"])

	listResult := extractToolText(t, listResp)
	assert.Contains(t, listResult, "1 total")
	assert.Contains(t, listResult, "The sky is blue")
}

func TestMCPStoreAndQuery(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Store an entity
	storeResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "entity",
		"name":         "Acme Corp",
		"kind":         "company",
		"description":  "A fictional company",
		"source_agent": "test",
	})
	assert.Nil(t, storeResp["error"])
	storeResult := extractToolText(t, storeResp)
	assert.Contains(t, storeResult, "Stored entity")

	// Query exact by name
	queryResp := callTool(t, w, r, 3, "mie_query", map[string]any{
		"query":      "Acme Corp",
		"mode":       "exact",
		"node_types": []string{"entity"},
	})
	assert.Nil(t, queryResp["error"])

	queryResult := extractToolText(t, queryResp)
	assert.Contains(t, queryResult, "Acme Corp")
	assert.Contains(t, queryResult, "Entities")
}

func TestMCPStoreAndUpdate(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Store the old fact
	storeResp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Water freezes at 10 degrees Celsius",
		"category":     "general",
		"confidence":   0.7,
		"source_agent": "test",
	})
	assert.Nil(t, storeResp["error"])

	storeText := extractToolText(t, storeResp)
	oldFactID := extractFactID(t, storeText)
	require.NotEmpty(t, oldFactID)

	// Store the replacement fact
	replaceResp := callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Water freezes at 0 degrees Celsius",
		"category":     "general",
		"confidence":   0.95,
		"source_agent": "test",
	})
	assert.Nil(t, replaceResp["error"])

	replaceText := extractToolText(t, replaceResp)
	newFactID := extractFactID(t, replaceText)
	require.NotEmpty(t, newFactID)

	// Invalidate the old fact via mie_update, providing replacement_id
	updateResp := callTool(t, w, r, 4, "mie_update", map[string]any{
		"node_id":        oldFactID,
		"action":         "invalidate",
		"reason":         "Incorrect temperature",
		"replacement_id": newFactID,
	})
	assert.Nil(t, updateResp["error"])

	updateResult := extractToolText(t, updateResp)
	assert.Contains(t, updateResult, "Invalidated")

	// List valid_only=true -- only the new fact should appear
	listResp := callTool(t, w, r, 5, "mie_list", map[string]any{
		"node_type":  "fact",
		"valid_only": true,
	})
	assert.Nil(t, listResp["error"])

	listResult := extractToolText(t, listResp)
	assert.Contains(t, listResult, "1 total")
	assert.Contains(t, listResult, "Water freezes at 0 degrees")
	assert.NotContains(t, listResult, "Water freezes at 10 degrees")
}

func TestMCPExport(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Store multiple nodes
	callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "fact",
		"content":      "Export test fact",
		"category":     "general",
		"source_agent": "test",
	})
	callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "entity",
		"name":         "Export Entity",
		"kind":         "project",
		"source_agent": "test",
	})

	// Export
	exportResp := callTool(t, w, r, 4, "mie_export", map[string]any{
		"format": "json",
	})
	assert.Nil(t, exportResp["error"])

	exportText := extractToolText(t, exportResp)

	// Parse the exported JSON to verify structure
	var exportData map[string]any
	require.NoError(t, json.Unmarshal([]byte(exportText), &exportData))

	assert.Contains(t, exportData, "version")
	assert.Contains(t, exportData, "exported_at")
	assert.Contains(t, exportData, "stats")

	// Verify facts contain our stored fact
	facts, ok := exportData["facts"].([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(facts), 1)

	// Verify entities contain our stored entity
	entities, ok := exportData["entities"].([]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(entities), 1)
}

func TestMCPAnalyze(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	resp := callTool(t, w, r, 2, "mie_analyze", map[string]any{
		"content":      "The user prefers dark mode in all applications",
		"content_type": "statement",
	})
	assert.Nil(t, resp["error"])

	text := extractToolText(t, resp)
	assert.Contains(t, text, "Evaluation Guide")
	assert.Contains(t, text, "Existing Memory Context")
	assert.Contains(t, text, "mie_store")
}

func TestMCPErrorHandling(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Call mie_store with missing required "type" field
	resp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"content": "some content without type",
	})
	assert.Nil(t, resp["error"]) // JSON-RPC level should succeed

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)

	content, ok := result["content"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, content)

	firstContent, ok := content[0].(map[string]any)
	require.True(t, ok)
	text, ok := firstContent["text"].(string)
	require.True(t, ok)
	assert.Contains(t, text, "Missing required parameter: type")

	isError, _ := result["isError"].(bool)
	assert.True(t, isError)
}

func TestMCPUnknownMethod(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	resp := sendRequest(t, w, r, 1, "unknown/method", nil)
	assert.Equal(t, "2.0", resp["jsonrpc"])

	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "response should have error")
	assert.Equal(t, float64(-32601), errObj["code"])
	assert.Equal(t, "Method not found", errObj["message"])
}

func TestMCPConflicts(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Call mie_conflicts on empty db -- should return without error
	resp := callTool(t, w, r, 2, "mie_conflicts", map[string]any{})
	assert.Nil(t, resp["error"])

	text := extractToolText(t, resp)
	// Should not be an error, just an empty or minimal result
	assert.NotEmpty(t, text)
}

func TestMCPStoreMultipleTypes(t *testing.T) {
	w, r := startTestServer(t)
	defer w.Close()

	initSession(t, w, r)

	// Store a decision
	resp := callTool(t, w, r, 2, "mie_store", map[string]any{
		"type":         "decision",
		"title":        "Use Go for the backend",
		"rationale":    "Performance and simplicity",
		"alternatives": "Rust, Python",
		"source_agent": "test",
	})
	assert.Nil(t, resp["error"])
	assert.Contains(t, extractToolText(t, resp), "Stored decision")

	// Store an event
	resp = callTool(t, w, r, 3, "mie_store", map[string]any{
		"type":         "event",
		"title":        "Project kickoff",
		"description":  "Initial meeting for the project",
		"event_date":   "2026-01-15",
		"source_agent": "test",
	})
	assert.Nil(t, resp["error"])
	assert.Contains(t, extractToolText(t, resp), "Stored event")

	// Store a topic
	resp = callTool(t, w, r, 4, "mie_store", map[string]any{
		"type":        "topic",
		"name":        "backend-architecture",
		"description": "Discussion about backend design choices",
	})
	assert.Nil(t, resp["error"])
	assert.Contains(t, extractToolText(t, resp), "Stored topic")

	// Verify status shows correct counts
	statusResp := callTool(t, w, r, 5, "mie_status", map[string]any{})
	statusText := extractToolText(t, statusResp)
	assert.Contains(t, statusText, "Decisions: 1")
	assert.Contains(t, statusText, "Events: 1")
	assert.Contains(t, statusText, "Topics: 1")
}

// --- helpers ---

// extractToolText extracts the text content from a tools/call response.
func extractToolText(t *testing.T, resp map[string]any) string {
	t.Helper()

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "response should have result")

	content, ok := result["content"].([]any)
	require.True(t, ok, "result should have content array")
	require.NotEmpty(t, content, "content array should not be empty")

	firstContent, ok := content[0].(map[string]any)
	require.True(t, ok)

	text, ok := firstContent["text"].(string)
	require.True(t, ok)
	return text
}

// extractFactID extracts a fact ID (fact:...) from tool response text.
func extractFactID(t *testing.T, text string) string {
	t.Helper()
	// The store response format is "Stored fact [fact:xxxxxxxx]"
	start := strings.Index(text, "[fact:")
	if start == -1 {
		t.Fatal("no fact ID found in text")
	}
	end := strings.Index(text[start:], "]")
	if end == -1 {
		t.Fatal("no closing bracket for fact ID")
	}
	return text[start+1 : start+end]
}