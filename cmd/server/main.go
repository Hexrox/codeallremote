// Package main is the entry point for the CAR server.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/code-all-remote/car/internal/app"
	"github.com/code-all-remote/car/internal/config"
	"github.com/code-all-remote/car/internal/server"
)

var version = "dev"

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to configuration file (required)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("CAR Server %s\n", version)
		os.Exit(0)
	}

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		fmt.Fprintln(os.Stderr, "usage: car-server --config /path/to/config.json")
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	logger, err := createLogger(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating logger: %v\n", err)
		os.Exit(1)
	}

	// Compose the application (core services + adapters).
	ctx := context.Background()
	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("failed to compose application", "error", err)
		os.Exit(1)
	}
	if err := application.Start(ctx); err != nil {
		logger.Error("application startup failed", "error", err)
		os.Exit(1)
	}

	// Create and run server, wired to the application.
	srv := server.NewWithApp(cfg, application, logger)

	if err := srv.Run(); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func createLogger(cfg config.LoggingConfig) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var writer io.Writer
	switch strings.ToLower(cfg.Output) {
	case "stdout", "":
		writer = os.Stdout
	case "stderr":
		writer = os.Stderr
	default:
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("opening log file: %w", err)
		}
		writer = f
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	case "text", "":
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	}

	return slog.New(handler), nil
}
