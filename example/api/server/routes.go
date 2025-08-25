package server

// setupRoutes configures all API routes
func (s *APIServer) setupRoutes() {
    // Collection data operations - now using proper HTTP methods
    s.mux.HandleFunc("/api/collections/", s.collections.Handle)
    
    // Collection management operations
    s.mux.HandleFunc("/api/management/collections/list", s.management.ListCollections)
    s.mux.HandleFunc("/api/management/collections/create", s.management.CreateCollection)
    s.mux.HandleFunc("/api/management/collections/schema", s.management.GetSchema)
    s.mux.HandleFunc("/api/management/collections/delete", s.management.DeleteCollection)
    
    // Transaction operations
    s.mux.HandleFunc("/api/transactions/execute", s.transactions.Execute)
    
    // Subscription operations
    // s.mux.HandleFunc("/api/subscriptions/", s.subscriptions.Handle)
    
    // Schema operations
    // s.mux.HandleFunc("/api/schema/", s.schema.Handle)
    
    // Metadata operations
    // s.mux.HandleFunc("/api/metadata", s.metadata.Handle)
}
