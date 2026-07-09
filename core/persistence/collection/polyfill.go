package collection

import (
	"context"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/data"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/persistence/transaction"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"go.uber.org/zap"
)

// polyfillCollection is a decorator that adds polyfills for database features
// that might not be supported by the underlying interactor, such as returning
// documents on update.
type polyfillCollection struct {
	base.Collection
	interactor query.DatabaseInteractor
	logger     *zap.Logger
}

var _ base.Collection = (*polyfillCollection)(nil)

// newPolyfillCollection creates a new polyfill decorator.
func newPolyfillCollection(
	wrapped base.Collection,
	interactor query.DatabaseInteractor,
	logger *zap.Logger,
) base.Collection {
	return &polyfillCollection{
		Collection: wrapped,
		interactor: interactor,
		logger:     logger,
	}
}

// getCurrentInteractor returns the interactor from the context if a transaction
// is in progress, otherwise it returns the base interactor for the collection.
func (p *polyfillCollection) getCurrentInteractor(ctx context.Context) query.DatabaseInteractor {
	if result, ok := query.GetInteractor(ctx); ok {
		return result
	}
	return p.interactor
}

// findCandidateIDs recursively traverses a query filter to find a bounding
// set of IDs. It returns a slice of ID values and a boolean indicating if a
// valid bounding set was found. This allows skipping the initial 'Read' query
// in the polyfill.
func findCandidateIDs(filter *query.QueryFilter) ([]query.FilterValue, bool) {
	if filter == nil {
		return nil, false
	}

	// Base Case: A single condition
	if filter.Condition != nil {
		cond := filter.Condition
		if cond.Field == data.DocumentIDField {
			if cond.Operator == query.ComparisonOperatorEq && cond.Value.StringVal != nil {
				return []query.FilterValue{cond.Value}, true
			}
			if cond.Operator == query.ComparisonOperatorIn && len(cond.Value.ArrayVal) > 0 {
				return cond.Value.ArrayVal, true
			}
		}
		return nil, false
	}

	// Recursive Step: A group of conditions
	if filter.Group != nil {
		group := filter.Group

		// For an AND group, we find the intersection of all ID sets.
		if group.Operator == common.LogicalAnd {
			var intersectionSet map[string]query.FilterValue
			isInitialized := false

			for _, subFilter := range group.Conditions {
				subIDs, ok := findCandidateIDs(&subFilter)
				if !ok {
					continue // This branch isn't an ID filter, so we ignore it.
				}

				// Convert the slice of IDs to a map for efficient lookup.
				currentSet := make(map[string]query.FilterValue)
				for _, id := range subIDs {
					if id.StringVal != nil {
						currentSet[*id.StringVal] = id
					}
				}

				if !isInitialized {
					intersectionSet = currentSet
					isInitialized = true
				} else {
					// Compute the intersection with the main set.
					nextIntersection := make(map[string]query.FilterValue)
					for k, v := range currentSet {
						if _, exists := intersectionSet[k]; exists {
							nextIntersection[k] = v
						}
					}
					intersectionSet = nextIntersection
				}
			}

			if isInitialized {
				// Convert map back to slice
				finalIDs := make([]query.FilterValue, 0, len(intersectionSet))
				for _, v := range intersectionSet {
					finalIDs = append(finalIDs, v)
				}
				return finalIDs, true
			}
		}

		// For an OR group, we can only optimize if ALL branches are ID-based.
		if group.Operator == common.LogicalOr {
			finalIDs := make([]query.FilterValue, 0)
			for _, subFilter := range group.Conditions {
				subIDs, ok := findCandidateIDs(&subFilter)
				if !ok {
					// If any branch is not a valid ID filter, we can't optimize the OR group.
					return nil, false
				}
				finalIDs = append(finalIDs, subIDs...)
			}
			// Succeeded, so all branches were ID-based.
			return finalIDs, true
		}
	}

	return nil, false
}

// Update modifies documents in the collection that match the filter. If the
// underlying driver does not support returning updated documents, this method
// polyfills the behavior by performing a multi-step fetch-update-fetch process
// within a single transaction.
func (p *polyfillCollection) Update(ctx context.Context, params *base.CollectionUpdate) (*base.ReadResult, error) {
	if params == nil || params.Filter == nil {
		return nil, base.ErrInvalidUpdateParams
	}

	// Determine if the polyfill is needed: documents are requested, but the driver can't return them on update.
	needsPolyfill := params.ReturnDocument && !p.Capabilities(ctx).ReturnOnUpdate
	if !needsPolyfill {
		return p.Collection.Update(ctx, params)
	}

	// --- Polyfill execution ---
	// The operation is wrapped in a transaction to ensure atomicity for the multi-step process.
	// We use `getCurrentInteractor` to correctly join an existing transaction if one is present.
	result, err := transaction.Execute(ctx, p.getCurrentInteractor(ctx), p.logger, func(transactionCtx context.Context, _ query.DatabaseInteractor) (any, error) {
		// Use the new `transactionCtx` for all subsequent operations to ensure they run within the same transaction.

		var affectedIDs []query.FilterValue

		// Attempt to extract IDs directly from the filter to optimize away the initial Read.
		knownIDs, canSkipRead := findCandidateIDs(params.Filter)
		if canSkipRead {
			affectedIDs = knownIDs
		} else {
			// 1. Fallback: Fetch IDs of documents that will be updated.
			idQuery := query.NewQueryBuilder().
				Select().
				Include(data.DocumentIDField).
				End().
				AndFilter(*params.Filter).
				Build()

			idResult, err := p.Collection.Read(transactionCtx, &idQuery)
			if err != nil {
				return nil, common.SystemErrorFrom(err, "ERR_PERSISTENCE_POLYFILL_FETCH_IDS_FAILED")
			}
			if idResult.Count == 0 {
				return &base.ReadResult{Count: 0, Data: []*data.Document{}}, nil
			}

			affectedIDs = data.MapDocumentSet(idResult.Data, func(d *data.Document) query.FilterValue {
				idStr := d.ID()
				idCopy := idStr // Create a copy
				return query.FilterValue{StringVal: &idCopy}
			})

		}

		if len(affectedIDs) == 0 {
			return &base.ReadResult{Count: 0, Data: []*data.Document{}}, nil
		}

		// 2. Perform the actual update. We create a new params object with ReturnDocument set to false
		// because we are handling the document retrieval manually.
		updateOnlyParams := &base.CollectionUpdate{
			Filter:         params.Filter,
			Set:            params.Set,
			Compute:        params.Compute,
			Version:        params.Version,
			ReturnDocument: false, // Polyfill is handling the return.
		}

		updateResult, err := p.Collection.Update(transactionCtx, updateOnlyParams)
		if err != nil {
			return nil, err
		}

		if updateResult.Total == nil || *updateResult.Total == 0 {
			return &base.ReadResult{Count: 0, Data: []*data.Document{}}, nil
		}

		// 3. Fetch the full, updated documents using the IDs.
		fetchQuery := query.NewQueryBuilder().
			AndFilter(query.QueryFilter{
				Condition: &query.FilterCondition{
					Field:    data.DocumentIDField,
					Operator: query.ComparisonOperatorIn,
					Value:    query.FilterValue{ArrayVal: affectedIDs},
				},
			}).
			Build()

		fetchResult, err := p.Collection.Read(transactionCtx, &fetchQuery)
		if err != nil {
			// The update succeeded, but the final fetch failed. Return an empty document list with the correct count.
			return &base.ReadResult{Count: 0, Total: updateResult.Total, Data: []*data.Document{}}, nil
		}

		// The final result uses the documents from the final fetch and the count from the update.
		return &base.ReadResult{Count: len(fetchResult.Data), Total: updateResult.Total, Data: fetchResult.Data}, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*base.ReadResult), nil
}
