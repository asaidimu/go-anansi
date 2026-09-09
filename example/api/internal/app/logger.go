package app

import (
	"log"

	"go.uber.org/zap"
)

// NewLogger initializes and returns a new Zap logger.
func NewLogger() *zap.Logger {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}
