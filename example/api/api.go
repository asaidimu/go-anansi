package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/example/api/handlers"
	"github.com/asaidimu/go-anansi/v6/example/api/middleware"
	"github.com/asaidimu/go-anansi/v6/example/api/server"
	"github.com/asaidimu/go-anansi/v6/example/api/types"
	"github.com/asaidimu/go-anansi/v6/example/api/webhooks"

	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Initialize persistence layer (e.g., in-memory for example)
	// In a real application, this would be configured based on environment
	persistence := base.NewInMemoryPersistence(logger)

	// Configure the API server
	config := &server.Config{
		RequestTimeout: 30 * time.Second,
		MaxRequestSize: 1024 * 1024, // 1MB
		CORSEnabled:    true,
		CORSOrigins:    []string{"*"}, // Allow all for example, restrict in production
	}

	// Create API server instance
	apiServer := server.New(persistence, logger, config)

	// Start the server
	addr := ":8080"
	logger.Info("Starting API server", zap.String("address", addr))
	if err := apiServer.Start(addr); err != nil && err != http.ErrServerClosed {
		logger.Fatal("API server failed to start", zap.Error(err))
	}

	// Graceful shutdown (example, typically handled by OS signals)
	// ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// defer cancel()
	// if err := apiServer.Close(ctx); err != nil {
	// 	logger.Error("API server graceful shutdown failed", zap.Error(err))
	// }
}