//go:build cozodb

// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kraklabs/mie/pkg/memory"
	"github.com/kraklabs/mie/pkg/storage"
	flag "github.com/spf13/pflag"
)

func runDaemon(args []string, configPath string, globals GlobalFlags) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: mie daemon <start|stop|status>\n")
		os.Exit(ExitGeneral)
	}

	subcommand := args[0]

	switch subcommand {
	case "start":
		runDaemonStart(args[1:], configPath, globals)
	case "stop":
		runDaemonStop()
	case "status":
		runDaemonStatus()
	default:
		fmt.Fprintf(os.Stderr, "Unknown daemon subcommand: %s\n", subcommand)
		os.Exit(ExitGeneral)
	}
}

func runDaemonStart(args []string, configPath string, _ GlobalFlags) {
	fs := flag.NewFlagSet("daemon start", flag.ExitOnError)
	foreground := fs.Bool("foreground", false, "Run daemon in foreground (for debugging)")
	_ = fs.Parse(args)

	if !*foreground {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot find executable: %v\n", err)
			os.Exit(ExitGeneral)
		}

		cmdArgs := []string{"daemon", "start"}
		if configPath != "" {
			cmdArgs = append([]string{"--config", configPath}, cmdArgs...)
		}

		cmd := exec.Command(exe, cmdArgs...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot start daemon: %v\n", err)
			os.Exit(ExitGeneral)
		}

		// Reap the child in background to avoid zombie when daemon exits.
		go cmd.Wait()

		// Verify the process is still alive after startup period.
		time.Sleep(500 * time.Millisecond)
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: daemon process died during startup\n")
			os.Exit(ExitGeneral)
		}

		fmt.Fprintf(os.Stderr, "MIE daemon started (PID %d)\n", cmd.Process.Pid)
		return
	}

	// Foreground mode
	cfg, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		cfg = DefaultConfig()
		cfg.applyEnvOverrides()
	}

	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitConfig)
	}

	if err := os.MkdirAll(dataDir, 0750); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create data directory: %v\n", err)
		os.Exit(ExitDatabase)
	}

	embedded, err := storage.NewEmbeddedBackend(storage.EmbeddedConfig{
		DataDir:             dataDir,
		Engine:              cfg.Storage.Engine,
		EmbeddingDimensions: cfg.Embedding.Dimensions,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot open database: %v\n", err)
		os.Exit(ExitDatabase)
	}

	if err := embedded.EnsureSchema(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot ensure schema: %v\n", err)
		os.Exit(ExitDatabase)
	}

	dim := cfg.Embedding.Dimensions
	if dim <= 0 {
		dim = 768
	}
	if err := memory.EnsureSchema(embedded, dim); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot ensure MIE schema: %v\n", err)
		os.Exit(ExitDatabase)
	}

	if cfg.Embedding.Enabled {
		if err := memory.EnsureHNSWIndexes(embedded, dim); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot create HNSW indexes: %v\n", err)
			os.Exit(ExitDatabase)
		}
	}

	// Store embedding dimensions so clients can validate compatibility.
	if err := embedded.SetMeta("embedding_dimensions", strconv.Itoa(dim)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot store embedding dimensions: %v\n", err)
	}

	socketPath := storage.DefaultSocketPath()
	pidPath := storage.DefaultPIDPath()

	// Ensure parent directory exists — fatal if it fails since Serve
	// will also fail to bind the socket.
	if err := os.MkdirAll(filepath.Dir(socketPath), 0750); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create socket directory: %v\n", err)
		os.Exit(ExitGeneral)
	}

	// Acquire exclusive lock on PID file to prevent concurrent daemon starts.
	pidFile, err := os.OpenFile(pidPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot open PID file: %v\n", err)
		os.Exit(ExitGeneral)
	}
	if err := syscall.Flock(int(pidFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		pidFile.Close()
		fmt.Fprintf(os.Stderr, "Error: another daemon is already running (PID file locked)\n")
		os.Exit(ExitGeneral)
	}
	fmt.Fprintf(pidFile, "%d", os.Getpid())
	defer func() {
		_ = syscall.Flock(int(pidFile.Fd()), syscall.LOCK_UN)
		pidFile.Close()
		os.Remove(pidPath)
	}()

	daemon := storage.NewDaemon(embedded, socketPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// When running as PID 1 (e.g. in a container without tini), reap
	// orphaned child processes to prevent zombie accumulation.
	if os.Getpid() == 1 {
		reapChildren()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nMIE daemon received %s, shutting down...\n", sig)
		cancel()
	}()

	fmt.Fprintf(os.Stderr, "MIE daemon starting (PID %d)\n", os.Getpid())
	fmt.Fprintf(os.Stderr, "  Socket: %s\n", socketPath)
	fmt.Fprintf(os.Stderr, "  Storage: %s (%s)\n", cfg.Storage.Engine, dataDir)

	if err := daemon.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: daemon serve failed: %v\n", err)
		os.Exit(ExitGeneral)
	}

	_ = embedded.Close()
	fmt.Fprintf(os.Stderr, "MIE daemon stopped.\n")
}

func runDaemonStop() {
	pidPath := storage.DefaultPIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No running daemon found (no PID file at %s)\n", pidPath)
		os.Exit(ExitGeneral)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid PID file: %v\n", err)
		os.Exit(ExitGeneral)
	}

	// On Unix, FindProcess always succeeds. The actual check is the signal.
	proc, _ := os.FindProcess(pid)

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if strings.Contains(err.Error(), "no such process") {
			fmt.Fprintf(os.Stderr, "Daemon process %d not found (already stopped?)\n", pid)
			os.Remove(pidPath)
		} else {
			fmt.Fprintf(os.Stderr, "Cannot signal process %d: %v\n", pid, err)
		}
		os.Exit(ExitGeneral)
	}

	fmt.Fprintf(os.Stderr, "Sent SIGTERM to daemon (PID %d)\n", pid)
}

func runDaemonStatus() {
	socketPath := storage.DefaultSocketPath()
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		fmt.Printf("Daemon: not running (cannot connect to %s)\n", socketPath)
		return
	}
	conn.Close()

	pidPath := storage.DefaultPIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Printf("Daemon: running (socket available, no PID file)\n")
		return
	}

	fmt.Printf("Daemon: running (PID %s)\n", strings.TrimSpace(string(data)))
}

// connectOrStartDaemon tries to connect to the daemon socket.
// If the daemon is not running, starts it and retries the connection.
// Verifies the daemon is alive via Ping after connecting (handles stale sockets).
func connectOrStartDaemon(configPath string) (*storage.SocketBackend, error) {
	socketPath := storage.DefaultSocketPath()

	// Try connecting first and verify the daemon is alive.
	sb, err := storage.NewSocketBackend(socketPath)
	if err == nil {
		if pingErr := sb.Ping(); pingErr == nil {
			return sb, nil
		}
		// Stale socket — daemon not responding. Clean up and start fresh.
		sb.Close()
		os.Remove(socketPath)
	}

	// Daemon not running — start it
	fmt.Fprintf(os.Stderr, "Starting MIE daemon...\n")

	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find executable: %w", err)
	}

	cmdArgs := []string{"daemon", "start"}
	if configPath != "" {
		cmdArgs = append([]string{"--config", configPath}, cmdArgs...)
	}

	cmd := exec.Command(exe, cmdArgs...)
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start daemon: %w", err)
	}

	// Reap the child in background to avoid zombie when daemon exits.
	go cmd.Wait()

	// Verify process is still alive after a brief startup period.
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		return nil, fmt.Errorf("daemon process died immediately after start")
	}

	// Retry connection with backoff, verifying via Ping.
	delays := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, 1 * time.Second, 2 * time.Second}
	for _, delay := range delays {
		time.Sleep(delay)
		sb, err = storage.NewSocketBackend(socketPath)
		if err != nil {
			continue
		}
		if pingErr := sb.Ping(); pingErr != nil {
			sb.Close()
			continue
		}
		fmt.Fprintf(os.Stderr, "Connected to MIE daemon.\n")
		return sb, nil
	}

	return nil, fmt.Errorf("daemon started but cannot connect after retries: %w", err)
}

// reapChildren installs a SIGCHLD handler that reaps terminated child
// processes. This prevents zombie accumulation when the daemon runs as
// PID 1 in a container without an init process like tini.
func reapChildren() {
	sigchld := make(chan os.Signal, 32)
	signal.Notify(sigchld, syscall.SIGCHLD)
	go func() {
		for range sigchld {
			for {
				var ws syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
				if pid <= 0 || err != nil {
					break
				}
			}
		}
	}()
}
