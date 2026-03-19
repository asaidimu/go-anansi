package ir

import (
	"github.com/asaidimu/go-anansi/v6/core/document"
)

// constraints.go implements Pass 11: resolve root constraints into the hot
// ResolvedConstraintTree form.
//
// Constraints are defined at the root of the source document and are resolved
// relative to the root schema using absolute field paths. Every field path
// is converted to a document.DocumentKey using cs.DocumentKey(), ensuring that
// the 64-bit key carries both the ordinal and the field descriptor.

// buildResolvedConstraints resolves all constraints in the source schema.
func buildResolvedConstraints(
	cs *CompiledSchema,
	predicates PredicateMap,
	src *sourceSchema,
) (*ResolvedConstraintTree, []CompileError) {
	if len(src.Constraints) == 0 {
		return nil, nil
	}

	tree, treeErrs := buildConstraintTree(src.Constraints)
	if tree == nil {
		return nil, treeErrs
	}

	rt := &ResolvedConstraintTree{
		Index: make(map[uint16]ResolvedConstraintNode, len(tree.Ordinals)),
	}
	var errs []CompileError
	errs = append(errs, treeErrs...)

	for _, root := range tree.Roots {
		rn, nodeErrs := resolveConstraintNode(cs, root, tree, rt, predicates)
		errs = append(errs, nodeErrs...)
		rt.Roots = append(rt.Roots, rn)
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return rt, nil
}

func resolveConstraintNode(
	cs *CompiledSchema,
	node ConstraintNode,
	tree *ConstraintTree,
	rt *ResolvedConstraintTree,
	predicates PredicateMap,
) (ResolvedConstraintNode, []CompileError) {
	switch n := node.(type) {
	case Constraint:
		return resolveLeafConstraint(cs, n, tree, rt, predicates)
	case ConstraintGroup:
		return resolveGroupConstraint(cs, n, tree, rt, predicates)
	default:
		return nil, []CompileError{{
			Pass:    PassConstraints,
			Message: "unknown ConstraintNode type — internal compiler error",
		}}
	}
}

func resolveLeafConstraint(
	cs *CompiledSchema,
	c Constraint,
	tree *ConstraintTree,
	rt *ResolvedConstraintTree,
	predicates PredicateMap,
) (ResolvedConstraintNode, []CompileError) {
	var errs []CompileError

	pred, ok := predicates[c.Predicate]
	if !ok {
		errs = append(errs, CompileError{
			Pass:    PassConstraints,
			Message: "unknown predicate: " + c.Predicate,
		})
	}

	fields := make([]document.DocumentKey, 0, len(c.Fields))
	for _, path := range c.Fields {
		dk, err := cs.DocumentKey(path)
		if err != nil {
			errs = append(errs, CompileError{
				Pass:    PassConstraints,
				Message: "constraint " + c.Name + ": cannot resolve absolute field path " + path + ": " + err.Error(),
			})
			continue
		}
		fields = append(fields, dk)
	}

	rc := ResolvedConstraint{
		UUID:          c.UUID,
		Name:          c.Name,
		Description:   c.Description,
		PredicateName: c.Predicate,
		Predicate:     pred,
		Fields:        fields,
		Parameters:    n_parameters(c.Parameters),
	}

	if ordinal, hasOrdinal := tree.Ordinals[c.UUID]; hasOrdinal {
		rt.Index[ordinal] = rc
	}

	return rc, errs
}

func resolveGroupConstraint(
	cs *CompiledSchema,
	g ConstraintGroup,
	tree *ConstraintTree,
	rt *ResolvedConstraintTree,
	predicates PredicateMap,
) (ResolvedConstraintNode, []CompileError) {
	rg := ResolvedConstraintGroup{
		UUID:        g.UUID,
		Name:        g.Name,
		Description: g.Description,
		Operator:    g.Operator,
	}
	var errs []CompileError

	for _, child := range g.Constraints {
		rc, childErrs := resolveConstraintNode(cs, child, tree, rt, predicates)
		errs = append(errs, childErrs...)
		rg.Constraints = append(rg.Constraints, rc)
	}

	if ordinal, hasOrdinal := tree.Ordinals[g.UUID]; hasOrdinal {
		rt.Index[ordinal] = rg
	}

	return rg, errs
}

// n_parameters ensures parameters are normalized.
func n_parameters(p any) any {
	if p == nil {
		return nil
	}
	return p
}

// ── Helpers ───────────────────────────────────────────────────────────────

func buildConstraintTree(constraints map[string]sourceConstraint) (*ConstraintTree, []CompileError) {
	if len(constraints) == 0 {
		return nil, nil
	}
	tree := &ConstraintTree{
		Index:    make(map[string]ConstraintNode),
		Ordinals: make(map[string]uint16),
	}
	var errs []CompileError
	var ordinal uint16

	uuids := sortedKeys(constraints)
	for _, uuid := range uuids {
		c := constraints[uuid]
		node, nodeErrs := buildConstraintNode(uuid, c, tree, &ordinal)
		errs = append(errs, nodeErrs...)
		tree.Roots = append(tree.Roots, node)
	}

	return tree, errs
}

func buildConstraintNode(
	uuid string,
	src sourceConstraint,
	tree *ConstraintTree,
	ordinal *uint16,
) (ConstraintNode, []CompileError) {
	var errs []CompileError

	thisOrdinal := *ordinal
	*ordinal++
	tree.Ordinals[uuid] = thisOrdinal

	if src.Operator != "" && len(src.Rules) > 0 {
		op, ok := parseLogicalOperator(src.Operator)
		if !ok {
			errs = append(errs, CompileError{
				Pass:    PassMeta,
				Message: "unknown logical operator in constraint group: " + src.Operator,
			})
		}
		group := ConstraintGroup{
			UUID:        uuid,
			Name:        src.Name,
			Description: src.Description,
			Operator:    op,
		}
		for i, rule := range src.Rules {
			syntheticUUID := uuid + "/rule/" + itoa(i)
			child, childErrs := buildConstraintNode(syntheticUUID, *rule, tree, ordinal)
			errs = append(errs, childErrs...)
			group.Constraints = append(group.Constraints, child)
		}
		tree.Index[uuid] = group
		return group, errs
	}

	leaf := Constraint{
		UUID:        uuid,
		Name:        src.Name,
		Description: src.Description,
		Predicate:   src.Predicate,
		Fields:      src.Fields,
		Parameters:  src.Parameters,
	}
	tree.Index[uuid] = leaf
	return leaf, errs
}
