//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/kraklabs/mie/pkg/tools"
)

// runImport imports data from a JSON or Datalog export file into the memory graph.
func runImport(args []string, configPath string, globals GlobalFlags) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	format := fs.String("format", "json", "Import format: json or datalog")
	input := fs.StringP("input", "i", "", "Input file path (default: stdin)")
	dryRun := fs.Bool("dry-run", false, "Preview what would be imported without writing")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie import [options]

Description:
  Import data from a JSON or Datalog export file into the memory graph.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  mie import --input memory.json              Import from JSON file
  mie import --input backup.json --dry-run    Preview import
  mie import --format datalog --input data.dl Import Datalog
  cat memory.json | mie import                Import from stdin

`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *format != "json" && *format != "datalog" {
		fmt.Fprintf(os.Stderr, "Error: unsupported format %q (supported: json, datalog)\n", *format)
		os.Exit(ExitGeneral)
	}

	// Read input data.
	var data []byte
	var err error
	if *input != "" {
		data, err = os.ReadFile(*input) //nolint:gosec // G304: Path comes from user flag
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot read %s: %v\n", *input, err)
			os.Exit(ExitGeneral)
		}
	} else {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot read stdin: %v\n", err)
			os.Exit(ExitGeneral)
		}
	}

	if len(data) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no input data\n")
		os.Exit(ExitGeneral)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		cfg = DefaultConfig()
		cfg.applyEnvOverrides()
	}

	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: no data found at %s\n", dataDir)
		os.Exit(ExitDatabase)
	}

	client, err := memory.NewClient(memory.ClientConfig{
		DataDir:       dataDir,
		StorageEngine: cfg.Storage.Engine,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot open database: %v\n", err)
		os.Exit(ExitDatabase)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	switch *format {
	case "json":
		importJSON(ctx, client, data, *dryRun, globals)
	case "datalog":
		importDatalog(ctx, client, data, *dryRun, globals)
	}
}

func importJSON(ctx context.Context, client *memory.Client, data []byte, dryRun bool, globals GlobalFlags) {
	var export tools.ExportData
	if err := json.Unmarshal(data, &export); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON: %v\n", err)
		os.Exit(ExitGeneral)
	}

	counts := map[string]int{
		"facts":     len(export.Facts),
		"decisions": len(export.Decisions),
		"entities":  len(export.Entities),
		"events":    len(export.Events),
		"topics":    len(export.Topics),
	}

	if dryRun {
		fmt.Println("Dry run — would import:")
		for kind, n := range counts {
			if n > 0 {
				fmt.Printf("  %d %s\n", n, kind)
			}
		}
		return
	}

	for _, f := range export.Facts {
		_, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:            f.Content,
			Category:           f.Category,
			Confidence:         f.Confidence,
			SourceAgent:        f.SourceAgent,
			SourceConversation: f.SourceConversation,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to import fact: %v\n", err)
		}
	}

	for _, d := range export.Decisions {
		_, err := client.StoreDecision(ctx, tools.StoreDecisionRequest{
			Title:              d.Title,
			Rationale:          d.Rationale,
			Alternatives:       d.Alternatives,
			Context:            d.Context,
			SourceAgent:        d.SourceAgent,
			SourceConversation: d.SourceConversation,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to import decision %q: %v\n", d.Title, err)
		}
	}

	for _, e := range export.Entities {
		_, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
			Name:        e.Name,
			Kind:        e.Kind,
			Description: e.Description,
			SourceAgent: e.SourceAgent,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to import entity %q: %v\n", e.Name, err)
		}
	}

	for _, ev := range export.Events {
		_, err := client.StoreEvent(ctx, tools.StoreEventRequest{
			Title:              ev.Title,
			Description:        ev.Description,
			EventDate:          ev.EventDate,
			SourceAgent:        ev.SourceAgent,
			SourceConversation: ev.SourceConversation,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to import event %q: %v\n", ev.Title, err)
		}
	}

	for _, tp := range export.Topics {
		_, err := client.StoreTopic(ctx, tools.StoreTopicRequest{
			Name:        tp.Name,
			Description: tp.Description,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to import topic %q: %v\n", tp.Name, err)
		}
	}

	if !globals.Quiet {
		fmt.Printf("Imported %d facts, %d decisions, %d entities, %d events, %d topics\n",
			counts["facts"], counts["decisions"], counts["entities"], counts["events"], counts["topics"])
	}
}

func importDatalog(ctx context.Context, client *memory.Client, data []byte, dryRun bool, globals GlobalFlags) {
	script := string(data)

	if dryRun {
		fmt.Println("Dry run — would execute CozoScript:")
		fmt.Println(script)
		return
	}

	_, err := client.RawQuery(ctx, script)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: CozoScript execution failed: %v\n", err)
		os.Exit(ExitQuery)
	}

	if !globals.Quiet {
		fmt.Println("Datalog import completed successfully")
	}
}