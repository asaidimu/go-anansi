package ir

import "github.com/asaidimu/go-anansi/v6/core/document"

// constraints.go implements Pass 11: resolve every schema's constraint forest
// into the hot ResolvedConstraintTree form. Predicate names are looked up in
// the provided PredicateMap — an unknown predicate is a compile error. Field
// path strings are resolved via cs.Address().
//
// Pass 11 runs after the address space is built so cs.Address() is available.

// buildResolvedConstraints populates CompiledSchema.ResolvedConstraints for
// every schema that has a ConstraintTree in its SchemaMetadata.
func buildResolvedConstraints(
	cs *CompiledSchema,
	si *schemaIndex,
	predicates PredicateMap,
) (map[uint8]*ResolvedConstraintTree, []CompileError) {
	resolved := make(map[uint8]*ResolvedConstraintTree)
	var errs []CompileError

	if m := cs.Meta[0]; m != nil && m.Constraints != nil {
		rt, rtErrs := resolveConstraintTree(cs, m.Constraints, predicates)
		errs = append(errs, rtErrs...)
		resolved[0] = rt
	}

	for _, uuid := range si.order {
		schemaIdx := si.byUUID[uuid]
		m := cs.Meta[schemaIdx]
		if m == nil || m.Constraints == nil {
			continue
		}
		rt, rtErrs := resolveConstraintTree(cs, m.Constraints, predicates)
		for i := range rtErrs {
			rtErrs[i].SchemaUUID = uuid
		}
		errs = append(errs, rtErrs...)
		resolved[schemaIdx] = rt
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return resolved, nil
}

func resolveConstraintTree(
	cs *CompiledSchema,
	tree *ConstraintTree,
	predicates PredicateMap,
) (*ResolvedConstraintTree, []CompileError) {
	rt := &ResolvedConstraintTree{
		Index: make(map[uint16]ResolvedConstraintNode, len(tree.Ordinals)),
	}
	var errs []CompileError

	for _, root := range tree.Roots {
		rn, nodeErrs := resolveConstraintNode(cs, root, tree, rt, predicates)
		errs = append(errs, nodeErrs...)
		rt.Roots = append(rt.Roots, rn)
	}

	return rt, errs
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

	fields := make([]document.DataPoint, 0, len(c.Fields))
	for _, path := range c.Fields {
		dp, err := cs.Address(path)
		if err != nil {
			errs = append(errs, CompileError{
				Pass:    PassConstraints,
				Message: "constraint " + c.Name + ": cannot resolve field " + path + ": " + err.Error(),
			})
			continue
		}
		fields = append(fields, dp)
	}

	rc := ResolvedConstraint{
		Predicate:  pred,
		Fields:     fields,
		Parameters: c.Parameters,
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
	rg := ResolvedConstraintGroup{Operator: g.Operator}
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
