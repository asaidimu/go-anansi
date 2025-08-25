package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/example/api/types"
)

// TransactionsHandler handles transaction operations
type TransactionsHandler struct {
	*BaseHandler
}

func NewTransactionsHandler(base *BaseHandler) *TransactionsHandler {
	return &TransactionsHandler{BaseHandler: base}
}

// Execute handles transaction execution
func (h *TransactionsHandler) Execute(w http.ResponseWriter, r *http.Request) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.TransactionExecuteRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	// Validate operations before executing transaction
	if err := h.validateTransactionOperations(req.Operations); err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeValidationFailed, "Transaction operations validation failed", err.Error()))
		return
	}

	// Execute transaction using the persistence layer
	result, err := h.persistence.Transact(ctx, func(tx base.BasePersistence) (any, error) {
		results := make([]types.TransactionResult, 0, len(req.Operations))

		for i, op := range req.Operations {
			opResult, err := h.executeOperation(ctx, tx, op)
			opResult.OperationIndex = i
			opResult.Type = op.Type
			if err != nil {
				opResult.Error = err.Error()
				opResult.Success = false
				results = append(results, opResult)
				return nil, fmt.Errorf("operation %d failed: %s", i, opResult.Error)
			}
			opResult.Success = true
			results = append(results, opResult)
		}

		return map[string]any{
			"executed":         true,
			"operations_count": len(req.Operations),
			"results":          results,
		}, nil
	})

	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeTransactionFailed, "Transaction execution failed", err))
		return
	}

	h.WriteResponse(w, result)
}

// executeOperation executes a single transaction operation
func (h *TransactionsHandler) executeOperation(ctx context.Context, tx base.BasePersistence, op types.TransactionOperation) (types.TransactionResult, error) {
	collection, err := tx.Collection(ctx, op.Collection)
	if err != nil {
		return types.TransactionResult{}, fmt.Errorf("collection '%s' not found: %v", op.Collection, err)
	}

	switch strings.ToLower(op.Type) {
	case "create":
		if op.Data == nil {
			return types.TransactionResult{}, fmt.Errorf("data is required for create operation")
		}
		doc := data.MustNewDocument(op.Data)
		createResult, err := collection.CreateOne(ctx, doc)
		if err != nil {
			return types.TransactionResult{}, err
		}
		return types.TransactionResult{Data: createResult}, nil

	case "read":
		queryDSL := op.Query
		if queryDSL == nil {
			queryDSL = &query.Query{}
		}
		readResult, err := collection.Read(ctx, queryDSL)
		if err != nil {
			return types.TransactionResult{}, err
		}
		return types.TransactionResult{Data: readResult}, nil

	case "update":
		if op.Data == nil || op.Filter == nil {
			return types.TransactionResult{}, fmt.Errorf("data and filter are required for update operation")
		}
		doc := data.MustNewDocument(op.Data)
		updateParams := &base.CollectionUpdate{
			Data:   doc,
			Filter: op.Filter,
		}
		if op.Options != nil {
			if version, ok := op.Options["version"].(float64); ok {
				v := int(version)
				updateParams.Version = &v
			}
			if recover, ok := op.Options["recover"].(bool); ok {
				updateParams.Recover = recover
			}
		}
		count, err := collection.Update(ctx, updateParams)
		if err != nil {
			return types.TransactionResult{}, err
		}
		return types.TransactionResult{Data: map[string]any{"updatedCount": count}}, nil

	case "delete":
		if op.Filter == nil {
			return types.TransactionResult{}, fmt.Errorf("filter is required for delete operation")
		}
		unsafe := false
		if op.Options != nil {
			if u, ok := op.Options["unsafe"].(bool); ok {
				unsafe = u
			}
		}
		count, err := collection.Delete(ctx, op.Filter, unsafe)
		if err != nil {
			return types.TransactionResult{}, err
		}
		return types.TransactionResult{Data: map[string]any{"deletedCount": count}}, nil

	default:
		return types.TransactionResult{}, fmt.Errorf("unsupported operation type: %s", op.Type)
	}
}

// validateTransactionOperations validates the transaction operations
func (h *TransactionsHandler) validateTransactionOperations(operations []types.TransactionOperation) error {
	for i, op := range operations {
		if op.Type == "" {
			return fmt.Errorf("operation %d: type is required", i)
		}
		if op.Collection == "" {
			return fmt.Errorf("operation %d: collection is required", i)
		}

		switch strings.ToLower(op.Type) {
		case "create":
			if op.Data == nil {
				return fmt.Errorf("operation %d: data is required for create operation", i)
			}
		case "read":
			// Query is optional for read operations
		case "update":
			if op.Data == nil {
				return fmt.Errorf("operation %d: data is required for update operation", i)
			}
			if op.Filter == nil {
				return fmt.Errorf("operation %d: filter is required for update operation", i)
			}
		case "delete":
			if op.Filter == nil {
				return fmt.Errorf("operation %d: filter is required for delete operation", i)
			}
		default:
			return fmt.Errorf("operation %d: unsupported operation type '%s'", i, op.Type)
		}
	}
	return nil
}
