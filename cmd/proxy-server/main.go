package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"frugal-llm/internal/config"
	"frugal-llm/internal/logger"
	"frugal-llm/internal/router"
)

var (
	Version   = "0.0.1"
	Commit    = "dev"
	BuildTime = "unknown"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "path to config file (defaults to config.yaml or FRUGAL_LLM_CONFIG)")
	flag.Parse()

	// 1. Load & Validate Configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Configuration Error: %v", err)
	}
	logger.SetLevel(cfg.LogLevel)

	logger.Infof("Starting Frugal LLM Proxy (Version: %s, Commit: %s, Built: %s)", Version, Commit, BuildTime)

	// 2. Initialize Router & Adapters
	r := router.NewRouter(cfg)

	// 3. Configure HTTP Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Extended timeout for long LLM streaming sessions
		IdleTimeout:  60 * time.Second,
	}

	// 4. Start Server in a background Goroutine
	go func() {
		logger.Infof("Server listening on http://localhost:%s", cfg.Port)
		logger.Infof("  -> Health Check:      http://localhost:%s/health", cfg.Port)
		logger.Infof("  -> Chat Completions:  http://localhost:%s/v1/chat/completions", cfg.Port)
		logger.Infof("  -> List Models:       http://localhost:%s/v1/models", cfg.Port)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("Server failed to start: %v", err)
			os.Exit(1)
		}
	}()

	// 5. Setup Graceful Shutdown Signal Interception
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-stop
	logger.Infof("Received signal '%v', initiating graceful shutdown...", sig)

	// Context with 10-second timeout for outstanding requests to complete
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Errorf("Server forced to shutdown: %v", err)
	}

	logger.Infof("Server gracefully stopped.")
}
