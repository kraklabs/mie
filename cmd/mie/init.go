//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/kraklabs/mie/pkg/tools"
)

// runInit creates a new .mie/config.yaml configuration file.
func runInit(args []string, globals GlobalFlags) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing configuration")
	interview := fs.Bool("interview", false, "Run interactive onboarding to pre-populate memory")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: mie init [options]

Description:
  Create a new .mie/config.yaml configuration file in the current directory
  with sensible defaults.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  mie init                  Create configuration with defaults
  mie init --force          Overwrite existing configuration
  mie init --interview      Create config and pre-populate memory

`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}

	configPath := ConfigPath(cwd)

	if _, err := os.Stat(configPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "Error: %s already exists\n", configPath)
		fmt.Fprintf(os.Stderr, "Use --force to overwrite\n")
		os.Exit(1)
	}

	cfg := DefaultConfig()
	if err := SaveConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	if !globals.Quiet {
		fmt.Printf("Created %s\n", configPath)
	}

	if *interview {
		runInterview(cfg, globals)
	} else if !globals.Quiet {
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Edit .mie/config.yaml to customize settings")
		fmt.Println("  2. Run 'mie --mcp' to start the MCP server")
	}
}

// runInterview asks interactive questions and pre-populates the memory graph.
func runInterview(cfg *Config, globals GlobalFlags) {
	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
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
	reader := bufio.NewReader(os.Stdin)

	var entityCount, factCount, topicCount int
	var topicIDs []string

	fmt.Println()
	fmt.Println("Let's set up your memory graph.")
	fmt.Println()

	// Project name
	if name := prompt(reader, "Project name?"); name != "" {
		_, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
			Name:        name,
			Kind:        "project",
			Description: name + " project",
			SourceAgent: "mie-init",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store project entity: %v\n", err)
		} else {
			entityCount++
		}
	}

	// Primary language
	if lang := prompt(reader, "Primary language? (e.g., Go, Python, TypeScript)"); lang != "" {
		_, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
			Name:        lang,
			Kind:        "technology",
			Description: lang + " programming language",
			SourceAgent: "mie-init",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store language entity: %v\n", err)
		} else {
			entityCount++
		}
	}

	// Database
	if db := prompt(reader, "Database? (e.g., PostgreSQL, MongoDB, none)"); db != "" && !isNone(db) {
		_, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
			Name:        db,
			Kind:        "technology",
			Description: db + " database",
			SourceAgent: "mie-init",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store database entity: %v\n", err)
		} else {
			entityCount++
		}
	}

	// Cloud provider
	if cloud := prompt(reader, "Cloud provider? (e.g., AWS, GCP, none)"); cloud != "" && !isNone(cloud) {
		_, err := client.StoreEntity(ctx, tools.StoreEntityRequest{
			Name:        cloud,
			Kind:        "technology",
			Description: cloud + " cloud provider",
			SourceAgent: "mie-init",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store cloud entity: %v\n", err)
		} else {
			entityCount++
		}
	}

	// Team size
	if size := prompt(reader, "Team size? (e.g., 1, 5, 20)"); size != "" {
		_, err := client.StoreFact(ctx, tools.StoreFactRequest{
			Content:     "Team size is " + size,
			Category:    "professional",
			Confidence:  0.9,
			SourceAgent: "mie-init",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to store team size fact: %v\n", err)
		} else {
			factCount++
		}
	}

	// Topics
	if topicsStr := prompt(reader, "Main topics? (comma-separated, e.g., backend,api,auth)"); topicsStr != "" {
		for _, t := range strings.Split(topicsStr, ",") {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			tp, err := client.StoreTopic(ctx, tools.StoreTopicRequest{
				Name:        t,
				Description: t + " topic",
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to store topic %q: %v\n", t, err)
			} else {
				topicCount++
				topicIDs = append(topicIDs, tp.ID)
			}
		}
	}

	_ = topicIDs // relationships could be added here in the future

	if !globals.Quiet {
		fmt.Println()
		fmt.Printf("Stored %d entities, %d facts, %d topics\n", entityCount, factCount, topicCount)
		fmt.Println("Your memory graph is ready! Run 'mie --mcp' to start the server.")
	}
}

// prompt prints a question and reads a trimmed line from the reader.
func prompt(reader *bufio.Reader, question string) string {
	fmt.Printf("  %s ", question)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// isNone returns true if the answer indicates no value.
func isNone(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "none" || s == "n/a" || s == "-" || s == ""
}
