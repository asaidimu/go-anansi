package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

func (f *sqliteFactory) nextParam() string {
	f.paramCounter++
	return fmt.Sprintf("$%d", f.paramCounter)
}

func (f *sqliteFactory) addAlias(original, alias string) {
	f.aliases[original] = alias
}

func (f *sqliteFactory) resolveFieldReference(fieldRef string, schemas map[string]*schema.SchemaDefinition) (string, error) {
	if !isValidIdentifier(fieldRef) {
		return "", fmt.Errorf("unsupported field reference: %s", fieldRef)
	}

	parts := strings.Split(fieldRef, ".")
	if len(parts) > 1 {
		if schema, ok := schemas[parts[0]]; ok {
			if fieldDef := schema.FindField(parts[1]); fieldDef != nil && fieldDef.Type.IsComplex() {
				jsonPath := "$." + strings.Join(parts[2:], ".")
				return fmt.Sprintf("json_extract(%s, '%s')", quoteIdentifier(parts[0])+"."+quoteIdentifier(parts[1]), jsonPath), nil
			}
		}
	}

	quotedParts := make([]string, len(parts))
	for i, part := range parts {
		quotedParts[i] = quoteIdentifier(part)
	}
	return strings.Join(quotedParts, "."), nil
}

// SQLiteSelectProjection handles SELECT clause projection
type SQLiteSelectProjection struct {
	factory      *sqliteFactory
	projection   *query.ProjectionConfiguration
	aggregations []query.AggregationConfiguration
	distinct     *query.QueryDistinctConfig
	schemas      map[string]*schema.SchemaDefinition
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

	// Handle aggregations first
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
				return "", nil, fmt.Errorf("unsupported aggregation type: %s", agg.Type)
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

	// Default to SELECT * if no specific fields
	if len(parts) == 0 {
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
				// Ensure the schema definition and its fields are available
				if schemaDef != nil && len(schemaDef.Fields) > 0 {
					for _, field := range schemaDef.GetFields() {
						// Qualify the field with the table alias (e.g., "u.id")
						resolvedField := fmt.Sprintf("%s.%s", alias, field.Name)
						// Create a unique alias for the column (e.g., "u_id")
						fieldAlias := fmt.Sprintf("'%s.%s'", alias, field.Name)
						aliasedFields = append(aliasedFields, fmt.Sprintf("%s AS %s", resolvedField, fieldAlias))
					}
				}
			}
			if len(aliasedFields) > 0 {
				parts = append(parts, aliasedFields...)
			} else {
				// Fallback to '*' if schemas were present but had no fields.
				parts = append(parts, "*")
			}
		} else {
			// Fallback to SELECT * if no schemas are available at all.
			parts = append(parts, "*")
		}
	}

	sql := fmt.Sprintf("SELECT %s%s", distinctClause, strings.Join(parts, ", "))
	return sql, params, nil
}

func (p *SQLiteSelectProjection) buildFunctionCall(fc *query.FunctionCall) (string, []any, error) {
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
	return "", nil, fmt.Errorf("empty filter")
}

func (p *SQLiteSelectProjection) buildFilterCondition(condition *query.FilterCondition) (string, []any, error) {
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
		return "", nil, fmt.Errorf("unsupported operator: %s", condition.Operator)
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
		return "", nil, fmt.Errorf("unsupported logical operator: %s", group.Operator)
	}

	sql := fmt.Sprintf("(%s)", strings.Join(conditions, fmt.Sprintf(" %s ", operator)))
	return sql, params, nil
}

func (p *SQLiteSelectProjection) buildTextSearch(search *query.TextSearchQuery) (string, []any, error) {
	var conditions []string
	var params []any

	searchValue := search.Query
	if search.CaseSensitive == nil || !*search.CaseSensitive {
		searchValue = strings.ToLower(searchValue)
	}

	for _, field := range search.Fields {
		resolvedField, err := p.factory.resolveFieldReference(field, p.schemas)
		if err != nil {
			return "", nil, err
		}
		var condition string
		param := p.factory.nextParam()

		switch search.Type {
		case query.TextSearchTypeContains:
			if search.CaseSensitive == nil || !*search.CaseSensitive {
				condition = fmt.Sprintf("LOWER(%s) LIKE %s", resolvedField, param)
				params = append(params, "%"+searchValue+"%")
			} else {
				condition = fmt.Sprintf("%s LIKE %s", resolvedField, param)
				params = append(params, "%"+searchValue+"%")
			}
		case query.TextSearchTypeExact:
			if search.CaseSensitive == nil || !*search.CaseSensitive {
				condition = fmt.Sprintf("LOWER(%s) = %s", resolvedField, param)
				params = append(params, searchValue)
			} else {
				condition = fmt.Sprintf("%s = %s", resolvedField, param)
				params = append(params, searchValue)
			}
		case query.TextSearchTypePhrase:
			// For phrase search, we'll use FTS if available, otherwise fall back to LIKE
			if search.CaseSensitive == nil || !*search.CaseSensitive {
				condition = fmt.Sprintf("LOWER(%s) LIKE %s", resolvedField, param)
				params = append(params, "%"+searchValue+"%")
			} else {
				condition = fmt.Sprintf("%s LIKE %s", resolvedField, param)
				params = append(params, "%"+searchValue+"%")
			}
		default:
			return "", nil, fmt.Errorf("unsupported text search type: %s", search.Type)
		}

		conditions = append(conditions, condition)
	}

	var operator string
	switch search.Operator {
	case query.TextOperatorAnd:
		operator = "AND"
	case query.TextOperatorOr:
		operator = "OR"
	default:
		operator = "OR" // Default to OR
	}

	sql := strings.Join(conditions, fmt.Sprintf(" %s ", operator))
	return fmt.Sprintf("(%s)", sql), params, nil
}

func (p *SQLiteSelectProjection) buildFilterValue(value *query.FilterValue) (string, []any, error) {
	if value.StringVal != nil {
		if strings.Contains(*value.StringVal, ";") {
			return "", nil, fmt.Errorf("unsupported filter value: %s", *value.StringVal)
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
		// This is a simplified implementation
		return "(SELECT ...)", nil, fmt.Errorf("subquery support not yet implemented")
	}
	if value.ObjectVal != nil {
		// For object values, we might serialize to JSON
		param := p.factory.nextParam()
		return param, []any{value.ObjectVal}, nil
	}

	return "NULL", nil, nil
}

// SQLiteFromClause handles FROM clause
type SQLiteFromClause struct {
	factory *sqliteFactory
	target  *query.QueryTarget
}

func (f *SQLiteFromClause) Value() (string, []any, error) {
	if f.target == nil {
		return "", nil, fmt.Errorf("no target specified")
	}

	sql := fmt.Sprintf("FROM %s", f.target.Name)
	if f.target.Alias != nil {
		sql = fmt.Sprintf("%s AS %s", sql, *f.target.Alias)
		f.factory.addAlias(f.target.Name, *f.target.Alias)
	}

	return sql, nil, nil
}

// SQLiteJoinClause handles JOIN clauses
type SQLiteJoinClause struct {
	factory *sqliteFactory
	joins   []query.JoinConfiguration
	schemas map[string]*schema.SchemaDefinition
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
			return "", nil, fmt.Errorf("unsupported join type: %s", join.Type)
		}

		joinSQL := fmt.Sprintf("%s %s", joinType, join.Target.Name)
		if join.Target.Alias != nil {
			joinSQL = fmt.Sprintf("%s AS %s", joinSQL, *join.Target.Alias)
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
}

func (w *SQLiteWhereClause) Value() (string, []any, error) {
	if w.filter == nil {
		return "", nil, nil
	}

	filterSQL, params, err := w.projection.buildQueryFilter(w.filter)
	if err != nil {
		return "", nil, err
	}

	return fmt.Sprintf("WHERE %s", filterSQL), params, nil
}

// SQLiteGroupByClause handles GROUP BY clause
type SQLiteGroupByClause struct {
	factory      *sqliteFactory
	aggregations []query.AggregationConfiguration
	schemas      map[string]*schema.SchemaDefinition
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
	schemas      map[string]*schema.SchemaDefinition
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
	factory *sqliteFactory
	sorts   []query.SortConfiguration
	schemas map[string]*schema.SchemaDefinition
}

func (o *SQLiteOrderByClause) Value() (string, []any, error) {
	if len(o.sorts) == 0 {
		return "", nil, nil
	}

	var parts []string
	for _, sort := range o.sorts {
		resolvedField, err := o.factory.resolveFieldReference(sort.Field, o.schemas)
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

// SQLiteLimitClause handles LIMIT and OFFSET clauses
type SQLiteLimitClause struct {
	factory    *sqliteFactory
	pagination *query.PaginationOptions
}

func (l *SQLiteLimitClause) Value() (string, []any, error) {
	if l.pagination == nil {
		return "", nil, nil
	}

	sql := fmt.Sprintf("LIMIT %d", l.pagination.Limit)
	if l.pagination.Offset != nil && *l.pagination.Offset > 0 {
		sql = fmt.Sprintf("%s OFFSET %d", sql, *l.pagination.Offset)
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
		subFactory := newSQLiteFactory()
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
				return "", nil, fmt.Errorf("error building %s: %w", part.name, err)
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

// buildSelectTree builds the complete query tree with proper schema context
func (f *sqliteFactory) buildSelectTree(q *query.Query) (SQLNode, error) {
	tree := &selectTree{}
	schemas := make(map[string]*schema.SchemaDefinition)

	// Collect schema from the main target and register alias
	if q.Target != nil {
		name := q.Target.Name
		if q.Target.Alias != nil {
			name = *q.Target.Alias
			f.addAlias(q.Target.Name, name)
		}
		if q.Target.Schema != nil {
			schemas[name] = q.Target.Schema
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
			schemas[name] = join.Target.Schema
		}
	}

	// Build projection
	tree.projection = &SQLiteSelectProjection{
		factory:      f,
		projection:   q.Projection,
		aggregations: q.Aggregations,
		distinct:     q.Distinct,
		schemas:      schemas,
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
			schemas: schemas,
		}
	}

	// Build filters (WHERE clause)
	if q.Filters != nil {
		tree.filters = &SQLiteWhereClause{
			factory:    f,
			filter:     q.Filters,
			projection: tree.projection.(*SQLiteSelectProjection),
		}
	}

	// Build GROUP BY clause
	if len(q.Aggregations) > 0 {
		tree.groupBy = &SQLiteGroupByClause{
			factory:      f,
			aggregations: q.Aggregations,
			schemas:      schemas,
		}

		// Build HAVING clause for aggregation filters
		tree.having = &SQLiteHavingClause{
			factory:      f,
			aggregations: q.Aggregations,
			schemas:      schemas,
		}
	}

	// Build ORDER BY clause
	if len(q.Sort) > 0 {
		tree.orderBy = &SQLiteOrderByClause{
			factory: f,
			sorts:   q.Sort,
			schemas: schemas,
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
