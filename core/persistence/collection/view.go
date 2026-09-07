package collection

import (
        "fmt"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/query"
)

// composeViewQuery merges a stored view query with a user-supplied read query.
// The resulting query is what gets handed to the engine for execution against
// the view's underlying collection.
//
// Composition rules:
//
//  1. Target: the view's Target is used (NOT the user's). Views pin the
//     underlying collection; the user cannot redirect a view at a different
//     collection by passing a different Target.
//  2. Filters: the view's filters AND the user's filters are combined into a
//     single AND group. Either side may be nil.
//  3. Projection: the user's projection (if set) is intersected with the
//     view's projection (if set). If only one is set, that one is used. If
//     neither is set, no projection is applied (the engine returns all
//     fields of the underlying schema).
//  4. Sort: the view's sort is applied first; the user's sort is appended.
//     This means the view's primary sort order wins, with the user's sort
//     acting as a tiebreaker.
//  5. Limit: the user's limit (if set) takes precedence. If only the view
//     sets a limit, the view's limit is used. If both set a limit, the
//     smaller of the two is used.
//  6. Pagination, Joins, Distinct, Aggregations, Union, Hints, Raw: the
//     user's values are used as-is. Views cannot pre-define these — the
//     view's values for these fields are ignored.
func composeViewQuery(view, user *query.Query) (*query.Query, error) {
        if view == nil {
                return user, nil
        }
        if user == nil {
                // Should not happen — the caller always passes a non-nil user
                // query. But defend against it anyway by returning the view as-is.
                return view, nil
        }

        composed := *user // shallow copy; we'll override selected fields

        // 1. Target: the view wins.
        if view.Target != nil {
                composed.Target = view.Target
        }

        // 2. Filters: AND-merge.
        composed.Filters = andMergeFilters(view.Filters, user.Filters)

        // 3. Projection: intersect if both set; otherwise the set one wins.
        if view.Projection != nil && user.Projection != nil {
                intersected, err := intersectProjections(view.Projection, user.Projection)
                if err != nil {
                        return nil, err
                }
                composed.Projection = intersected
        } else if view.Projection != nil {
                composed.Projection = view.Projection
        }
        // else: user.Projection stays (may be nil)

        // 4. Sort: view's first, user's appended.
        if len(view.Sort) > 0 {
                composed.Sort = append(append([]query.SortConfiguration{}, view.Sort...), user.Sort...)
        }

        // 5. Limit: smaller of the two wins.
        if view.Limit != nil && user.Limit != nil {
                if *view.Limit < *user.Limit {
                        composed.Limit = view.Limit
                }
                // else: user.Limit (already in composed via shallow copy)
        } else if view.Limit != nil {
                composed.Limit = view.Limit
        }
        // else: user.Limit (already in composed)

        return &composed, nil
}

// andMergeFilters combines two QueryFilter trees under a single AND group.
// If either side is nil, the other is returned unchanged. If both are nil,
// nil is returned.
func andMergeFilters(a, b *query.QueryFilter) *query.QueryFilter {
        if a == nil {
                return b
        }
        if b == nil {
                return a
        }
        // If both are present, wrap them in an AND group. We use a shallow
        // copy of each side to avoid mutating the caller's filter trees.
        return &query.QueryFilter{
                Group: &query.FilterGroup{
                        Operator:   common.LogicalAnd,
                        Conditions: []query.QueryFilter{*a, *b},
                },
        }
}

// intersectProjections produces a projection that includes only the fields
// present in BOTH projections. If either projection uses Exclude semantics
// (rather than Include), the intersection falls back to the user's projection
// (since Exclude semantics don't compose well with Include semantics without
// a full set-algebra implementation).
func intersectProjections(view, user *query.ProjectionConfiguration) (*query.ProjectionConfiguration, error) {
        if view == nil {
                return user, nil
        }
        if user == nil {
                return view, nil
        }
        // If either side uses Exclude, the intersection is ambiguous — fall
        // back to the user's projection. The user's intent (which fields to
        // exclude) takes precedence over the view's.
        if len(user.Exclude) > 0 || len(view.Exclude) > 0 {
                return user, nil
        }
        // Both are Include-style (or empty). Compute the intersection.
        if len(user.Include) == 0 {
                // User wants all fields — the view's projection wins.
                return view, nil
        }
        if len(view.Include) == 0 {
                // View allows all fields — user's projection wins.
                return user, nil
        }
        // Both have explicit Include lists. Intersect them by field name.
        viewSet := make(map[string]struct{}, len(view.Include))
        for _, f := range view.Include {
                viewSet[f.Name] = struct{}{}
        }
        var intersection []query.ProjectionField
        for _, f := range user.Include {
                if _, ok := viewSet[f.Name]; ok {
                        intersection = append(intersection, f)
                }
        }
        if len(intersection) == 0 {
                // No overlap — the user asked for fields the view doesn't
                // expose. Return an empty projection rather than failing; the
                // engine will return no fields, which makes the mismatch
                // observable in the result shape.
                return &query.ProjectionConfiguration{Include: []query.ProjectionField{}}, nil
        }
        return &query.ProjectionConfiguration{Include: intersection}, nil
}

// _ = fmt.Sprintf // keep import in case of future error formatting
var _ = fmt.Sprintf
