package main

import (
	"context"
	"log"

	"github.com/asaidimu/go-anansi/v6/example/api/model/user"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("Failed to create logger:", err)
	}
	defer logger.Sync()

	// Initialize your persistence layer (you'll need to implement this)
	persistence, cleanup := setup()
	defer cleanup()

	// 9. Populate some initial data (e.g., an admin user)
	users, err := user.NewUserModel(context.Background(), persistence)
	if err != nil {
		log.Fatal("Failed to create users model:", err)
	}

	_ = users.CreateAdminUser()

	// Create the API server
	server := NewAPIServer(persistence, logger)

	// Start the server
	if err := server.Start(":8081"); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
