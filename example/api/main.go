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

	usersCollection, err := persistence.Collection(context.Background(), user.ModelName)
	// cast users to UserModel below
	users, ok := usersCollection.(*user.UserModel)
	if !ok {
		log.Fatal("That's fishy:", err)
	}

	_ = users.CreateAdminUser()

	// Create the API server
	server := NewAPIServer(persistence, logger)

	// Start the server
	if err := server.Start(":8081"); err != nil {
		logger.Fatal("Server failed to start", zap.Error(err))
	}
}
