package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/asaidimu/go-anansi/v6"
	"github.com/asaidimu/go-anansi/v6/core/common" // Import common for Issue
	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/persistence/utils"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/query/native"
	"github.com/asaidimu/go-anansi/v6/core/schema"
	coreutils "github.com/asaidimu/go-anansi/v6/core/utils" // For BoolPtr
	sqliteExecutor "github.com/asaidimu/go-anansi/v6/sqlite/executor"
	sqliteQuery "github.com/asaidimu/go-anansi/v6/sqlite/query"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"go.uber.org/zap"
)

// Schemas
func getUserSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "User",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Name: "name", Type: "string", Required: coreutils.BoolPtr(true)},
			"email": {Name: "email", Type: "string", Required: coreutils.BoolPtr(true), Unique: coreutils.BoolPtr(true)},
		},
	}
}

func getAccountSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "Account",
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"userId":  {Name: "userId", Type: "string", Required: coreutils.BoolPtr(true)}, // Foreign key to User
			"balance": {Name: "balance", Type: "number", Required: coreutils.BoolPtr(true)},
		},
	}
}

func getLedgerTransactionSchema() *schema.SchemaDefinition {
	return &schema.SchemaDefinition{
		Name:    "LedgerTransaction", // Renamed from Transaction
		Version: "1.0.0",
		Fields: map[string]*schema.FieldDefinition{
			"accountId": {Name: "accountId", Type: "string", Required: coreutils.BoolPtr(true)}, // Foreign key to Account
			"amount":    {Name: "amount", Type: "number", Required: coreutils.BoolPtr(true)},
			"type":      {Name: "type", Type: "string", Required: coreutils.BoolPtr(true)}, // e.g., "deposit", "withdrawal"
			"timestamp": {Name: "timestamp", Type: "integer", Required: coreutils.BoolPtr(true)},
		},
	}
}

// NegativeAmountValidator is a CollectionDecorator that prevents transactions with negative amounts.
func NegativeAmountValidator(logger *zap.Logger) utils.CollectionDecorator {
	return func(next base.Collection) base.Collection {
		return &negativeAmountValidator{
			Collection: next,
			logger: logger,
		}
	}
}

type negativeAmountValidator struct {
	base.Collection
	logger *zap.Logger
}

// Ensure negativeAmountValidator implements base.Collection
var _ base.Collection = (*negativeAmountValidator)(nil)

func (d *negativeAmountValidator) validateAmount(doc *data.Document) error {
	// 1. Check if the "amount" key even exists in this document/update.
	// If it's missing, we skip validation (typical for partial updates).
	if !doc.HasKey("amount") {
		return nil
	}

	// 2. Since the key exists, retrieve the value using coercion.
	// doc.GetInt is robust—it handles float64 (common from JSON) or int types.
	amount, err := doc.GetInt("amount")
	if err != nil {
		// If the key is there but isn't a valid number, that's a type error.
		return fmt.Errorf("invalid amount format for document: %w", err)
	}

	// 3. Perform the business logic check.
	if amount < 0 {
		d.logger.Warn("Attempted to create/update transaction with negative amount",
			zap.Int("amount", amount))
		return fmt.Errorf("transaction amount cannot be negative: %d", amount)
	}

	return nil
}

func (d *negativeAmountValidator) CreateOne(ctx context.Context, doc *data.Document) (base.CreateResult, error) {
	if err := d.validateAmount(doc); err != nil {
		return base.CreateResult{Status: base.StatusFailedValidation, Data: doc, Issues: []common.Issue{{Message: err.Error()}}}, err
	}
	return d.Collection.CreateOne(ctx, doc)
}

func (d *negativeAmountValidator) CreateMany(ctx context.Context, docs []*data.Document) ([]base.CreateResult, error) {
	for _, doc := range docs {
		if err := d.validateAmount(doc); err != nil {
			return nil, err // Or return a partial result with errors
		}
	}
	return d.Collection.CreateMany(ctx, docs)
}


func (d *negativeAmountValidator) Update(ctx context.Context, params *base.CollectionUpdate) (int, error) {
	if err := d.validateAmount(params.Set); err != nil {
		return 0, err
	}
	return d.Collection.Update(ctx, params)
}

func main() {
	// 1. Setup Logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Sync()

	// 2. Setup In-Memory SQLite Database
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
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
	// Add our custom NegativeAmountValidator to the collection decorators
	decorators := &utils.Decorators{
		CollectionDecorators: []utils.DecoratorFunc[base.Collection]{
			(utils.DecoratorFunc[base.Collection])(NegativeAmountValidator(logger)), // Explicit cast
		},
	}

	// 6. Initialize Anansi Persistence Layer
	cfg := anansi.SetupConfig{
		Interactor:            interactor,
		Logger:                logger,
		DocumentFactoryConfig: factoryConfig,
		Decorators:            decorators,
	}
	p, err := anansi.Setup(cfg)
	if err != nil {
		log.Fatalf("Failed to setup Anansi: %v", err)
	}
	logger.Info("Anansi persistence layer initialized successfully.")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 7. Create Collections
	userSchema := getUserSchema()
	usersCollection, err := p.CreateCollection(ctx, *userSchema)
	if err != nil {
		log.Fatalf("Failed to create users collection: %v", err)
	}
	logger.Info("Users collection created.")

	accountSchema := getAccountSchema()
	accountsCollection, err := p.CreateCollection(ctx, *accountSchema)
	if err != nil {
		log.Fatalf("Failed to create accounts collection: %v", err)
	}
	logger.Info("Accounts collection created.")

	ledgerTransactionSchema := getLedgerTransactionSchema()                          // Renamed
	transactionsCollection, err := p.CreateCollection(ctx, *ledgerTransactionSchema) // Renamed
	if err != nil {
		log.Fatalf("Failed to create transactions collection: %v", err)
	}
	logger.Info("Transactions collection created.")

	// 8. Populate Data
	logger.Info("Populating data...")
	user1 := data.MustNewDocument(map[string]any{"id": "U001", "name": "Alice", "email": "alice@example.com"})
	user2 := data.MustNewDocument(map[string]any{"id": "U002", "name": "Bob", "email": "bob@example.com"})
	_, err = usersCollection.CreateMany(ctx, []*data.Document{user1, user2})
	if err != nil {
		log.Fatalf("Failed to create users: %v", err)
	}
	logger.Info("Users created.")

	account1 := data.MustNewDocument(map[string]any{"id": "A001", "userId": "U001", "balance": 1000.00})
	account2 := data.MustNewDocument(map[string]any{"id": "A002", "userId": "U002", "balance": 500.00})
	_, err = accountsCollection.CreateMany(ctx, []*data.Document{account1, account2})
	if err != nil {
		log.Fatalf("Failed to create accounts: %v", err)
	}
	logger.Info("Accounts created.")

	// Create valid transactions
	tx1 := data.MustNewDocument(map[string]any{"id": "T001", "accountId": "A001", "amount": 200.00, "type": "deposit", "timestamp": time.Now().Unix()})
	tx2 := data.MustNewDocument(map[string]any{"id": "T002", "accountId": "A002", "amount": 50.00, "type": "withdrawal", "timestamp": time.Now().Unix()})
	_, err = transactionsCollection.CreateMany(ctx, []*data.Document{tx1, tx2})
	if err != nil {
		log.Fatalf("Failed to create valid transactions: %v", err)
	}
	logger.Info("Valid transactions created.")

	// Attempt to create an invalid transaction (negative amount)
	logger.Info("Attempting to create invalid transaction (negative amount)...")
	invalidTx := data.MustNewDocument(map[string]any{"id": "T003", "accountId": "A001", "amount": -10.00, "type": "withdrawal", "timestamp": time.Now().Unix()})
	_, err = transactionsCollection.CreateOne(ctx, invalidTx)
	if err != nil {
		logger.Info(fmt.Sprintf("Successfully prevented invalid transaction: %v", err))
	} else {
		log.Fatalf("ERROR: Invalid transaction (negative amount) was created!")
	}

	// 9. Complex Queries with Joins

	// Get all transactions for Alice (User U001)
	logger.Info("Querying all transactions for Alice (User U001)...")
	aliceTransactionsQuery := query.NewQueryBuilder().
		From("LedgerTransaction"). // Renamed
		LeftJoin("Account").On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "LedgerTransaction.accountId", // Renamed
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				FieldRefVal: &query.FieldReference{
					Type:  "field",
					Field: "Account.id",
				},
			},
		},
	}).End().
		LeftJoin("User").On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "Account.userId",
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				FieldRefVal: &query.FieldReference{
					Type:  "field",
					Field: "User.id",
				},
			},
		},
	}).End().
		Where("User.id").Eq("U001").
		Build()

	txResult, err := transactionsCollection.Read(ctx, &aliceTransactionsQuery)
	if err != nil {
		log.Fatalf("Failed to query transactions for Alice: %v", err)
	}

	logger.Info(fmt.Sprintf("Found %d transactions for Alice:", txResult.Count))
	var aliceDocs = txResult.Data

	for _, doc := range aliceDocs {
		// Access nested fields using schema names as keys
		ledgerTx := doc.Must().Get("LedgerTransaction").(map[string]any)
		user := doc.Must().Get("User").(map[string]any)
		logger.Info(fmt.Sprintf("  Transaction ID: %s, Amount: %.2f, Type: %s, Account ID: %s, User Name: %s",
			ledgerTx["id"], ledgerTx["amount"], ledgerTx["type"], ledgerTx["accountId"], user["name"]))
	}

	// Get user details for a specific account (A002)
	logger.Info("Querying user details for Account A002...")
	accountUserDetailsQuery := query.NewQueryBuilder().
		From("Account").
		LeftJoin("User").On(query.QueryFilter{
		Condition: &query.FilterCondition{
			Field:    "Account.userId",
			Operator: query.ComparisonOperatorEq,
			Value: query.FilterValue{
				FieldRefVal: &query.FieldReference{
					Type:  "field",
					Field: "User.id",
				},
			},
		},
	}).End().
		Where("Account.id").Eq("A002").
		Build()

	accountResult, err := accountsCollection.Read(ctx, &accountUserDetailsQuery)
	if err != nil {
		log.Fatalf("Failed to query user details for account A002: %v", err)
	}

	var accountDocs = accountResult.Data

	if len(accountDocs) > 0 {
		doc := accountDocs[0]
		name := doc.Must().GetString("User.name")
		email := doc.Must().GetString("User.email")
		balance := doc.Must().GetFloat64("Account.balance")
		logger.Info(fmt.Sprintf("Account A002 User: Name=%s, Email=%s, Balance=%.2f", name, email, balance))
	} else {
		logger.Info("No user found for Account A002.")
	}

	logger.Info("Advanced example finished.")
}
