package query

import (
        "encoding/json"
        "fmt"
        "sort"
        "strings"

        "github.com/asaidimu/go-anansi/v8/core/common"
        "github.com/asaidimu/go-anansi/v8/core/query"
        "github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

// SQLiteSelectProjection handles SELECT clause projection
type SQLiteSelectProjection struct {
        factory      *sqliteFactory
        projection   *query.ProjectionConfiguration
        aggregations []query.AggregationConfiguration
        distinct     *query.QueryDistinctConfig
        schemas      map[string]*definition.Schema
        total        bool
}

func (p *SQLiteSelectProjection) Value() (string, []any, error) {
        var parts []string
        var params []any

        // Handle DISTINCT
        distinctClause := ""
        if p.distinct != nil {
                if p.distinct.IsDistinct != nil && *p.distinct.IsDistinct {
                        distinctClause = "DISTINCT "
                } else if len(p.distinct.Fields) > 0 {
                        distinctClause = "DISTINCT "
                }
        }

        // 1. Optional Total Count Injection
        // We only add the window function if this is the root query (depth 0).
        // This ensures total_count is available for pagination at the top level
        // but omitted in subqueries to maintain performance and compatibility.
        includeTotal := p.factory.depth == 0 && p.total
        if includeTotal {
                parts = append(parts, fmt.Sprintf("COUNT(*) OVER() AS %s", query.MatchCountName))
        }

        // Handle aggregations
        if len(p.aggregations) > 0 {
                for _, agg := range p.aggregations {
                        if agg.Type == "" { // This is a grouping/having configuration, not a projection
                                continue
                        }
                        var aggPart string
                        switch agg.Type {
                        case query.AggregationTypeCount:
                                if agg.Field == "*" {
                                        aggPart = "COUNT(*)"
                                } else {
                                        resolvedField, err := p.factory.resolveFieldReference(agg.Field, p.schemas)
                                        if err != nil {
                                                return "", nil, err
                                        }
                                        aggPart = fmt.Sprintf("COUNT(%s)", resolvedField)
                                }
                        case query.AggregationTypeSum:
                                resolvedField, err := p.factory.resolveFieldReference(agg.Field, p.schemas)
                                if err != nil {
                                        return "", nil, err
                                }
                                aggPart = fmt.Sprintf("SUM(%s)", resolvedField)
                        case query.AggregationTypeAvg:
                                resolvedField, err := p.factory.resolveFieldReference(agg.Field, p.schemas)
                                if err != nil {
                                        return "", nil, err
                                }
                                aggPart = fmt.Sprintf("AVG(%s)", resolvedField)
                        case query.AggregationTypeMin:
                                resolvedField, err := p.factory.resolveFieldReference(agg.Field, p.schemas)
                                if err != nil {
                                        return "", nil, err
                                }
                                aggPart = fmt.Sprintf("MIN(%s)", resolvedField)
                        case query.AggregationTypeMax:
                                resolvedField, err := p.factory.resolveFieldReference(agg.Field, p.schemas)
                                if err != nil {
                                        return "", nil, err
                                }
                                aggPart = fmt.Sprintf("MAX(%s)", resolvedField)
                        default:
                                return "", nil, ErrSelectUnsupportedAggregationType.WithCause(fmt.Errorf("unsupported aggregation type: %s", agg.Type))
                        }

                        if agg.Alias != nil {
                                aggPart = fmt.Sprintf("%s AS %s", aggPart, *agg.Alias)
                        }
                        parts = append(parts, aggPart)
                }
        }

        // Handle regular projection
        if p.projection != nil {
                // Handle included fields
                if len(p.projection.Include) > 0 {
                        for _, field := range p.projection.Include {
                                resolvedField, err := p.factory.resolveFieldReference(field.Name, p.schemas)
                                if err != nil {
                                        return "", nil, err
                                }
                                fieldPart := resolvedField
                                if field.Alias != nil {
                                        fieldPart = fmt.Sprintf("%s AS %s", resolvedField, *field.Alias)
                                }
                                parts = append(parts, fieldPart)
                        }
                }

                // Handle computed fields
                for _, computed := range p.projection.Computed {
                        if computed.ComputedFieldExpression != nil {
                                expr := computed.ComputedFieldExpression
                                funcSQL, funcParams, err := p.buildFunctionCall(expr.Expression)
                                if err != nil {
                                        return "", nil, err
                                }
                                params = append(params, funcParams...)
                                computedPart := fmt.Sprintf("%s AS %s", funcSQL, expr.Alias)
                                parts = append(parts, computedPart)
                        }

                        if computed.CaseExpression != nil {
                                caseExpr := computed.CaseExpression
                                caseSQL, caseParams, err := p.buildCaseExpression(caseExpr)
                                if err != nil {
                                        return "", nil, err
                                }
                                params = append(params, caseParams...)
                                casePart := fmt.Sprintf("%s AS %s", caseSQL, caseExpr.Alias)
                                parts = append(parts, casePart)
                        }
                }
        }

        // 2. Default Fallback Logic
        // We calculate the threshold based on whether total_count was injected.
        // If total_count was added, len(parts) is at least 1 even if no other fields were requested.
        threshold := 0
        if includeTotal {
                threshold = 1
        }

        if len(parts) == threshold {
                if len(p.schemas) > 0 {
                        var aliasedFields []string

                        // Create a slice to hold the map keys (aliases)
                        aliases := make([]string, 0, len(p.schemas))
                        for alias := range p.schemas {
                                aliases = append(aliases, alias)
                        }

                        sort.Strings(aliases)

                        for _, alias := range aliases {
                                schemaDef := p.schemas[alias]
                                if schemaDef != nil && len(schemaDef.Fields) > 0 {
                for _, name := range schemaDef.FieldNames() {
                                _, field := schemaDef.FindField(name)
                                resolvedField := fmt.Sprintf("%s.%s", quoteIdentifier(alias), quoteIdentifier(string(field.Name)))
                                fieldAlias := fmt.Sprintf("'%s.%s'", alias, field.Name)
                                aliasedFields = append(aliasedFields, fmt.Sprintf("%s AS %s", resolvedField, fieldAlias))
                                        }
                                }
                        }
                        if len(aliasedFields) > 0 {
                                parts = append(parts, aliasedFields...)
                        } else {
                                parts = append(parts, "*")
                        }
                } else {
                        parts = append(parts, "*")
                }
        }

        sql := fmt.Sprintf("SELECT %s%s", distinctClause, strings.Join(parts, ", "))
        return sql, params, nil
}

var expressionOperators = map[string]string{
        "ADD":      "+",
        "SUBTRACT": "-",
        "MULTIPLY": "*",
        "DIVIDE":   "/",
}

func (p *SQLiteSelectProjection) buildFunctionCall(fc *query.FunctionCall) (string, []any, error) {
        // Handle expression operators
        if sqlOp, ok := expressionOperators[fc.Function]; ok {
                if len(fc.Arguments) != 2 {
                        return "", nil, ErrSelectBinaryOperatorArgs.WithCause(fmt.Errorf("binary operator %s expects 2 arguments, got %d", fc.Function, len(fc.Arguments)))
                }

                arg1SQL, arg1Params, err := p.buildProjectionValue(&fc.Arguments[0])
                if err != nil {
                        return "", nil, err
                }

                arg2SQL, arg2Params, err := p.buildProjectionValue(&fc.Arguments[1])
                if err != nil {
                        return "", nil, err
                }

                sql := fmt.Sprintf("(%s %s %s)", arg1SQL, sqlOp, arg2SQL)
                params := append(arg1Params, arg2Params...)
        return sql, params, nil
}

        // Default to standard function call
        var args []string
        var params []any

        for _, arg := range fc.Arguments {
                argSQL, argParams, err := p.buildProjectionValue(&arg)
                if err != nil {
                        return "", nil, err
                }
                args = append(args, argSQL)
                params = append(params, argParams...)
        }

        sql := fmt.Sprintf("%s(%s)", fc.Function, strings.Join(args, ", "))
        return sql, params, nil
}

func (p *SQLiteSelectProjection) buildCaseExpression(ce *query.CaseExpression) (string, []any, error) {
        var parts []string
        var params []any

        parts = append(parts, "CASE")

        for _, condition := range ce.Conditions {
                whenSQL, whenParams, err := p.buildQueryFilter(&condition.When)
                if err != nil {
                        return "", nil, err
                }
                thenSQL, thenParams, err := p.buildProjectionValue(&condition.Then)
                if err != nil {
                        return "", nil, err
                }

                parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", whenSQL, thenSQL))
                params = append(params, whenParams...)
                params = append(params, thenParams...)
        }

        elseSQL, elseParams, err := p.buildProjectionValue(&ce.Else)
        if err != nil {
                return "", nil, err
        }
        parts = append(parts, fmt.Sprintf("ELSE %s", elseSQL))
        params = append(params, elseParams...)

        parts = append(parts, "END")
        return strings.Join(parts, " "), params, nil
}

func (p *SQLiteSelectProjection) buildProjectionValue(value *query.FilterValue) (string, []any, error) {
        if value.FieldRefVal != nil {
                resolvedField, err := p.factory.resolveFieldReference(value.FieldRefVal.Field, p.schemas)
                if err != nil {
                        return "", nil, err
                }
                return resolvedField, nil, nil
        }

        if value.StringVal != nil || value.NumberVal != nil || value.BoolVal != nil {
                param := p.factory.nextParam()
                var v any
                if value.StringVal != nil {
                        v = *value.StringVal
                } else if value.NumberVal != nil {
                        v = *value.NumberVal
                } else {
                        v = *value.BoolVal
                }
                return param, []any{v}, nil
        }

        if value.FunctionCallVal != nil {
                return p.buildFunctionCall(value.FunctionCallVal)
        }

        return p.buildFilterValue(value)
}

func (p *SQLiteSelectProjection) buildQueryFilter(filter *query.QueryFilter) (string, []any, error) {
        if filter.Condition != nil {
                return p.buildFilterCondition(filter.Condition)
        }
        if filter.Group != nil {
                return p.buildFilterGroup(filter.Group)
        }
        if filter.TextSearchQuery != nil {
                return p.buildTextSearch(filter.TextSearchQuery)
        }
        return "", nil, ErrSelectEmptyFilter
}

func (p *SQLiteSelectProjection) buildFilterCondition(condition *query.FilterCondition) (string, []any, error) {
        isNull := condition.Value.StringVal == nil &&
                condition.Value.NumberVal == nil &&
                condition.Value.BoolVal == nil &&
                condition.Value.ObjectVal == nil &&
                condition.Value.ArrayVal == nil &&
                condition.Value.FieldRefVal == nil &&
                condition.Value.FunctionCallVal == nil &&
                condition.Value.SubqueryVal == nil

        if isNull {
                var resolvedField string
                resolvedField, err := p.factory.resolveFieldReference(condition.Field, p.schemas)
                if err != nil {
                        return "", nil, err
                }
                switch condition.Operator {
                case query.ComparisonOperatorEq:
                        return fmt.Sprintf("%s IS NULL", resolvedField), nil, nil
                case query.ComparisonOperatorNeq:
                        return fmt.Sprintf("%s IS NOT NULL", resolvedField), nil, nil
                }
        }

        valueSQL, params, err := p.buildFilterValue(&condition.Value)
        if err != nil {
                return "", nil, err
        }

        var operator string
        switch condition.Operator {
        case query.ComparisonOperatorEq:
                operator = "="
        case query.ComparisonOperatorNeq:
                operator = "!="
        case query.ComparisonOperatorLt:
                operator = "<"
        case query.ComparisonOperatorLte:
                operator = "<="
        case query.ComparisonOperatorGt:
                operator = ">"
        case query.ComparisonOperatorGte:
                operator = ">="
        case query.ComparisonOperatorIn:
                operator = "IN"
        case query.ComparisonOperatorNin:
                operator = "NOT IN"
        case query.ComparisonOperatorContains:
                operator = "LIKE"
                valueSQL = "'%' || " + valueSQL + " || '%'"
        case query.ComparisonOperatorNotContains:
                operator = "NOT LIKE"
                valueSQL = "'%' || " + valueSQL + " || '%'"
        case query.ComparisonOperatorExists:
                resolvedField, err := p.factory.resolveFieldReference(condition.Field, p.schemas)
                if err != nil {
                        return "", nil, err
                }
                return fmt.Sprintf("%s IS NOT NULL", resolvedField), nil, nil
        case query.ComparisonOperatorNotExists:
                resolvedField, err := p.factory.resolveFieldReference(condition.Field, p.schemas)
                if err != nil {
                        return "", nil, err
                }
                return fmt.Sprintf("%s IS NULL", resolvedField), nil, nil
        default:
                return "", nil, ErrSelectUnsupportedOperator.WithCause(fmt.Errorf("unsupported operator: %s", condition.Operator))
        }

        resolvedField, err := p.factory.resolveFieldReference(condition.Field, p.schemas)
        if err != nil {
                return "", nil, err
        }
        sql := fmt.Sprintf("%s %s %s", resolvedField, operator, valueSQL)
        return sql, params, nil
}

func (p *SQLiteSelectProjection) buildFilterGroup(group *query.FilterGroup) (string, []any, error) {
        var conditions []string
        var params []any

        for _, condition := range group.Conditions {
                condSQL, condParams, err := p.buildQueryFilter(&condition)
                if err != nil {
                        return "", nil, err
                }
                conditions = append(conditions, condSQL)
                params = append(params, condParams...)
        }

        var operator string
        switch group.Operator {
        case common.LogicalAnd:
                operator = "AND"
        case common.LogicalOr:
                operator = "OR"
        default:
                return "", nil, ErrSelectUnsupportedLogicalOperator.WithCause(fmt.Errorf("unsupported logical operator: %s", group.Operator))
        }

        sql := fmt.Sprintf("(%s)", strings.Join(conditions, fmt.Sprintf(" %s ", operator)))
        return sql, params, nil
}

func (p *SQLiteSelectProjection) buildTextSearch(search *query.TextSearchQuery) (string, []any, error) {
        collectionQuoted, ftsTable, err := resolveFTSScope(p.factory)
        if err != nil {
                return "", nil, err
        }

        // Validate that the requested search fields actually exist as columns
        // on the FTS5 virtual table. The FTS table mirrors the string-typed
        // fields declared in the collection's fulltext index — searching on a
        // field the FTS table doesn't have would be a runtime error.
        //
        // We don't have the original schema at this layer (the factory only has
        // the schemas map), but the resolveFieldReference call below already
        // validates the field name against the schema, which is sufficient for
        // the FTS5 case since the FTS table uses the same field names as the
        // underlying collection.
        if err := validateFTSSearchFields(p.factory, p.schemas, search); err != nil {
                return "", nil, err
        }

        // Build the FTS5 MATCH query string. FTS5 syntax:
        //   - "phrase query"           → exact phrase match
        //   - term1 term2              → implicit AND (both must match)
        //   - term1 OR term2           → either matches
        //   - prefix*                  → prefix match (contains)
        // The MATCH operator is applied to the whole FTS table; we then
        // filter the result by rowid to map back to the collection rows.
        param := p.factory.nextParam()
        matchExpr, err := buildFTSMatchExpr(search)
        if err != nil {
                return "", nil, err
        }

        // Build the predicate. We use a subquery against the FTS table to
        // keep the main query's FROM clause untouched — the caller does not
        // need to know whether the search is backed by FTS or by LIKE.
        //
        //   <collectionAlias>.rowid IN (
        //     SELECT rowid FROM <fts_table> WHERE <fts_table> MATCH ?
        //   )
        //
        // The alias is used because the FROM clause emits the table with an
        // AS alias; SQLite requires aliased tables to be referenced by their
        // alias, not the original name.
        //
        // FTS5 is case-insensitive by default for ASCII text, so the
        // CaseSensitive flag is honored only approximately (FTS5 has no
        // per-query case sensitivity toggle; the choice is made at index
        // creation time). We accept the param so callers get the same shape
        // as the LIKE-based path.
        sql := fmt.Sprintf(
                "%s.%s IN (SELECT %s FROM %s WHERE %s MATCH %s)",
                collectionQuoted, ftsRowIDColumn,
                ftsRowIDColumn, ftsTable, ftsTable, param,
        )

        // Honor the per-field restriction by scoping the MATCH to the listed
        // columns (FTS5 {col1 col2} : query syntax).
        params := []any{scopeFTSMatch(search, matchExpr)}

        return sql, params, nil
}

// resolveFTSScope returns the quoted collection alias used in the FROM clause
// and the quoted FTS5 virtual table derived from the physical collection name
// (<physical>_fts). It is shared by the MATCH filter and the bm25 ranking
// clause so both address the same tables.
func resolveFTSScope(f *sqliteFactory) (collectionQuoted, ftsQuoted string, err error) {
        // primaryTargetName returns whichever name the FROM clause uses to
        // address the table — the alias when set, the physical name otherwise.
        // We derive the FTS table name from the ORIGINAL physical name by
        // looking it up via the factory's aliases map.
        collectionAlias := f.primaryTargetName()
        if collectionAlias == "" {
                return "", "", ErrSelectNoTargetSpecified.WithCause(
                        fmt.Errorf("text search requires a query target"))
        }

        // If collectionAlias is the alias (not the original), find the original
        // via the aliases map. If it is the original (no alias set), the same
        // name is used.
        physicalName := collectionAlias
        for original, alias := range f.aliases {
                if alias == collectionAlias {
                        physicalName = original
                        break
                }
        }

        // The alias is used because the FROM clause emits the table with an
        // AS alias; SQLite requires aliased tables to be referenced by their
        // alias, not the original name.
        return quoteIdentifier(collectionAlias), quoteIdentifier(ftsIndexName(physicalName)), nil
}

// validateFTSSearchFields rejects search fields that do not exist as columns
// on the FTS5 virtual table (which mirrors the collection's indexed fields).
func validateFTSSearchFields(f *sqliteFactory, schemas map[string]*definition.Schema, search *query.TextSearchQuery) error {
        for _, field := range search.Fields {
                if _, err := f.resolveFieldReference(field, schemas); err != nil {
                        return err
                }
        }
        return nil
}

// buildFTSMatchExpr renders the FTS5 MATCH right-hand side for a search:
//   - contains → "term1"* OR "term2"* (prefix match per term)
//   - exact / phrase → "whole query" (quoted match)
// Column scoping ({col} : ...) is applied by the caller, which owns the
// parameter placeholder.
func buildFTSMatchExpr(search *query.TextSearchQuery) (string, error) {
        var matchExpr string

        switch search.Type {
        case query.TextSearchTypeContains:
                // "Contains" semantics: any of the search terms appears as a
                // prefix in any indexed column. FTS5 prefix queries use the
                // trailing asterisk: term*. Multiple terms are joined with OR
                // so that "go database" matches rows containing either term.
                terms := tokenize(search.Query)
                if len(terms) == 0 {
                        return "", ErrSelectUnsupportedTextSearchType.WithCause(
                                fmt.Errorf("contains query is empty after tokenization"))
                }
                parts := make([]string, 0, len(terms))
                for _, t := range terms {
                        // Escape double quotes inside the term so the phrase
                        // quoting remains valid even for adversarial input.
                        escaped := strings.ReplaceAll(t, `"`, `""`)
                        parts = append(parts, `"`+escaped+`"*`)
                }
                op := " OR "
                if search.Operator == query.TextOperatorAnd {
                        op = " AND "
                }
                matchExpr = strings.Join(parts, op)

        case query.TextSearchTypeExact:
                // "Exact" semantics: the whole query string must appear as a
                // single token. We quote it as a phrase and require an exact
                // (non-prefix) match.
                escaped := strings.ReplaceAll(search.Query, `"`, `""`)
                matchExpr = `"` + escaped + `"`

        case query.TextSearchTypePhrase:
                // "Phrase" semantics: the whole query string must appear as a
                // contiguous phrase. FTS5 phrase queries are double-quoted.
                escaped := strings.ReplaceAll(search.Query, `"`, `""`)
                matchExpr = `"` + escaped + `"`

        default:
                return "", ErrSelectUnsupportedTextSearchType.WithCause(
                        fmt.Errorf("unsupported text search type: %s", search.Type))
        }

        return matchExpr, nil
}

// scopeFTSMatch applies the caller's per-field restriction to a match
// expression using FTS5's {col1 col2} : query column-scoped syntax.
func scopeFTSMatch(search *query.TextSearchQuery, matchExpr string) string {
        if len(search.Fields) == 0 {
                return matchExpr
        }
        var cols []string
        for _, f := range search.Fields {
                cols = append(cols, string(f))
        }
        return "{" + strings.Join(cols, " ") + "}" + " : " + matchExpr
}

// tokenize splits a free-text search query into individual terms suitable
// for FTS5 prefix matching. Whitespace separates terms; punctuation is
// preserved as part of the term (FTS5 tokenizes on whitespace by default
// for the unicode61 tokenizer). Empty terms are dropped.
func tokenize(s string) []string {
        if strings.TrimSpace(s) == "" {
                return nil
        }
        parts := strings.Fields(s)
        out := make([]string, 0, len(parts))
        for _, p := range parts {
                if p == "" {
                        continue
                }
                out = append(out, p)
        }
        return out
}

func (p *SQLiteSelectProjection) buildFilterValue(value *query.FilterValue) (string, []any, error) {
        if value.StringVal != nil {
                if strings.Contains(*value.StringVal, ";") {
                        return "", nil, ErrSelectUnsupportedFilterValue.WithCause(fmt.Errorf("unsupported filter value: %s", *value.StringVal))
                }
                param := p.factory.nextParam()
                return param, []any{*value.StringVal}, nil
        }
        if value.NumberVal != nil {
                param := p.factory.nextParam()
                return param, []any{*value.NumberVal}, nil
        }
        if value.BoolVal != nil {
                param := p.factory.nextParam()
                return param, []any{*value.BoolVal}, nil
        }
        if value.ArrayVal != nil {
                var placeholders []string
                var params []any
                for _, item := range value.ArrayVal {
                        itemSQL, itemParams, err := p.buildFilterValue(&item)
                        if err != nil {
                                return "", nil, err
                        }
                        placeholders = append(placeholders, itemSQL)
                        params = append(params, itemParams...)
                }
                return fmt.Sprintf("(%s)", strings.Join(placeholders, ", ")), params, nil
        }
        if value.FieldRefVal != nil {
                resolvedField, err := p.factory.resolveFieldReference(value.FieldRefVal.Field, p.schemas)
                if err != nil {
                        return "", nil, err
                }
                return resolvedField, nil, nil
        }
        if value.FunctionCallVal != nil {
                return p.buildFunctionCall(value.FunctionCallVal)
        }
        if value.SubqueryVal != nil {
                // For subqueries, we'd need to recursively build the query
                return p.buildSubquery(value.SubqueryVal)
        }
        if value.ObjectVal != nil {
                // ObjectVal can be either:
                // 1. A genuine object value (JSON column) — serialize to JSON.
                // 2. A mis-deserialized FieldRefVal — after JSON roundtrip
                //    via Query.Clone(), FilterValue{FieldRefVal: &FieldReference{Field: "x"}}
                //    becomes FilterValue{ObjectVal: map[string]any{"field":"x","type":""}}
                //    because the custom UnmarshalJSON requires type:"field" but
                //    FieldReference.Type defaults to "". Detect case 2 by
                //    checking for the "field" key.
                if fieldName, ok := value.ObjectVal["field"].(string); ok && fieldName != "" {
                        resolvedField, err := p.factory.resolveFieldReference(fieldName, p.schemas)
                        if err == nil {
                                return resolvedField, nil, nil
                        }
                        // If field resolution fails, treat as a
                        // genuine object value and fall through to JSON.
                }
                // Genuine object value — serialize to JSON before binding.
                jsonBytes, err := json.Marshal(value.ObjectVal)
                if err != nil {
                        return "", nil, ErrConvertMarshalValueFailed.WithCause(
                                fmt.Errorf("failed to marshal object filter value: %w", err))
                }
                param := p.factory.nextParam()
                return param, []any{string(jsonBytes)}, nil
        }

        return "NULL", nil, nil
}

// buildSubquery builds a complete subquery with full support for joins, nesting, etc.
func (p *SQLiteSelectProjection) buildSubquery(subquery *query.SubqueryValue) (string, []any, error) {
        // Check depth to prevent infinite recursion
        if err := p.factory.checkDepth(); err != nil {
                return "", nil, err
        }

        // Create a child factory for the subquery scope
        childFactory := p.factory.createChildScope()

        // Build the complete subquery tree
        // This recursively handles all features: joins, nested subqueries, aggregations, etc.
        subTree, err := childFactory.buildSelectTree(&subquery.Query)
        if err != nil {
                return "", nil, fmt.Errorf("failed to build subquery at depth %d: %w", childFactory.depth, err)
        }

        // Generate SQL for the subquery
        subSQL, subParams, err := subTree.Value()
        if err != nil {
                return "", nil, fmt.Errorf("failed to generate subquery SQL at depth %d: %w", childFactory.depth, err)
        }

        // Wrap in parentheses
        return fmt.Sprintf("(%s)", subSQL), subParams, nil
}

// SQLiteFromClause handles FROM clause
type SQLiteFromClause struct {
        factory *sqliteFactory
        target  *query.QueryTarget
}

func (f *SQLiteFromClause) Value() (string, []any, error) {
        if f.target == nil {
                return "", nil, ErrSelectNoTargetSpecified
        }

        sql := fmt.Sprintf("FROM %s", quoteIdentifier(f.target.Name))
        if f.target.Alias != nil {
                sql = fmt.Sprintf("%s AS %s", sql, quoteIdentifier(*f.target.Alias))
                f.factory.addAlias(f.target.Name, *f.target.Alias)
        }

        return sql, nil, nil
}

// SQLiteJoinClause handles JOIN clauses
type SQLiteJoinClause struct {
        factory *sqliteFactory
        joins   []query.JoinConfiguration
        schemas map[string]*definition.Schema
}

func (j *SQLiteJoinClause) Value() (string, []any, error) {
        if len(j.joins) == 0 {
                return "", nil, nil
        }

        var parts []string
        var params []any

        for _, join := range j.joins {
                var joinType string
                switch join.Type {
                case query.JoinTypeInner:
                        joinType = "INNER JOIN"
                case query.JoinTypeLeft:
                        joinType = "LEFT JOIN"
                case query.JoinTypeRight:
                        joinType = "RIGHT JOIN"
                case query.JoinTypeFull:
                        joinType = "FULL OUTER JOIN"
                default:
                        return "", nil, ErrSelectUnsupportedJoinType.WithCause(fmt.Errorf("unsupported join type: %s", join.Type))
                }

                joinSQL := fmt.Sprintf("%s %s", joinType, quoteIdentifier(join.Target.Name))
                if join.Target.Alias != nil {
                        joinSQL = fmt.Sprintf("%s AS %s", joinSQL, quoteIdentifier(*join.Target.Alias))
                        j.factory.addAlias(join.Target.Name, *join.Target.Alias)
                }

                if join.On != nil {
                        projection := &SQLiteSelectProjection{
                                factory: j.factory,
                                schemas: j.schemas,
                        }
                        onSQL, onParams, err := projection.buildQueryFilter(join.On)
                        if err != nil {
                                return "", nil, err
                        }
                        joinSQL = fmt.Sprintf("%s ON %s", joinSQL, onSQL)
                        params = append(params, onParams...)
                }

                parts = append(parts, joinSQL)
        }

        return strings.Join(parts, " "), params, nil
}

// SQLiteWhereClause handles WHERE clause
type SQLiteWhereClause struct {
        factory    *sqliteFactory
        filter     *query.QueryFilter
        projection *SQLiteSelectProjection
        pagination *query.PaginationOptions
}

func (w *SQLiteWhereClause) Value() (string, []any, error) {
        var parts []string
        var params []any

        // Build filter SQL if present
        if w.filter != nil {
                filterSQL, filterParams, err := w.projection.buildQueryFilter(w.filter)
                if err != nil {
                        return "", nil, err
                }
                if filterSQL != "" {
                        parts = append(parts, filterSQL)
                        params = append(params, filterParams...)
                }
        }

        // Build cursor SQL if present
        if w.pagination != nil && w.pagination.Type == query.PaginationTypeCursor && w.pagination.Cursor != nil {
                cursorField := w.pagination.Cursor.Field
                cursorValue := w.pagination.Cursor.Cursor

                if cursorField != nil && cursorValue != nil {
                        cursorOp := query.ComparisonOperatorGt
                        if len(w.pagination.Order) > 0 && w.pagination.Order[0].Direction == query.SortDirectionDesc {
                                cursorOp = query.ComparisonOperatorLt
                        }

                        cursorCondition := &query.FilterCondition{
                                Field:    *cursorField,
                                Operator: cursorOp,
                                Value:    *cursorValue,
                        }

                        cursorSQL, cursorParams, err := w.projection.buildFilterCondition(cursorCondition)
                        if err != nil {
                                return "", nil, err
                        }

                        if cursorSQL != "" {
                                parts = append(parts, cursorSQL)
                                params = append(params, cursorParams...)
                        }
                }
        }

        if len(parts) == 0 {
                return "", nil, nil
        }

        sql := "WHERE " + strings.Join(parts, " AND ")
        return sql, params, nil
}

// SQLiteGroupByClause handles GROUP BY clause
type SQLiteGroupByClause struct {
        factory      *sqliteFactory
        aggregations []query.AggregationConfiguration
        schemas      map[string]*definition.Schema
}

func (g *SQLiteGroupByClause) Value() (string, []any, error) {
        var groupFields []string

        for _, agg := range g.aggregations {
                if len(agg.Groups) > 0 {
                        for _, field := range agg.Groups {
                                resolvedField, err := g.factory.resolveFieldReference(field, g.schemas)
                                if err != nil {
                                        return "", nil, err
                                }
                                groupFields = append(groupFields, resolvedField)
                        }
                }
        }

        if len(groupFields) == 0 {
                return "", nil, nil
        }

        // Remove duplicates
        seen := make(map[string]bool)
        var uniqueFields []string
        for _, field := range groupFields {
                if !seen[field] {
                        seen[field] = true
                        uniqueFields = append(uniqueFields, field)
                }
        }

        return fmt.Sprintf("GROUP BY %s", strings.Join(uniqueFields, ", ")), nil, nil
}

// SQLiteHavingClause handles HAVING clause
type SQLiteHavingClause struct {
        factory      *sqliteFactory
        aggregations []query.AggregationConfiguration
        schemas      map[string]*definition.Schema
}

func (h *SQLiteHavingClause) Value() (string, []any, error) {
        var havingConditions []string
        var params []any

        for _, agg := range h.aggregations {
                if agg.Filter != nil {
                        projection := &SQLiteSelectProjection{
                                factory: h.factory,
                                schemas: h.schemas,
                        }
                        filterSQL, filterParams, err := projection.buildQueryFilter(agg.Filter)
                        if err != nil {
                                return "", nil, err
                        }
                        havingConditions = append(havingConditions, filterSQL)
                        params = append(params, filterParams...)
                }
        }

        if len(havingConditions) == 0 {
                return "", nil, nil
        }

        return fmt.Sprintf("HAVING %s", strings.Join(havingConditions, " AND ")), params, nil
}

// SQLiteOrderByClause handles ORDER BY clause
type SQLiteOrderByClause struct {
        factory    *sqliteFactory
        sorts      []query.SortConfiguration
        schemas    map[string]*definition.Schema
        pagination *query.PaginationOptions
        // rankSearch, when set with no explicit sorts, orders pure
        // text-search reads by FTS5 bm25 relevance (best match first).
        // See rankEligibleQuery in buildSelectTree.
        rankSearch *query.TextSearchQuery
}

func (o *SQLiteOrderByClause) Value() (string, []any, error) {
        allSorts := make([]query.SortConfiguration, 0, len(o.sorts))
        allSorts = append(allSorts, o.sorts...) // explicit user sorts

        if o.pagination != nil && len(o.pagination.Order) > 0 {
                allSorts = append(allSorts, o.pagination.Order...) // pagination sorts appended
        }

        // No explicit ordering on a pure text-search read: rank by relevance.
        // bm25() yields negative scores with lower-is-better, so a plain
        // ascending ORDER BY returns the best match first. The score is
        // computed per row in a correlated subquery running the same MATCH,
        // keeping the main FROM clause untouched (no JOIN restructuring).
        if len(allSorts) == 0 && o.rankSearch != nil {
                return o.buildRankOrder(o.rankSearch)
        }

        if len(allSorts) == 0 {
                return "", nil, nil // no ordering
        }

        var parts []string
        for _, sort := range allSorts {
                resolvedField, err := o.factory.resolveFieldReference(sort.Field, o.schemas, true)
                if err != nil {
                        return "", nil, err
                }
                direction := "ASC"
                if sort.Direction == query.SortDirectionDesc {
                        direction = "DESC"
                }
                parts = append(parts, fmt.Sprintf("%s %s", resolvedField, direction))
        }

        return fmt.Sprintf("ORDER BY %s", strings.Join(parts, ", ")), nil, nil
}

// buildRankOrder emits ORDER BY (SELECT bm25(...) ...) for a pure text-search
// read. The correlated subquery re-runs the same MATCH for the outer row and
// scores it; only rows that passed the identical WHERE filter reach ORDER BY,
// so bm25() is always evaluated in a valid MATCH context.
func (o *SQLiteOrderByClause) buildRankOrder(search *query.TextSearchQuery) (string, []any, error) {
        collectionQuoted, ftsTable, err := resolveFTSScope(o.factory)
        if err != nil {
                return "", nil, err
        }
        if err := validateFTSSearchFields(o.factory, o.schemas, search); err != nil {
                return "", nil, err
        }
        matchExpr, err := buildFTSMatchExpr(search)
        if err != nil {
                return "", nil, err
        }
        param := o.factory.nextParam()
        sql := fmt.Sprintf(
                "ORDER BY (SELECT bm25(%s) FROM %s WHERE %s MATCH %s AND %s.%s = %s.%s)",
                ftsTable, ftsTable, ftsTable, param,
                ftsTable, ftsRowIDColumn, collectionQuoted, ftsRowIDColumn,
        )
        return sql, []any{scopeFTSMatch(search, matchExpr)}, nil
}

// SQLiteLimitClause handles LIMIT and OFFSET clauses
type SQLiteLimitClause struct {
        factory    *sqliteFactory
        pagination *query.PaginationOptions
}

func (l *SQLiteLimitClause) Value() (string, []any, error) {
        if l.pagination == nil || len(l.pagination.Type) == 0{
                return "", nil, nil
        }

        if l.pagination.Limit <= 0 {
                return "", nil, ErrSelectLimitInvalid
        }

        sql := fmt.Sprintf("LIMIT %d", l.pagination.Limit)

        if l.pagination.Type == query.PaginationTypeOffset && l.pagination.Offset != nil && *l.pagination.Offset > 0 {
                sql += fmt.Sprintf(" OFFSET %d", *l.pagination.Offset)
        }

        return sql, nil, nil
}

// SQLiteUnionClause handles UNION operations
type SQLiteUnionClause struct {
        factory *sqliteFactory
        union   *query.QueryUnion
}

func (u *SQLiteUnionClause) Value() (string, []any, error) {
        if u.union == nil || len(u.union.Queries) == 0 {
                return "", nil, nil
        }

        var parts []string
        var params []any

        for i, subQuery := range u.union.Queries {
                subFactory := newSQLiteFactory(u.factory.logger)
                // UNION legs cannot carry ORDER BY without LIMIT — suppress
                // implicit clauses (e.g. bm25 ranking) in leg scopes.
                subFactory.inCompound = true
                selectTree, err := subFactory.buildSelectTree(&subQuery)
                if err != nil {
                        return "", nil, err
                }

                subSQL, subParams, err := selectTree.Value()
                if err != nil {
                        return "", nil, err
                }

                if i > 0 {
                        var unionType string
                        switch u.union.Type {
                        case "union":
                                unionType = "UNION"
                        case "all":
                                unionType = "UNION ALL"
                        case "intersect":
                                unionType = "INTERSECT"
                        case "except":
                                unionType = "EXCEPT"
                        default:
                                unionType = "UNION"
                        }
                        parts = append(parts, unionType)
                }

                parts = append(parts, fmt.Sprintf("(%s)", subSQL))
                params = append(params, subParams...)
        }

        return strings.Join(parts, " "), params, nil
}

// SQLiteSelectStatement represents a complete SELECT statement
type SQLiteSelectStatement struct {
        tree *selectTree
}

func (s *SQLiteSelectStatement) Value() (string, []any, error) {
        var sqlParts []string
        var allParams []any

        // Build each part of the query
        parts := []*struct {
                node SQLNode
                name string
        }{
                {s.tree.projection, "projection"},
                {s.tree.target, "target"},
                {s.tree.joins, "joins"},
                {s.tree.filters, "filters"},
                {s.tree.groupBy, "groupBy"},
                {s.tree.having, "having"},
                {s.tree.orderBy, "orderBy"},
                {s.tree.limit, "limit"},
        }

        for _, part := range parts {
                if part.node != nil {
                        sql, params, err := part.node.Value()
                        if err != nil {
                                return "", nil, ErrSelectBuildError.WithCause(fmt.Errorf("error building %s: %w", part.name, err))
                        }
                        if sql != "" {
                                sqlParts = append(sqlParts, sql)
                                allParams = append(allParams, params...)
                        }
                }
        }

        return strings.Join(sqlParts, " "), allParams, nil
}

func (s *SQLiteSelectStatement) StatementType() string {
        return "SELECT"
}

// rankEligibleQuery returns the text search to rank by when q is a pure
// text-search read: the whole filter is a single TextSearchQuery with no
// explicit ordering, aggregations, or distinct. Mixed filters (groups,
// conditions) and explicit sorts keep their existing behavior — ranking an
// arbitrary boolean combination by one of its clauses would be surprising,
// and an explicit sort always wins over implicit relevance.
//
// Ranking is top-level only (depth 0, outside UNION legs): ORDER BY is
// meaningless in CTAS/subquery scopes and illegal in compound legs.
func rankEligibleQuery(q *query.Query, f *sqliteFactory) *query.TextSearchQuery {
        if f.depth != 0 || f.inCompound {
                return nil
        }
        if q.Filters == nil || q.Filters.TextSearchQuery == nil {
                return nil
        }
        if q.Filters.Condition != nil || q.Filters.Group != nil {
                return nil
        }
        if len(q.Aggregations) > 0 {
                return nil
        }
        if q.Distinct != nil && ((q.Distinct.IsDistinct != nil && *q.Distinct.IsDistinct) || len(q.Distinct.Fields) > 0) {
                return nil
        }
        return q.Filters.TextSearchQuery
}

// buildSelectTree builds the complete query tree with proper schema context.
// This is now recursive and handles subqueries at any depth.
func (f *sqliteFactory) buildSelectTree(q *query.Query) (SQLNode, error) {
        tree := &selectTree{}

        // Collect schema from the main target and register alias
        if q.Target != nil {
                name := q.Target.Name
                if q.Target.Alias != nil {
                        name = *q.Target.Alias
                        f.addAlias(q.Target.Name, name)
                }
                if q.Target.Schema != nil {
                        f.addSchema(name, q.Target.Schema)
                }
        }

        // Collect schemas from joins and register aliases
        for _, join := range q.Joins {
                name := join.Target.Name
                if join.Target.Alias != nil {
                        name = *join.Target.Alias
                        f.addAlias(join.Target.Name, name)
                }
                if join.Target.Schema != nil {
                        f.addSchema(name, join.Target.Schema)
                }
        }

        // Build projection
        tree.projection = &SQLiteSelectProjection{
                factory:      f,
                projection:   q.Projection,
                aggregations: q.Aggregations,
                distinct:     q.Distinct,
                schemas:      f.schemas,
                total: q.Pagination != nil && q.Pagination.IncludeTotal != nil && *q.Pagination.IncludeTotal,
        }

        // Build target (FROM clause)
        if q.Target != nil {
                tree.target = &SQLiteFromClause{
                        factory: f,
                        target:  q.Target,
                }
        }

        // Build joins
        if len(q.Joins) > 0 {
                tree.joins = &SQLiteJoinClause{
                        factory: f,
                        joins:   q.Joins,
                        schemas: f.schemas,
                }
        }

        // Build filters (WHERE clause)
        if q.Filters != nil || (q.Pagination != nil && q.Pagination.Type == query.PaginationTypeCursor) {
                tree.filters = &SQLiteWhereClause{
                        factory:    f,
                        filter:     q.Filters,
                        projection: tree.projection.(*SQLiteSelectProjection),
                        pagination: q.Pagination,
                }
        }

        // Build GROUP BY clause
        if len(q.Aggregations) > 0 {
                tree.groupBy = &SQLiteGroupByClause{
                        factory:      f,
                        aggregations: q.Aggregations,
                        schemas:      f.schemas,
                }

                // Build HAVING clause for aggregation filters
                tree.having = &SQLiteHavingClause{
                        factory:      f,
                        aggregations: q.Aggregations,
                        schemas:      f.schemas,
                }
        }

        // Build ORDER BY clause. Explicit sorts (or pagination order) win.
        // A pure text-search read with no explicit ordering ranks by bm25
        // relevance instead of returning storage order.
        if len(q.Sort) > 0 || (q.Pagination != nil && len(q.Pagination.Order) > 0) {
                tree.orderBy = &SQLiteOrderByClause{
                        factory:    f,
                        sorts:      q.Sort,
                        schemas:    f.schemas,
                        pagination: q.Pagination,
                }
        } else if rankSearch := rankEligibleQuery(q, f); rankSearch != nil {
                tree.orderBy = &SQLiteOrderByClause{
                        factory:    f,
                        schemas:    f.schemas,
                        pagination: q.Pagination,
                        rankSearch: rankSearch,
                }
        }

        // Build LIMIT clause
        if q.Pagination != nil {
                tree.limit = &SQLiteLimitClause{
                        factory:    f,
                        pagination: q.Pagination,
                }
        }

        return &SQLiteSelectStatement{tree: tree}, nil
}
