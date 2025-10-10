package app

import (
	"os"
)

// Config holds application-wide configuration settings.
type Config struct {
	Port   string
	DBPath string
}

// NewConfig loads configuration from environment variables or provides defaults.
func NewConfig() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/anansi.db" // Default to a persistent SQLite file
	}

	return &Config{
		Port:   port,
		DBPath: dbPath,
	}
}
