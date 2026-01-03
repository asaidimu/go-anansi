package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

//go:embed schemas/*.json
var schemasFS embed.FS

const (
	userCtxKey = "userID" // Key for user ID in context
)

// SecurityDecorator enforces document-level security based on ownerId.
func SecurityDecorator(logger *zap.Logger) utils.CollectionDecorator {
	return func(next base.Collection) base.Collection {
		return &securityDecorator{
			Collection: next,
			logger:     logger,
		}
	}
}

type securityDecorator struct {
	base.Collection
	logger *zap.Logger
}

var _ base.Collection = (*securityDecorator)(nil)

// getUserIDFromContext extracts the user ID from the context.
func (d *securityDecorator) getUserIDFromContext(ctx context.Context) (string, error) {
	userID, ok := ctx.Value(userCtxKey).(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("unauthorized: user ID not found in context")
	}
	return userID, nil
}

// checkAccess checks if the user has access to the document.
func (d *securityDecorator) checkAccess(ctx context.Context, doc *data.Document) error {
	ownerID, err := d.getOwnerId(doc)
	if err != nil {
		return err
	}

	userID, err := d.getUserIDFromContext(ctx)
	if err != nil {
		return err // Unauthorized
	}

	if userID != ownerID {
		return fmt.Errorf("forbidden: user %s does not own this document", userID)
	}
	return nil
}

func (d *securityDecorator) getOwnerId(doc *data.Document) (string, error) {
	ownerID, err := doc.GetString("ownerId")

	if err != nil {
		return "", err
	}
	if ownerID == "" {
		return "", fmt.Errorf("unauthorized: user ID not found in doc")
	}
	return ownerID, nil
}

func (d *securityDecorator) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	// For creation, ensure the ownerId matches the user in context if provided
	if ownerID, err := d.getOwnerId(doc); err == nil {
		userID, err := d.getUserIDFromContext(ctx)
		if err != nil {
			return base.CreateResult{}, err
		}
		if userID != ownerID {
			return base.CreateResult{}, fmt.Errorf("forbidden: cannot create document with ownerId %s as user %s", ownerID, userID)
		}
	}
	return d.Collection.CreateOne(ctx, doc)
}

func (d *securityDecorator) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	for _, doc := range docs {
		if ownerID, err := d.getOwnerId(doc); err == nil {
			userID, err := d.getUserIDFromContext(ctx)
			if err != nil {
				return nil, err
			}
			if userID != ownerID {
				return nil, fmt.Errorf("forbidden: cannot create document with ownerId %s as user %s in batch", ownerID, userID)
			}
		}
	}
	return d.Collection.CreateMany(ctx, docs)
}

func (d *securityDecorator) Read(ctx context.Context, q *query.Query) (*base.ReadResult, error) {
	// For read, we need to modify the query to include ownerId filter
	userID, err := d.getUserIDFromContext(ctx)
	if err != nil {
		return nil, err // Unauthorized
	}

	// Add a filter to ensure only documents owned by the user are returned
	ownerFilter := query.NewQueryBuilder().Where("ownerId").Eq(userID).Build().Filters

	// Combine with existing filters
	if q.Filters == nil {
		q.Filters = ownerFilter
	} else {
		q.Filters = query.NewQueryBuilder().AndFilter(*q.Filters).AndFilter(*ownerFilter).Build().Filters
	}

	return d.Collection.Read(ctx, q)
}

func (d *securityDecorator) Update(ctx context.Context, update *base.CollectionUpdate) (int, error) {
	// For update, ensure the user owns the document being updated
	userID, err := d.getUserIDFromContext(ctx)
	if err != nil {
		return 0, err // Unauthorized
	}

	// Add a filter to ensure only documents owned by the user are updated
	ownerFilter := query.NewQueryBuilder().Where("ownerId").Eq(userID).Build().Filters

	// Combine with existing filters
	if update.Filter == nil {
		update.Filter = ownerFilter
	} else {
		update.Filter = query.NewQueryBuilder().AndFilter(*update.Filter).AndFilter(*ownerFilter).Build().Filters
	}

	return d.Collection.Update(ctx, update)
}

func (d *securityDecorator) Delete(ctx context.Context, qf *query.QueryFilter, unsafe bool) (int, error) {
	// For delete, ensure the user owns the document being deleted
	userID, err := d.getUserIDFromContext(ctx)
	if err != nil {
		return 0, err // Unauthorized
	}

	// Add a filter to ensure only documents owned by the user are deleted
	ownerFilter := query.NewQueryBuilder().Where("ownerId").Eq(userID).Build().Filters

	// Combine with existing filters
	if qf == nil {
		qf = ownerFilter
	} else {
		qf = query.NewQueryBuilder().AndFilter(*qf).AndFilter(*ownerFilter).Build().Filters
	}

	return d.Collection.Delete(ctx, qf, unsafe)
}

// authenticateMiddleware is a simple middleware to simulate authentication
func authenticateMiddleware(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In a real app, this would validate a token/session
		// For this example, we'll use a hardcoded user ID from a header
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			logger.Warn("Unauthorized access attempt: X-User-ID header missing")
			http.Error(w, "Unauthorized: X-User-ID header required", http.StatusUnauthorized)
			return
		}

		// Add user ID to context
		ctx := context.WithValue(r.Context(), userCtxKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeJSONResponse writes a JSON response to the http.ResponseWriter
func writeJSONResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error writing JSON response: %v", err)
	}
}

func main() {
	// 1. Setup Logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 2. Setup In-Memory SQLite Database
	db, err := sql.Open("sqlite3", "database.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 3. Create Database Interactor for SQLite
	executor, err := sqliteExecutor.NewSQLiteExecutor(db, logger)
	if err != nil {
		log.Fatalf("Failed to create SQLite interactor: %v", err)
	}
	queryFactory := sqliteQuery.NewSQLiteFactory()
	interactor, err := native.NewNativeInteractor(executor, queryFactory, logger)
	if err != nil {
		log.Fatalf("Failed to create native interactor: %v", err)
	}

	// 4. Setup Document Factory Config
	factoryConfig := data.DocumentFactoryConfig{}

	// 5. Setup Decorators
	decorators := &utils.Decorators{
		CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
			(utils.DecoratorFunc[base.Collection])(SecurityDecorator(logger)),
		},
	}

	// 6. Load Schemas from embedded JSON files
	var userSchemaDef schema.SchemaDefinition
	userSchemaBytes, err := fs.ReadFile(schemasFS, "schemas/user.json")
	if err != nil {
		log.Fatalf("Failed to read user.json: %v", err)
	}
	if err := userSchemaDef.From(userSchemaBytes); err != nil {
		log.Fatalf("Failed to unmarshal user.json: %v", err)
	}

	var documentSchemaDef schema.SchemaDefinition
	documentSchemaBytes, err := fs.ReadFile(schemasFS, "schemas/document.json")
	if err != nil {
		log.Fatalf("Failed to read document.json: %v", err)
	}
	if err := documentSchemaDef.From(documentSchemaBytes); err != nil {
		log.Fatalf("Failed to unmarshal document.json: %v", err)
	}

	// 7. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:            interactor,
		Logger:                logger,
		DocumentFactoryConfig: factoryConfig,
		Decorators:            decorators,
		Schemas: []schema.SchemaDefinition{
			userSchemaDef,
			documentSchemaDef,
		},
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Use a context for initial setup
	defer cancel()

	// 8. Create Collections
	usersCollection, err := p.Collection(ctx, userSchemaDef.Name)
	if err != nil {
		log.Fatalf("Failed to create users collection: %v", err)
	}
	logger.Info("Users collection created.")

	documentsCollection, err := p.Collection(ctx, documentSchemaDef.Name)
	if err != nil {
		log.Fatalf("Failed to create documents collection: %v", err)
	}
	logger.Info("Documents collection created.")

	/* // 9. Populate some initial data (e.g., an admin user)
	adminUser := data.MustNewDocument(map[string]any{
		"id":           "admin123",
		"username":     "admin",
		"passwordHash": "hashed_admin_password", // In real app, hash this
		"role":         "admin",
	})

	_, err = usersCollection.CreateOne(ctx, adminUser)
	if err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}
	logger.Info("Admin user created.") */

	// 10. Set up HTTP Handlers
	mux := http.NewServeMux()

	// User API
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var newUser *data.Document
			if err := json.NewDecoder(r.Body).Decode(&newUser); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// In a real app, hash password before storing
			result, err := usersCollection.CreateOne(r.Context(), newUser)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSONResponse(w, http.StatusCreated, result.Data)
		case http.MethodGet:
			q := query.NewQueryBuilder().Build()
			result, err := usersCollection.Read(r.Context(), &q)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSONResponse(w, http.StatusOK, result.Data)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/documents", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var newDoc *data.Document
			if err := json.NewDecoder(r.Body).Decode(&newDoc); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			// ownerId should be set by the security decorator or validated
			// For this example, we'll assume it's passed in the request body
			// and the decorator will enforce it matches the X-User-ID
			result, err := documentsCollection.CreateOne(r.Context(), newDoc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSONResponse(w, http.StatusCreated, result.Data)
		case http.MethodGet:
			q := query.NewQueryBuilder().Build()
			result, err := documentsCollection.Read(r.Context(), &q)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSONResponse(w, http.StatusOK, result.Data)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Apply authentication middleware to all API routes
	authenticatedMux := authenticateMiddleware(mux, logger)

	// 11. Start HTTP Server
	port := ":8080"
	logger.Info(fmt.Sprintf("Server starting on port %s", port))
	log.Fatal(http.ListenAndServe(port, authenticatedMux))
}
