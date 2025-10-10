package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/data"
	"github.com/asaidimu/go-anansi/v6/core/query"
	"github.com/asaidimu/go-anansi/v6/core/schema"
)

// updateAssignment is an internal struct to represent a single field update,
// unifying both simple value sets and computed expressions.
type updateAssignment struct {
	fieldPath  string
	value      any // For 'set', this is the direct value. For 'compute', this is the query.Query.
	isComputed bool
}

// isJSONType is a helper to check if a schema field should be treated as a JSON object.
func isJSONType(field *schema.FieldDefinition) bool {
	if field == nil {
		return false
	}
	switch field.Type {
	case schema.FieldTypeObject, schema.FieldTypeArray, schema.FieldTypeSet, schema.FieldTypeRecord, schema.FieldTypeUnion:
		return true
	default:
		return false
	}
}

// sqliteUpdateAssignments handles the SET clause in an UPDATE statement.
type sqliteUpdateAssignments struct {
	factory *sqliteFactory
	set     map[string]any
	compute map[string]query.Query
	schema  *schema.SchemaDefinition
}

func (u *sqliteUpdateAssignments) Value() (string, []any, error) {
	if len(u.set) == 0 && len(u.compute) == 0 {
		return "", nil, nil
	}

	// 1. Unify all 'set' and 'compute' operations into a single slice for stable processing.
	assignments := make([]updateAssignment, 0, len(u.set)+len(u.compute))
	for path, val := range u.set {
		assignments = append(assignments, updateAssignment{fieldPath: path, value: val})
	}
	for path, val := range u.compute {
		assignments = append(assignments, updateAssignment{fieldPath: path, value: val, isComputed: true})
	}

	// Sort by field path to ensure deterministic order for parameter generation.
	sort.Slice(assignments, func(i, j int) bool {
		return assignments[i].fieldPath < assignments[j].fieldPath
	})

	// 2. Process the sorted assignments, populating intermediate maps.
	relationalParts := make(map[string]string)
	relationalParams := make(map[string]any)
	jsonUpdates := make(map[string]map[string]any) // parentColumn -> {jsonPath: valueOrExpr}
	jsonParams := make(map[string][]any)          // parentColumn -> params

	for _, assign := range assignments {
		fieldParts := strings.Split(assign.fieldPath, ".")
		topLevelField := u.schema.FindField(fieldParts[0])

		// Determine if this update targets a nested field within a JSON column.
		if len(fieldParts) > 1 && isJSONType(topLevelField) {
			parentColumn := fieldParts[0]
			jsonPath := "$." + strings.Join(fieldParts[1:], ".")

			if _, ok := jsonUpdates[parentColumn]; !ok {
				jsonUpdates[parentColumn] = make(map[string]any)
				jsonParams[parentColumn] = []any{}
			}

			if assign.isComputed {
				q := assign.value.(query.Query)
				expr, params, err := u.buildSetClauseExpression(&q)
				if err != nil {
					return "", nil, err
				}
				// FIX: Strip outer parentheses for expressions used inside json_set.
				if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
					expr = expr[1 : len(expr)-1]
				}
				jsonUpdates[parentColumn][jsonPath] = expr
				jsonParams[parentColumn] = append(jsonParams[parentColumn], params...)
			} else {
				param := u.factory.nextParam()
				jsonUpdates[parentColumn][jsonPath] = param
				fieldDef := u.schema.FindField(assign.fieldPath)
				convertedValue, err := toSQLiteValue(fieldDef, assign.value)
				if err != nil {
					return "", nil, err
				}
				jsonParams[parentColumn] = append(jsonParams[parentColumn], convertedValue)
			}
		} else { // This is a standard relational column update.
			if assign.isComputed {
				q := assign.value.(query.Query)
				expr, params, err := u.buildSetClauseExpression(&q)
				if err != nil {
					return "", nil, err
				}
				// FIX: Don't add extra parentheses; the expression is already parenthesized.
				relationalParts[assign.fieldPath] = fmt.Sprintf("%s = %s", quoteIdentifier(assign.fieldPath), expr)
				relationalParams[assign.fieldPath] = params
			} else {
				param := u.factory.nextParam()
				relationalParts[assign.fieldPath] = fmt.Sprintf("%s = %s", quoteIdentifier(assign.fieldPath), param)
				convertedValue, err := toSQLiteValue(topLevelField, assign.value)
				if err != nil {
					return "", nil, err
				}
				relationalParams[assign.fieldPath] = convertedValue
			}
		}
	}

	// 3. Assemble the final SQL string and parameters from the processed parts.
	var finalParts []string
	var finalParams []any

	// Process relational parts (already sorted by virtue of processing sorted assignments)
	relationalKeys := make([]string, 0, len(relationalParts))
	for k := range relationalParts {
		relationalKeys = append(relationalKeys, k)
	}
	sort.Strings(relationalKeys)
	for _, k := range relationalKeys {
		finalParts = append(finalParts, relationalParts[k])
		if params, ok := relationalParams[k].([]any); ok {
			finalParams = append(finalParams, params...)
		} else {
			finalParams = append(finalParams, relationalParams[k])
		}
	}

	// Process JSON parts
	jsonKeys := make([]string, 0, len(jsonUpdates))
	for k := range jsonUpdates {
		jsonKeys = append(jsonKeys, k)
	}
	sort.Strings(jsonKeys)
	for _, parentColumn := range jsonKeys {
		updates := jsonUpdates[parentColumn]
		paths := make([]string, 0, len(updates))
		for path := range updates {
			paths = append(paths, path)
		}
		sort.Strings(paths) // Sort paths within a single json_set for determinism.

		var jsonSetArgs []string
		for _, path := range paths {
			jsonSetArgs = append(jsonSetArgs, fmt.Sprintf("'%s'", path), fmt.Sprintf("%v", updates[path]))
		}

		expr := fmt.Sprintf("json_set(%s, %s)", quoteIdentifier(parentColumn), strings.Join(jsonSetArgs, ", "))
		finalParts = append(finalParts, fmt.Sprintf("%s = %s", quoteIdentifier(parentColumn), expr))
		finalParams = append(finalParams, jsonParams[parentColumn]...)
	}

	if len(finalParts) == 0 {
		return "", nil, nil
	}

	return fmt.Sprintf("SET %s", strings.Join(finalParts, ", ")), finalParams, nil
}

// buildSetClauseExpression generates the SQL expression for a computed value.
// It no longer wraps subqueries in parentheses, allowing the caller to decide.
func (u *sqliteUpdateAssignments) buildSetClauseExpression(q *query.Query) (string, []any, error) {
	// Case 1: Scalar Subquery
	if q.Target != nil {
		selectTree, err := u.factory.buildSelectTree(q)
		if err != nil {
			return "", nil, err
		}
		// Return raw subquery; caller will add parentheses if needed.
		return selectTree.Value()
	}

	// Case 2: Inline Expression
	if q.Projection != nil && len(q.Projection.Computed) == 1 {
		computed := q.Projection.Computed[0]
		if computed.ComputedFieldExpression != nil {
			schemas := map[string]*schema.SchemaDefinition{}
			if u.schema != nil {
				schemas[u.schema.Name] = u.schema
			}
			proj := &SQLiteSelectProjection{factory: u.factory, schemas: schemas}
			return proj.buildFunctionCall(computed.ComputedFieldExpression.Expression)
		}
	}

	return "", nil, fmt.Errorf("invalid query for computed field: %+v", q)
}

// SQLiteUpdateStatement represents a complete UPDATE statement
type SQLiteUpdateStatement struct {
	tree *updateTree
}

func (s *SQLiteUpdateStatement) Value() (string, []any, error) {
	var sqlParts []string
	var allParams []any

	// UPDATE clause (target)
	if s.tree.target == nil {
		return "", nil, fmt.Errorf("update statement must have a target")
	}
	targetSQL, targetParams, err := s.tree.target.Value()
	if err != nil {
		return "", nil, err
	}
	sqlParts = append(sqlParts, targetSQL)
	allParams = append(allParams, targetParams...)

	// SET clause
	if s.tree.assignments == nil {
		return "", nil, fmt.Errorf("update statement must have assignments")
	}
	assignmentsSQL, assignmentsParams, err := s.tree.assignments.Value()
	if err != nil {
		return "", nil, err
	}
	if assignmentsSQL != "" {
		sqlParts = append(sqlParts, assignmentsSQL)
		allParams = append(allParams, assignmentsParams...)
	}

	// WHERE clause
	if s.tree.filters != nil {
		filterSQL, filterParams, err := s.tree.filters.Value()
		if err != nil {
			return "", nil, err
		}
		if filterSQL != "" {
			sqlParts = append(sqlParts, filterSQL)
			allParams = append(allParams, filterParams...)
		}
	}

	return strings.Join(sqlParts, " "), allParams, nil
}

func (s *SQLiteUpdateStatement) StatementType() string {
	return "UPDATE"
}

// sqliteUpdateTargetClause handles the UPDATE clause
type sqliteUpdateTargetClause struct {
	target *query.QueryTarget
}

func (u *sqliteUpdateTargetClause) Value() (string, []any, error) {
	if u.target == nil {
		return "", nil, fmt.Errorf("no target specified for update")
	}
	return fmt.Sprintf("UPDATE %s", quoteIdentifier(u.target.Name)), nil, nil
}

// buildUpdateTree builds a SQLNode for an UPDATE statement.
func (f *sqliteFactory) buildUpdateTree(q *query.Query, extra any) (SQLNode, error) {
	if q.Target == nil {
		return nil, fmt.Errorf("update query must have a target")
	}
	if extra == nil {
		return nil, fmt.Errorf("update query must have data payload")
	}

	updatePayload, ok := extra.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid data type for update payload: expected map[string]any, got %T", extra)
	}

	var setData data.Document
	if setVal, ok := updatePayload["set"]; ok && setVal != nil {
		setData, ok = data.AsDocument(setVal)
		if !ok {
			return nil, fmt.Errorf("invalid data type for 'set' in update: %T", setVal)
		}
	}

	var computeData map[string]query.Query
	if computeVal, ok := updatePayload["compute"]; ok && computeVal != nil {
		computeData, ok = computeVal.(map[string]query.Query)
		if !ok {
			return nil, fmt.Errorf("invalid data type for 'compute' in update: %T", computeVal)
		}
	}

	tree := &updateTree{
		target: &sqliteUpdateTargetClause{
			target: q.Target,
		},
		assignments: &sqliteUpdateAssignments{
			factory: f,
			set:     setData,
			compute: computeData,
			schema:  q.Target.Schema,
		},
	}

	if q.Filters != nil {
		schemas := make(map[string]*schema.SchemaDefinition)
		if q.Target.Schema != nil {
			name := q.Target.Name
			if q.Target.Alias != nil {
				name = *q.Target.Alias
			}
			schemas[name] = q.Target.Schema
		}

		tree.filters = &SQLiteWhereClause{
			factory: f,
			filter:  q.Filters,
			projection: &SQLiteSelectProjection{
				factory: f,
				schemas: schemas,
			},
		}
	}

	return &SQLiteUpdateStatement{tree: tree}, nil
}
