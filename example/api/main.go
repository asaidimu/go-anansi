package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asaidimu/go-anansi/v6/example/api/internal/api"
	"github.com/asaidimu/go-anansi/v6/example/api/internal/app"
	"github.com/asaidimu/go-anansi/v6/example/api/internal/response"
	"github.com/asaidimu/go-anansi/v6/example/api/schema"
	"go.uber.org/zap"
)

func main() {
	// 1. Load Configuration
	cfg := app.NewConfig()

	// 2. Setup Logger
	logger := app.NewLogger()
	defer logger.Sync()

	// 3. Setup Response Handler
	rh := response.NewHandler()

	// 4. Setup Database
	db, err := app.NewDatabase(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to setup database", zap.Error(err))
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Failed to close database", zap.Error(err))
		}
	}()

	// 5. Load Schemas
	schemaLoader, err := schema.NewSchemaLoader()
	if err != nil {
		logger.Fatal("Failed to load schemas", zap.Error(err))
	}

	// 6. Setup Persistence Manager
	pm, err := app.NewPersistenceManager(db, schemaLoader, cfg, logger)
	if err != nil {
		logger.Fatal("Failed to setup persistence manager", zap.Error(err))
	}

	// 7. Setup API Server
	server := api.NewAPIServer(cfg, logger, pm, rh)
	server.SetupRoutes()

	// 8. Start HTTP Server in a goroutine
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	logger.Info("Application started successfully.")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down application...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}
	logger.Info("Application shutdown complete.")
}
