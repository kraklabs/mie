//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

// Package main implements the MIE CLI for managing the Memory Intelligence Engine.
//
// Usage:
//
//	mie --mcp                     Start as MCP server (JSON-RPC over stdio)
//	mie init                      Create .mie/config.yaml configuration
//	mie status [--json]           Show memory graph status
//	mie reset --yes               Delete all memory data
//	mie export [--format json]    Export memory graph
//	mie import [--format json]    Import memory graph
//	mie query <script>            Execute CozoScript query
package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
)

// Exit codes for the MIE CLI.
const (
	ExitSuccess  = 0
	ExitGeneral  = 1
	ExitConfig   = 2
	ExitDatabase = 3
	ExitQuery    = 4
)

// Version information (set via ldflags during build).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// GlobalFlags holds the global CLI flags that apply to all commands.
type GlobalFlags struct {
	JSON    bool
	Verbose int
	Quiet   bool
}

func main() {
	var (
		showVersion = flag.BoolP("version", "V", false, "Show version and exit")
		mcpMode     = flag.Bool("mcp", false, "Start as MCP server (JSON-RPC over stdio)")
		configPath  = flag.StringP("config", "c", "", "Path to .mie/config.yaml")
		jsonOutput  = flag.Bool("json", false, "Output in JSON format")
		verbose     = flag.CountP("verbose", "v", "Increase verbosity (-v info, -vv debug)")
		quiet       = flag.BoolP("quiet", "q", false, "Suppress non-essential output")
	)

	flag.SetInterspersed(false)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `MIE - Memory Intelligence Engine

MIE is a local-first personal memory graph for AI agents.
It provides semantic memory of facts, decisions, entities, events,
and their relationships, accessible via MCP (Model Context Protocol).

Usage:
  mie <command> [options]

Commands:
  init          Create .mie/config.yaml configuration
  status        Show memory graph status
  reset         Delete all memory data (destructive!)
  export        Export memory graph
  import        Import memory graph
  query         Execute CozoScript query (debugging)

Global Options:
  --json            Output in JSON format
  -v, --verbose     Increase verbosity (-v info, -vv debug)
  -q, --quiet       Suppress non-essential output
  --mcp             Start as MCP server (JSON-RPC over stdio)
  -c, --config      Path to .mie/config.yaml
  -V, --version     Show version and exit

Examples:
  mie init                         Create configuration
  mie --mcp                        Start MCP server
  mie status                       Show memory stats
  mie status --json                Output as JSON
  mie export --format json         Export all data
  mie import --input backup.json   Import from file
  mie query "?[name] := *mie_entity{name} :limit 10"

Getting Started:
  1. Initialize configuration:  mie init
  2. Start MCP server:          mie --mcp
  3. Configure your AI client to use MIE as an MCP server

Environment Variables:
  MIE_CONFIG_PATH       Path to config file
  MIE_STORAGE_ENGINE    Storage engine (sqlite, rocksdb, mem)
  MIE_STORAGE_PATH      Database file path
  MIE_EMBEDDING_ENABLED Enable embeddings (true/false)
  OLLAMA_HOST           Ollama URL (default: http://localhost:11434)
  OLLAMA_EMBED_MODEL    Embedding model (default: nomic-embed-text)

`)
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("mie version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built: %s\n", date)
		os.Exit(0)
	}

	if *quiet && *verbose > 0 {
		fmt.Fprintf(os.Stderr, "Error: cannot use --quiet and --verbose together\n")
		os.Exit(1)
	}

	if *jsonOutput {
		*quiet = true
	}

	globals := GlobalFlags{
		JSON:    *jsonOutput,
		Verbose: *verbose,
		Quiet:   *quiet,
	}

	if *mcpMode {
		runMCPServer(*configPath)
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]
	cmdArgs := args[1:]

	switch command {
	case "init":
		runInit(cmdArgs, globals)
	case "status":
		runStatus(cmdArgs, *configPath, globals)
	case "reset":
		runReset(cmdArgs, *configPath, globals)
	case "export":
		runExport(cmdArgs, *configPath, globals)
	case "import":
		runImport(cmdArgs, *configPath, globals)
	case "query":
		runQuery(cmdArgs, *configPath, globals)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}
