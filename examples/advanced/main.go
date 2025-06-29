package main

import (
	"database/sql"
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/asaidimu/go-anansi/v2/core/persistence"
	"github.com/asaidimu/go-anansi/v2/sqlite"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

func main() {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)       // Set minimum logging level to Info
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // Human-readable timestamps
	config.EncoderConfig.TimeKey = "timestamp"                   // Key for the timestamp field

	logger, err := config.Build()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// 1. Open SQLite database connection
	db, err := sql.Open("sqlite3", "./database.db")
	if err != nil {
		logger.Fatal("Failed to open database", zap.Error(err))
	}
	defer db.Close()

	// 2. Initialize SQLite Interactor
	// Setting DropIfExists to true for easy testing. In a real application, manage migrations carefully.
	interactorOptions := sqlite.DefaultInteractorOptions()
	interactor := sqlite.NewSQLiteInteractor(db, logger, interactorOptions, nil)

	// Create persistence layer
	persistenceLayer, err := persistence.NewPersistence(interactor, nil)
	if err != nil {
		log.Fatal("Failed to initialize persistence:", err)
	}

	// Create API server
	apiServer := NewAPIServer(persistenceLayer, logger)

	// Start server
	log.Println("Starting API server on :8080")
	if err := apiServer.Start(":8080"); err != nil {
		log.Fatal("Server failed:", err)
	}
}
