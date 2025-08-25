package server

import (
    "context"
    "net/http"
    "time"
    "strings"
    
    "github.com/asaidimu/go-anansi/v6/example/api/handlers"
    "github.com/asaidimu/go-anansi/v6/example/api/middleware"
    "github.com/asaidimu/go-anansi/v6/example/api/webhooks"
    "github.com/asaidimu/go-anansi/v6/core/persistence/base"
    "go.uber.org/zap"
)

// APIServer represents the main API server
type APIServer struct {
    persistence     base.Persistence
    logger         *zap.Logger
    mux            *http.ServeMux
    webhookManager *webhooks.Manager
    
    // Handlers
    collections  *handlers.CollectionsHandler
    management   *handlers.ManagementHandler
    transactions *handlers.TransactionsHandler
    subscriptions *handlers.SubscriptionsHandler
    schema       *handlers.SchemaHandler
    metadata     *handlers.MetadataHandler
    
    // Configuration
    config *Config
}

// Config holds server configuration
type Config struct {
    RequestTimeout time.Duration
    MaxRequestSize int64
    CORSEnabled    bool
    CORSOrigins    []string
}

func New(persistence base.Persistence, logger *zap.Logger, config *Config) *APIServer {
	s := &APIServer{
		persistence:    persistence,
		logger:         logger,
		mux:            http.NewServeMux(),
		webhookManager: webhooks.NewManager(logger),
		config:         config,
	}

	s.collections = handlers.NewCollectionsHandler(handlers.NewBaseHandler(persistence, logger))
	s.management = handlers.NewManagementHandler(handlers.NewBaseHandler(persistence, logger))
	s.transactions = handlers.NewTransactionsHandler(handlers.NewBaseHandler(persistence, logger))
	// s.subscriptions = handlers.NewSubscriptionsHandler(handlers.NewBaseHandler(persistence, logger))
	// s.schema = handlers.NewSchemaHandler(handlers.NewBaseHandler(persistence, logger))
	// s.metadata = handlers.NewMetadataHandler(handlers.NewBaseHandler(persistence, logger))

	s.setupRoutes()
	return s
}

// setupMiddleware configures the middleware chain
func (s *APIServer) setupMiddleware() *middleware.Chain {
	return middleware.New(
		middleware.WithRecovery(s.logger),
		middleware.WithRequestLogging(s.logger),
		middleware.WithTimeout(s.config.RequestTimeout),
		middleware.WithValidation(),
		s.corsMiddleware(),
	)
}

// Start starts the server
func (s *APIServer) Start(addr string) error {
	s.logger.Info("Starting API server", zap.String("address", addr))
	handler := s.setupMiddleware().Then(s.mux) // Apply middleware to the mux
	return http.ListenAndServe(addr, handler)
}

// Close gracefully shuts down the server
func (s *APIServer) Close(ctx context.Context) error {
	s.logger.Info("Shutting down API server")
	// Close the persistence layer
	if err := s.persistence.Close(ctx); err != nil {
		s.logger.Error("Failed to close persistence layer", zap.Error(err))
		return err
	}
	s.logger.Info("API server shutdown complete")
	return nil
}

// ServeHTTP implements http.Handler interface for APIServer
func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// corsMiddleware is a placeholder for CORS handling
func (s *APIServer) corsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.config.CORSEnabled {
				w.Header().Set("Access-Control-Allow-Origin", strings.Join(s.config.CORSOrigins, ","))
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusOK)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
