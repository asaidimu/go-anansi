# Revised Proposal: Pluggable Full-Text Search Functionality

This document proposes the integration of a pluggable, high-performance, full-text search capability into the `go-anansi` framework. The proposal leverages a coordinated dual-cursor architecture to provide both storage efficiency and performance, addressing the fundamental tension between consistency and speed in search implementations.

## 1. Current Architecture Analysis

Before proposing new components, it is essential to have a precise understanding of the existing architecture, particularly the query execution workflow.

### Core Components & Philosophy

The framework is built on a clean separation of data representation, data structure definition, query language, and persistence orchestration.

- **`core/schema`**: Defines the "blueprint" of the data, including structure, types, and validation rules.
- **`core/data`**: Represents the data records themselves. The `data.Document` type is the primary rich object used for all data manipulation.
- **`core/query`**: Defines the generic "language" for requesting data. It provides a rich Domain-Specific Language (DSL) via structs (`dsl.go`), a fluent `QueryBuilder` for constructing queries, and a `QueryEngine` for processing them.
- **`core/persistence`**: The high-level, user-facing API that manages collections and transactions.
- **`DatabaseInteractor`**: An interface within `core/query` that defines the contract for a primary, structured data store backend. This is the "doer" for structured data.

### The `Read` Workflow and the `QueryEngine`

The current query execution process is a sophisticated, multi-stage pipeline that is agnostic of any specific database language like SQL.

1. When `collection.Read(ctx, query)` is called, the `Query` object is passed to the `QueryEngine`.
2. The engine uses a **`QueryPartitioner`**. This component inspects the `Query` and compares its requirements against the declared `Capabilities` of the currently configured `DatabaseInteractor`.
3. The partitioner splits the `Query` into two distinct parts:
   - A **Database Query**: A `Query` object containing all the operations the `DatabaseInteractor` can execute natively (e.g., simple field equality, indexed sorting).
   - A **Residual Query**: The remaining operations that must be performed in-memory by Go code after the initial data retrieval (e.g., filtering based on a custom Go function, complex computed fields).
4. The `QueryEngine` first sends the **Database Query** to the `DatabaseInteractor` (e.g., the SQLite implementation). The interactor is responsible for translating this generic `Query` object into its specific native language (like SQL) and returning an initial, partially-filtered set of `data.Document`s.
5. The `QueryEngine` then takes this result set and uses a `QueryHelper` to perform the **Residual Query** operations on it (in-memory filtering, sorting, etc.).
6. The final, fully processed result set is then returned to the caller.

This two-phase process of **database-first filtering followed by in-memory refinement** is a critical architectural pattern. The proposed search functionality will extend this pattern by introducing a coordinated approach between search indices and database cursors.

## 2. Proposed Search Functionality

We propose a comprehensive search solution that addresses two distinct use cases through a unified architecture: consistent, filter-based searches and high-performance, real-time streaming searches. This will be achieved by introducing a coordinated dual-cursor system that maintains storage efficiency while providing excellent performance.

### Pathway 1: Consistent Search-as-a-Filter (via `Read()`)

This pathway addresses use cases where search is used as a powerful filter, but the primary requirement is to receive fully consistent `data.Document` objects from the main database.

- **User Experience**: The user will construct a query using the existing `QueryBuilder`, embedding a `TextSearchQuery` within the filter clause. They will pass this `Query` object to the existing `collection.Read()` method.
- **Implementation**: The `QueryEngine` will be enhanced to detect a `TextSearchQuery` in the filter tree.
  1. It will extract the `TextSearchQuery` and pass it to the `IndexManager.FindIDs()` method.
  2. This method will return a slice of document IDs that match the search criteria, ordered by relevance.
  3. Instead of rewriting the query with potentially large IN clauses, the `QueryEngine` will use a new **prefiltered query** mechanism. The `DatabaseInteractor` will be enhanced to accept a `PrefilterSet` parameter containing the search result IDs.
  4. The database backend can then optimize this operation using database-specific techniques (temporary tables, CTEs, or optimized IN clauses with batching).
  5. The remaining non-search filters in the original query will be applied normally by the database.
- **Outcome**: The user receives a standard `QueryResult` containing fully consistent `data.Document` objects, with search results properly integrated into the existing query pipeline.

### Pathway 2: Real-Time Streaming Search (via `Search()`)

This pathway is designed for performance-critical, interactive applications (e.g., live search UIs) where speed is paramount and eventual consistency is acceptable. It uses a coordinated dual-cursor architecture for optimal performance and storage efficiency.

- **User Experience**: A new `Search()` method will be available on collections. This method will establish a two-way stream for interactive, incremental searches.
- **Implementation**:
  1. **API**: A new `Searchable` interface will provide the `Search(ctx context.Context) (*SearchSession, error)` method. Collections will be wrapped with a decorator that implements this interface.
  2. **Coordinated Cursors**: The system maintains two coordinated cursors:
     - An **Index Cursor** that traverses search results in relevance order, returning document IDs with scores
     - A **Database Cursor** that efficiently fetches full documents by ID from the primary database
  3. **Streaming Coordination**: A `CursorCoordinator` manages the interaction between cursors, fetching results in optimized batches and streaming complete `SearchResultItem` objects to the client.
  4. **Session Management**: Each search session runs in its own goroutine with proper lifecycle management, context cancellation, and resource cleanup.
- **Outcome**: The user gets a highly responsive, streaming search experience with full document consistency and efficient resource utilization.

### Architectural Components

The new functionality will be implemented through a set of clean interfaces and structs, primarily defined in an enhanced `core/query/search.go`.

#### 1. Core Search Abstractions (`core/query/search.go`)

```go
package query

import (
    "context"
    "github.com/asaidimu/go-anansi/v6/core/data"
)

// SearchSession holds the channels for a two-way interactive search stream.
// Sessions are automatically managed with proper cleanup and context cancellation.
type SearchSession struct {
    QueryInput    chan<- *TextSearchQuery
    ResultsOutput <-chan StreamResult
}

// Close gracefully terminates the search session and cleans up resources.
func (s *SearchSession) Close() error

// SearchResultItem is a single search hit, containing the full document
// retrieved from the database, along with search-specific metadata.
type SearchResultItem struct {
    Document   data.Document
    Score      float64
    Highlights map[string][]string
}

// StreamResult is the object sent over the results channel for each found item.
type StreamResult struct {
    Item SearchResultItem
    Err  error
}

// ScoredID represents a document ID with its search relevance score.
type ScoredID struct {
    ID    string
    Score float64
}

// Searchable is the interface for collections that support interactive searching.
type Searchable interface {
    Search(ctx context.Context) (*SearchSession, error)
}

// IndexCursor provides ordered traversal of search results.
type IndexCursor interface {
    // NextBatch returns the next batch of document IDs with scores, ordered by relevance
    NextBatch(ctx context.Context, batchSize int) ([]ScoredID, error)
    
    // Reset repositions the cursor for a new query
    Reset(query *TextSearchQuery) error
    
    // Close releases cursor resources
    Close() error
}

// DatabaseCursor provides efficient document retrieval by ID.
type DatabaseCursor interface {
    // FetchByIDs retrieves documents by IDs, maintaining the provided order
    FetchByIDs(ctx context.Context, ids []string) ([]data.Document, error)
    
    // Close releases cursor resources
    Close() error
}

// PrefilterSet contains document IDs to be used as a prefilter in database queries.
type PrefilterSet struct {
    IDs    []string
    Limit  int  // Optional limit for the prefilter
}

// SearchConfig provides configuration options for search functionality.
type SearchConfig struct {
    BatchSize        int           // Number of results to process per batch
    SessionTimeout   time.Duration // Maximum session lifetime
    BufferSize       int           // Channel buffer size for results
    RetryPolicy      *RetryPolicy  // Retry configuration for failed operations
    IndexedFields    []string      // Subset of fields to index (optional)
    SyncMode         SyncMode      // Synchronous, Asynchronous, or Batch indexing
}

// RetryPolicy defines retry behavior for failed operations.
type RetryPolicy struct {
    MaxRetries      int
    InitialInterval time.Duration
    MaxInterval     time.Duration
    Multiplier      float64
}

// SyncMode defines how index updates are synchronized.
type SyncMode int

const (
    SyncModeSync SyncMode = iota  // Synchronous updates (slower, more consistent)
    SyncModeAsync                 // Asynchronous updates (faster, eventually consistent)
    SyncModeBatch                 // Batch updates (balanced approach)
)

// IndexSyncResult provides detailed feedback on indexing operations.
type IndexSyncResult struct {
    Success   bool
    Error     error
    Retryable bool
    DocumentID string
}

// SearchError provides detailed error information for search operations.
type SearchError struct {
    Type      SearchErrorType
    Message   string
    Retryable bool
    Cause     error
}

type SearchErrorType int

const (
    SearchErrorIndexUnavailable SearchErrorType = iota
    SearchErrorQueryInvalid
    SearchErrorTimeout
    SearchErrorResourceExhausted
)

// IndexManager is the backend interface that search engine implementations must satisfy.
type IndexManager interface {
    // Index adds or updates documents in the search index with detailed result feedback
    Index(collectionName string, documents ...data.Document) []IndexSyncResult
    
    // Delete removes documents from the search index
    Delete(collectionName string, docIDs ...string) error
    
    // OpenSearchCursor creates a new cursor for iterating search results
    OpenSearchCursor(ctx context.Context, collectionName string, query *TextSearchQuery) (IndexCursor, error)
    
    // FindIDs is used by the QueryEngine for consistent reads (Pathway 1)
    FindIDs(ctx context.Context, collectionName string, textSearch *TextSearchQuery) ([]string, error)
    
    // GetMetrics returns search engine performance metrics
    GetMetrics() SearchMetrics
    
    // Close gracefully shuts down the search manager
    Close() error
}

// SearchMetrics provides observability into search performance.
type SearchMetrics struct {
    TotalQueries        int64
    AverageQueryTime    time.Duration
    IndexSize          int64
    SyncSuccessRate    float64
    ActiveSessions     int32
}
```

#### 2. Enhanced Database Integration

The `DatabaseInteractor` interface will be extended to support prefiltered queries and cursor operations:

```go
// Enhanced DatabaseInteractor interface
type DatabaseInteractor interface {
    // Existing methods...
    Read(ctx context.Context, query Query) (*QueryResult, error)
    
    // New methods for search integration
    ReadWithPrefilter(ctx context.Context, query Query, prefilter *PrefilterSet) (*QueryResult, error)
    OpenCursor(ctx context.Context, collectionName string) (DatabaseCursor, error)
}
```

#### 3. Cursor Coordination Logic

```go
// CursorCoordinator manages the interaction between search and database cursors.
type CursorCoordinator struct {
    config       SearchConfig
    resultBuffer chan StreamResult
}

// NewCursorCoordinator creates a new coordinator with the specified configuration.
func NewCursorCoordinator(config SearchConfig) *CursorCoordinator {
    return &CursorCoordinator{
        config:       config,
        resultBuffer: make(chan StreamResult, config.BufferSize),
    }
}

// StreamResults coordinates between cursors to stream search results.
func (c *CursorCoordinator) StreamResults(ctx context.Context, 
    indexCursor IndexCursor, 
    dbCursor DatabaseCursor, 
    output chan<- StreamResult) {
    
    defer close(output)
    
    for {
        select {
        case <-ctx.Done():
            return
        default:
            // Get next batch of scored IDs from search index
            scoredIDs, err := indexCursor.NextBatch(ctx, c.config.BatchSize)
            if err != nil {
                if err != io.EOF {
                    output <- StreamResult{Err: NewSearchError(SearchErrorIndexUnavailable, err.Error(), true, err)}
                }
                return
            }
            
            if len(scoredIDs) == 0 {
                return // End of results
            }
            
            // Extract IDs for database lookup
            ids := make([]string, len(scoredIDs))
            for i, scored := range scoredIDs {
                ids[i] = scored.ID
            }
            
            // Fetch full documents from database
            documents, err := dbCursor.FetchByIDs(ctx, ids)
            if err != nil {
                output <- StreamResult{Err: NewSearchError(SearchErrorResourceExhausted, err.Error(), true, err)}
                continue
            }
            
            // Create a map for efficient document lookup
            docMap := make(map[string]data.Document)
            for _, doc := range documents {
                if id, exists := doc.Get("id"); exists {
                    if idStr, ok := id.(string); ok {
                        docMap[idStr] = doc
                    }
                }
            }
            
            // Stream results in search order, handling missing documents gracefully
            for _, scoredID := range scoredIDs {
                if doc, exists := docMap[scoredID.ID]; exists {
                    result := SearchResultItem{
                        Document: doc,
                        Score:    scoredID.Score,
                        // Highlights can be populated by the search engine
                    }
                    
                    select {
                    case output <- StreamResult{Item: result}:
                    case <-ctx.Done():
                        return
                    }
                } else {
                    // Document exists in index but not in database
                    // This could indicate a sync issue - log for monitoring
                    // but don't break the search flow
                    continue
                }
            }
        }
    }
}
```

#### 4. Enhanced Query Engine Integration

The `QueryEngine` will be updated to handle search queries intelligently:

```go
// Enhanced QueryPartitioner to handle TextSearchQuery
func (qp *QueryPartitioner) PartitionWithSearch(query Query, capabilities Capabilities, indexManager IndexManager) (*PartitionedQuery, error) {
    // Extract TextSearchQuery from filter tree
    textSearch, remainingQuery := qp.extractTextSearch(query)
    
    if textSearch != nil && indexManager != nil {
        // Get matching document IDs from search index
        matchingIDs, err := indexManager.FindIDs(context.Background(), query.Collection, textSearch)
        if err != nil {
            return nil, fmt.Errorf("search query failed: %w", err)
        }
        
        // Create prefilter set
        prefilter := &PrefilterSet{
            IDs:   matchingIDs,
            Limit: remainingQuery.Limit,
        }
        
        // Partition the remaining query normally
        partitioned, err := qp.partition(remainingQuery, capabilities)
        if err != nil {
            return nil, err
        }
        
        // Add prefilter to the database query
        partitioned.DatabaseQuery.Prefilter = prefilter
        
        return partitioned, nil
    }
    
    // No search query - proceed with normal partitioning
    return qp.partition(query, capabilities)
}
```

#### 5. Event-Driven Indexing with Enhanced Error Handling

```go
// SearchIndexSynchronizer listens for document events and updates the search index.
type SearchIndexSynchronizer struct {
    indexManager IndexManager
    eventBus     EventBus
    config       SearchConfig
    metrics      *SyncMetrics
}

// NewSearchIndexSynchronizer creates a new synchronizer with the specified configuration.
func NewSearchIndexSynchronizer(indexManager IndexManager, eventBus EventBus, config SearchConfig) *SearchIndexSynchronizer {
    return &SearchIndexSynchronizer{
        indexManager: indexManager,
        eventBus:     eventBus,
        config:       config,
        metrics:      NewSyncMetrics(),
    }
}

// Start begins listening for document events.
func (s *SearchIndexSynchronizer) Start(ctx context.Context) error {
    // Subscribe to document lifecycle events
    s.eventBus.Subscribe("DocumentCreateSuccess", s.handleDocumentCreate)
    s.eventBus.Subscribe("DocumentUpdateSuccess", s.handleDocumentUpdate)
    s.eventBus.Subscribe("DocumentDeleteSuccess", s.handleDocumentDelete)
    
    return nil
}

func (s *SearchIndexSynchronizer) handleDocumentCreate(event Event) {
    s.handleDocumentIndex(event)
}

func (s *SearchIndexSynchronizer) handleDocumentUpdate(event Event) {
    s.handleDocumentIndex(event)
}

func (s *SearchIndexSynchronizer) handleDocumentIndex(event Event) {
    // Extract document and collection info from event
    doc, collection := s.extractEventData(event)
    
    // Index the document with retry logic
    results := s.indexManager.Index(collection, doc)
    
    for _, result := range results {
        s.metrics.RecordSync(result.Success)
        
        if !result.Success && result.Retryable {
            // Implement exponential backoff retry
            go s.retryIndex(collection, doc, result.DocumentID)
        }
    }
}

func (s *SearchIndexSynchronizer) handleDocumentDelete(event Event) {
    docID, collection := s.extractDeleteEventData(event)
    
    err := s.indexManager.Delete(collection, docID)
    if err != nil {
        // Log error but don't break the flow
        // Consider implementing retry for delete operations as well
    }
}

func (s *SearchIndexSynchronizer) retryIndex(collection string, doc data.Document, docID string) {
    policy := s.config.RetryPolicy
    interval := policy.InitialInterval
    
    for attempt := 0; attempt < policy.MaxRetries; attempt++ {
        time.Sleep(interval)
        
        results := s.indexManager.Index(collection, doc)
        if len(results) > 0 && results[0].Success {
            s.metrics.RecordRetrySuccess()
            return
        }
        
        interval = time.Duration(float64(interval) * policy.Multiplier)
        if interval > policy.MaxInterval {
            interval = policy.MaxInterval
        }
    }
    
    s.metrics.RecordRetryFailure()
}
```

#### 6. Pluggable Backend (`bleve/` package)

A new top-level `bleve/` package will provide the first concrete implementation:

```go
package bleve

import (
    "context"
    "github.com/blevesearch/bleve/v2"
    "github.com/asaidimu/go-anansi/v6/core/query"
    "github.com/asaidimu/go-anansi/v6/core/data"
)

// BleveIndexManager implements query.IndexManager using the Bleve search engine.
type BleveIndexManager struct {
    indices map[string]bleve.Index
    config  BleveConfig
    metrics *BleveMetrics
}

// BleveConfig provides Bleve-specific configuration options.
type BleveConfig struct {
    IndexDir        string
    DefaultAnalyzer string
    BatchSize       int
}

// NewBleveIndexManager creates a new Bleve-based index manager.
func NewBleveIndexManager(config BleveConfig) (*BleveIndexManager, error) {
    return &BleveIndexManager{
        indices: make(map[string]bleve.Index),
        config:  config,
        metrics: NewBleveMetrics(),
    }, nil
}

// Implementation of query.IndexManager interface methods...
func (bim *BleveIndexManager) Index(collectionName string, documents ...data.Document) []query.IndexSyncResult {
    // Bleve-specific implementation
}

func (bim *BleveIndexManager) OpenSearchCursor(ctx context.Context, collectionName string, searchQuery *query.TextSearchQuery) (query.IndexCursor, error) {
    // Return a BleveIndexCursor implementation
}

// BleveIndexCursor implements query.IndexCursor for Bleve indices.
type BleveIndexCursor struct {
    index    bleve.Index
    query    *bleve.SearchRequest
    iterator *BleveResultIterator
}

// Implementation of query.IndexCursor interface methods...
```

#### 7. Configuration Integration (`anansi.go`)

```go
// Enhanced Setup function with search configuration
func Setup(config Config, searchConfig *SearchConfig, indexManager query.IndexManager) (*Anansi, error) {
    anansi := &Anansi{
        config: config,
        // ... existing initialization
    }
    
    // Initialize search functionality if provided
    if searchConfig != nil && indexManager != nil {
        // Set up event-driven indexing
        synchronizer := NewSearchIndexSynchronizer(indexManager, anansi.eventBus, *searchConfig)
        err := synchronizer.Start(context.Background())
        if err != nil {
            return nil, fmt.Errorf("failed to start search synchronization: %w", err)
        }
        
        // Enhance query engine with search capabilities
        anansi.queryEngine.SetIndexManager(indexManager)
        
        // Wrap collections with search functionality
        anansi.collections = wrapCollectionsWithSearch(anansi.collections, indexManager, *searchConfig)
    }
    
    return anansi, nil
}

// wrapCollectionsWithSearch applies the Searchable decorator to collections
func wrapCollectionsWithSearch(collections map[string]Collection, indexManager query.IndexManager, config SearchConfig) map[string]Collection {
    wrapped := make(map[string]Collection)
    
    for name, collection := range collections {
        wrapped[name] = &SearchableCollection{
            Collection:   collection,
            indexManager: indexManager,
            config:       config,
        }
    }
    
    return wrapped
}
```

## 3. Advanced Features and Considerations

### Schema Evolution and Migration

The system includes support for evolving search schemas:

```go
// IndexSchema defines the structure of searchable fields
type IndexSchema struct {
    Version int
    Fields  map[string]FieldConfig
}

type FieldConfig struct {
    Indexed      bool
    Stored       bool
    Highlighted  bool
    Analyzer     string
}

// Migration support for index schema changes
type SchemaMigration interface {
    Migrate(ctx context.Context, oldVersion, newVersion int) error
}
```

### Multi-tenancy Support

The architecture supports multi-tenant scenarios through collection-scoped indices:

```go
// Collections naturally provide tenant isolation
// Each collection maintains its own search index
// Access control is handled at the collection level
```

### Performance Monitoring and Observability

Built-in metrics and monitoring capabilities:

```go
// SearchMetrics provides comprehensive performance insights
type SearchMetrics struct {
    TotalQueries        int64         // Total number of search queries executed
    AverageQueryTime    time.Duration // Average query execution time
    P95QueryTime        time.Duration // 95th percentile query time
    IndexSize           int64         // Total size of search indices
    SyncSuccessRate     float64       // Percentage of successful sync operations
    ActiveSessions      int32         // Number of active search sessions
    CacheHitRate        float64       // Search result cache hit rate
    IndexingLatency     time.Duration // Average time to index a document
}
```

### Testing Strategy

The architecture includes comprehensive testing support:

```go
// MockIndexManager for unit testing
type MockIndexManager struct {
    indexedDocs map[string][]data.Document
    searchResults map[string][]ScoredID
}

// TestSearchSession for integration testing
type TestSearchSession struct {
    *SearchSession
    capturedQueries []TextSearchQuery
    injectedResults []SearchResultItem
}
```

## 4. Benefits and Trade-offs

### Benefits

**Storage Efficiency**: The dual-cursor approach eliminates document duplication between the database and search index, significantly reducing storage requirements.

**Consistency**: Documents are always retrieved from the primary database, ensuring full consistency with the authoritative data store.

**Performance**: Coordinated cursors with batch processing provide excellent performance for both small and large result sets.

**Scalability**: Cursor-based streaming handles arbitrarily large result sets without memory pressure.

**Pluggability**: The `IndexManager` interface enables easy integration of different search engines (Elasticsearch, Solr, etc.).

**Observability**: Built-in metrics and error reporting provide operational visibility.

**Flexibility**: Two-pathway design serves both consistency-focused and performance-focused use cases.

### Trade-offs

**Complexity**: The coordinated cursor system adds architectural complexity compared to simpler approaches.

**Latency**: Database lookups for each result batch add some latency compared to storing full documents in the search index.

**Resource Usage**: Maintains two open cursors per search session, increasing connection usage.

**Eventual Consistency**: Asynchronous indexing modes may result in temporary inconsistencies between search results and database state.

## 5. Migration and Adoption Path

### Backwards Compatibility

The search functionality is completely opt-in and maintains full backwards compatibility:

- Existing applications continue to work without modification
- Search features are only activated when an `IndexManager` is provided during setup
- All existing query operations remain unchanged

### Gradual Adoption

Organizations can adopt search functionality incrementally:

1. **Phase 1**: Enable search indexing without using search queries (background indexing)
2. **Phase 2**: Introduce search-as-filter functionality in non-critical paths
3. **Phase 3**: Implement real-time search for user-facing features
4. **Phase 4**: Optimize and tune search performance based on usage patterns

### Migration Tools

Helper utilities for migrating existing data to search indices:

```go
// IndexBuilder provides utilities for initial index population
type IndexBuilder interface {
    BuildFromCollection(ctx context.Context, collectionName string) error
    BuildFromQuery(ctx context.Context, query Query) error
    GetProgress() BuildProgress
}

type BuildProgress struct {
    TotalDocuments    int
    IndexedDocuments  int
    EstimatedTimeLeft time.Duration
    Errors           []error
}
```

## 6. Conclusion

This revised proposal presents a comprehensive, production-ready search solution for the `go-anansi` framework. The coordinated dual-cursor architecture elegantly balances storage efficiency, performance, and consistency while maintaining the framework's architectural principles.

Key innovations include:

- **Dual-pathway design** serving both consistency and performance use cases
- **Coordinated cursor system** eliminating storage duplication while maintaining performance
- **Enhanced error handling and retry logic** ensuring operational robustness
- **Comprehensive observability** providing insights into search performance and health
- **Pluggable architecture** enabling integration with multiple search engines

The solution respects the existing framework architecture while adding sophisticated search capabilities that can scale from simple applications to high-performance, production systems. The incremental adoption path ensures that organizations can implement search functionality at their own pace while maintaining full backwards compatibility.

This architecture provides a solid foundation for full-text search that can evolve with the framework's needs while maintaining operational excellence and developer experience.
