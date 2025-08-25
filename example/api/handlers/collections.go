package handlers

import (
	"fmt"
    "net/http"
    "strings"
    
    "github.com/asaidimu/go-anansi/v6/example/api/types"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
)

// CollectionsHandler handles collection CRUD operations
type CollectionsHandler struct {
    *BaseHandler
}

func NewCollectionsHandler(base *BaseHandler) *CollectionsHandler {
    return &CollectionsHandler{BaseHandler: base}
}

// Handle routes collection operations based on URL pattern
// /api/collections/{collection}/{operation}
func (h *CollectionsHandler) Handle(w http.ResponseWriter, r *http.Request) {
    // Parse URL path
    pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
    if len(pathParts) != 4 {
        h.WriteError(w, types.NewClientError(types.ErrCodeInvalidPath, "Invalid API path"))
        return
    }
    
    collection := pathParts[2]
    operation := pathParts[3]
    
    // Route to appropriate handler
    switch operation {
    case string(types.OpCreate):
        h.handleCreate(w, r, collection)
    case string(types.OpRead):
        h.handleRead(w, r, collection)
    case string(types.OpUpdate):
        h.handleUpdate(w, r, collection)
    case string(types.OpDelete):
        h.handleDelete(w, r, collection)
    case "validate":
        h.handleValidate(w, r, collection)
    case "capabilities":
        h.handleCapabilities(w, r, collection)
    case "metadata":
        h.handleMetadata(w, r, collection)
    default:
        h.WriteError(w, types.NewClientError(types.ErrCodeInvalidPath, "Invalid operation"))
    }
}

// Individual operation handlers with proper error handling and timeouts
func (h *CollectionsHandler) handleCreate(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionCreateRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	if len(req.Documents) == 1 {
		result, err := collection.CreateOne(ctx, req.Documents[0])
		if err != nil {
			h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to create document", err))
			return
		}
		h.WriteResponse(w, result)
	} else {
		results, err := collection.CreateMany(ctx, req.Documents)
		if err != nil {
			h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to create documents", err))
			return
		}
		h.WriteResponse(w, results)
	}
}

func (h *CollectionsHandler) handleRead(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionReadRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	queryDSL := req.Query
	if queryDSL == nil {
		queryDSL = &query.Query{}
	}

	result, err := collection.Read(ctx, queryDSL)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to read documents", err))
		return
	}

	h.WriteResponse(w, result)
}

func (h *CollectionsHandler) handleUpdate(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionUpdateRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	updateParams := &base.CollectionUpdate{
		Data:    req.Data,
		Filter:  req.Filter,
		Version: req.Version,
		Recover: req.Recover,
	}

	count, err := collection.Update(ctx, updateParams)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to update documents", err))
		return
	}

	h.WriteResponse(w, map[string]any{"updatedCount": count})
}

func (h *CollectionsHandler) handleDelete(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionDeleteRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	count, err := collection.Delete(ctx, req.Filter, req.Unsafe)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to delete documents", err))
		return
	}

	h.WriteResponse(w, map[string]any{"deletedCount": count})
}

func (h *CollectionsHandler) handleValidate(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionValidateRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	result, err := collection.Validate(ctx, req.Data, req.Loose)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeValidationFailed, "Failed to validate document", err))
		return
	}

	h.WriteResponse(w, result)
}

func (h *CollectionsHandler) handleCapabilities(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	capabilities := collection.Capabilities(ctx)
	h.WriteResponse(w, capabilities)
}

func (h *CollectionsHandler) handleMetadata(w http.ResponseWriter, r *http.Request, collectionName string) {
	if err := h.ValidateMethod(r, types.MethodPOST); err != nil {
		h.WriteError(w, err)
		return
	}

	ctx, cancel := h.WithTimeout(r.Context())
	defer cancel()

	var req types.CollectionMetadataRequest
	if err := h.ParseJSON(w, r, &req, 1024*1024); err != nil {
		h.WriteError(w, err)
		return
	}

	collection, err := h.persistence.Collection(ctx, collectionName)
	if err != nil {
		h.WriteError(w, types.NewClientError(types.ErrCodeCollectionNotFound, fmt.Sprintf("Collection '%s' not found", collectionName), err.Error()))
		return
	}

	metadata, err := collection.Metadata(ctx, req.Filter, req.ForceRefresh)
	if err != nil {
		h.WriteError(w, types.NewServerError(types.ErrCodeDatabaseError, "Failed to get collection metadata", err))
		return
	}

	h.WriteResponse(w, metadata)
}
