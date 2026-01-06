package json

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/asaidimu/go-anansi/v6/core/common"
)

const (
	MaxValidationDepth = 1000
	FloatEpsilon       = 1e-10
)

type Schema struct {
	raw map[string]any

	Type  any
	Enum  []any
	Const any

	MultipleOf       *float64
	Maximum          *float64
	ExclusiveMaximum *float64
	Minimum          *float64
	ExclusiveMinimum *float64

	MaxLength *int
	MinLength *int
	Pattern   *regexp.Regexp
	Format    string

	Items           any
	AdditionalItems any
	MaxItems        *int
	MinItems        *int
	UniqueItems     bool
	Contains        *Schema

	MaxProperties        *int
	MinProperties        *int
	Required             []string
	Properties           map[string]*Schema
	PatternProperties    map[*regexp.Regexp]*Schema
	AdditionalProperties any
	Dependencies         map[string]any
	PropertyNames        *Schema

	AllOf []*Schema
	AnyOf []*Schema
	OneOf []*Schema
	Not   *Schema

	If   *Schema
	Then *Schema
	Else *Schema

	Ref string

	// Metadata (Draft 7)
	Title       string
	Description string
	Default     any
	ReadOnly    bool
	WriteOnly   bool
	Examples    []any
}

type Compiler struct {
	rawSchema  map[string]any
	refSchemas map[string]*Schema
	mu         sync.RWMutex
}

func NewCompiler(schemaBytes []byte) (*Compiler, error) {
	var rawSchema map[string]any
	if err := json.Unmarshal(schemaBytes, &rawSchema); err != nil {
		return nil, fmt.Errorf("failed to parse schema JSON: %w", err)
	}

	compiler := &Compiler{
		rawSchema:  rawSchema,
		refSchemas: make(map[string]*Schema),
	}

	_, err := compiler.parseSchema(rawSchema, "#", make(map[string]bool))
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	return compiler, nil
}

func (c *Compiler) parseSchema(raw map[string]any, path string, visiting map[string]bool) (*Schema, error) {
	if visiting[path] {
		return nil, fmt.Errorf("circular reference detected at %s", path)
	}

	visiting[path] = true
	defer delete(visiting, path)

	schema := &Schema{raw: raw}

	c.mu.Lock()
	c.refSchemas[path] = schema
	c.mu.Unlock()

	if t, ok := raw["type"]; ok {
		schema.Type = t
	}

	if enum, ok := raw["enum"].([]any); ok {
		schema.Enum = make([]any, len(enum))
		copy(schema.Enum, enum)
	}

	if constVal, ok := raw["const"]; ok {
		schema.Const = constVal
	}

	if val, ok := getFloat(raw, "multipleOf"); ok {
		schema.MultipleOf = &val
	}
	if val, ok := getFloat(raw, "maximum"); ok {
		schema.Maximum = &val
	}
	if val, ok := getFloat(raw, "exclusiveMaximum"); ok {
		schema.ExclusiveMaximum = &val
	}
	if val, ok := getFloat(raw, "minimum"); ok {
		schema.Minimum = &val
	}
	if val, ok := getFloat(raw, "exclusiveMinimum"); ok {
		schema.ExclusiveMinimum = &val
	}

	if val, ok := getInt(raw, "maxLength"); ok {
		schema.MaxLength = &val
	}
	if val, ok := getInt(raw, "minLength"); ok {
		schema.MinLength = &val
	}
	if pattern, ok := raw["pattern"].(string); ok {
		normalized := normalizeUnicodeEscapes(pattern)
		re, err := regexp.Compile(normalized)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern at %s: %w", path, err)
		}
		schema.Pattern = re
	}
	if format, ok := raw["format"].(string); ok {
		schema.Format = format
	}

	if items, ok := raw["items"]; ok {
		if itemSchema, ok := items.(map[string]any); ok {
			parsed, err := c.parseSchema(itemSchema, path+"/items", visiting)
			if err != nil {
				return nil, err
			}
			schema.Items = parsed
		} else if itemsArray, ok := items.([]any); ok {
			schemas := make([]*Schema, len(itemsArray))
			for i, item := range itemsArray {
				itemMap, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("items array element must be schema object at %s", path)
				}
				parsed, err := c.parseSchema(itemMap, fmt.Sprintf("%s/items/%d", path, i), visiting)
				if err != nil {
					return nil, err
				}
				schemas[i] = parsed
			}
			schema.Items = schemas
		}
	}

	if additionalItems, ok := raw["additionalItems"]; ok {
		if b, ok := additionalItems.(bool); ok {
			schema.AdditionalItems = b
		} else if addItemSchema, ok := additionalItems.(map[string]any); ok {
			parsed, err := c.parseSchema(addItemSchema, path+"/additionalItems", visiting)
			if err != nil {
				return nil, err
			}
			schema.AdditionalItems = parsed
		}
	}

	if val, ok := getInt(raw, "maxItems"); ok {
		schema.MaxItems = &val
	}
	if val, ok := getInt(raw, "minItems"); ok {
		schema.MinItems = &val
	}
	if uniqueItems, ok := raw["uniqueItems"].(bool); ok {
		schema.UniqueItems = uniqueItems
	}

	if contains, ok := raw["contains"].(map[string]any); ok {
		parsed, err := c.parseSchema(contains, path+"/contains", visiting)
		if err != nil {
			return nil, err
		}
		schema.Contains = parsed
	}

	if val, ok := getInt(raw, "maxProperties"); ok {
		schema.MaxProperties = &val
	}
	if val, ok := getInt(raw, "minProperties"); ok {
		schema.MinProperties = &val
	}

	if required, ok := raw["required"].([]any); ok {
		schema.Required = make([]string, len(required))
		for i, r := range required {
			if s, ok := r.(string); ok {
				schema.Required[i] = s
			}
		}
	}

	if properties, ok := raw["properties"].(map[string]any); ok {
		schema.Properties = make(map[string]*Schema)
		for name, prop := range properties {
			propMap, ok := prop.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("property %s must be schema object at %s", name, path)
			}
			parsed, err := c.parseSchema(propMap, path+"/properties/"+name, visiting)
			if err != nil {
				return nil, err
			}
			schema.Properties[name] = parsed
		}
	}

	if patternProps, ok := raw["patternProperties"].(map[string]any); ok {
		schema.PatternProperties = make(map[*regexp.Regexp]*Schema)
		for pattern, prop := range patternProps {
			normalized := normalizeUnicodeEscapes(pattern)
			re, err := regexp.Compile(normalized)
			if err != nil {
				return nil, fmt.Errorf("invalid patternProperty regex '%s' at %s: %w", pattern, path, err)
			}

			propMap, ok := prop.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("patternProperty %s must be schema object at %s", pattern, path)
			}
			parsed, err := c.parseSchema(propMap, path+"/patternProperties/"+pattern, visiting)
			if err != nil {
				return nil, err
			}
			schema.PatternProperties[re] = parsed
		}
	}

	if additionalProps, ok := raw["additionalProperties"]; ok {
		if b, ok := additionalProps.(bool); ok {
			schema.AdditionalProperties = b
		} else if addPropSchema, ok := additionalProps.(map[string]any); ok {
			parsed, err := c.parseSchema(addPropSchema, path+"/additionalProperties", visiting)
			if err != nil {
				return nil, err
			}
			schema.AdditionalProperties = parsed
		}
	}

	if deps, ok := raw["dependencies"].(map[string]any); ok {
		schema.Dependencies = make(map[string]any)
		for name, dep := range deps {
			if depArray, ok := dep.([]any); ok {
				props := make([]string, len(depArray))
				for i, p := range depArray {
					if s, ok := p.(string); ok {
						props[i] = s
					}
				}
				schema.Dependencies[name] = props
			} else if depSchema, ok := dep.(map[string]any); ok {
				parsed, err := c.parseSchema(depSchema, path+"/dependencies/"+name, visiting)
				if err != nil {
					return nil, err
				}
				schema.Dependencies[name] = parsed
			}
		}
	}

	if propertyNames, ok := raw["propertyNames"].(map[string]any); ok {
		parsed, err := c.parseSchema(propertyNames, path+"/propertyNames", visiting)
		if err != nil {
			return nil, err
		}
		schema.PropertyNames = parsed
	}

	if allOf, ok := raw["allOf"].([]any); ok {
		schema.AllOf = make([]*Schema, len(allOf))
		for i, s := range allOf {
			sMap, ok := s.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("allOf element must be schema object at %s", path)
			}
			parsed, err := c.parseSchema(sMap, fmt.Sprintf("%s/allOf/%d", path, i), visiting)
			if err != nil {
				return nil, err
			}
			schema.AllOf[i] = parsed
		}
	}

	if anyOf, ok := raw["anyOf"].([]any); ok {
		schema.AnyOf = make([]*Schema, len(anyOf))
		for i, s := range anyOf {
			sMap, ok := s.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("anyOf element must be schema object at %s", path)
			}
			parsed, err := c.parseSchema(sMap, fmt.Sprintf("%s/anyOf/%d", path, i), visiting)
			if err != nil {
				return nil, err
			}
			schema.AnyOf[i] = parsed
		}
	}

	if oneOf, ok := raw["oneOf"].([]any); ok {
		schema.OneOf = make([]*Schema, len(oneOf))
		for i, s := range oneOf {
			sMap, ok := s.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("oneOf element must be schema object at %s", path)
			}
			parsed, err := c.parseSchema(sMap, fmt.Sprintf("%s/oneOf/%d", path, i), visiting)
			if err != nil {
				return nil, err
			}
			schema.OneOf[i] = parsed
		}
	}

	if not, ok := raw["not"].(map[string]any); ok {
		parsed, err := c.parseSchema(not, path+"/not", visiting)
		if err != nil {
			return nil, err
		}
		schema.Not = parsed
	}

	if ifSchema, ok := raw["if"].(map[string]any); ok {
		parsed, err := c.parseSchema(ifSchema, path+"/if", visiting)
		if err != nil {
			return nil, err
		}
		schema.If = parsed
	}

	if thenSchema, ok := raw["then"].(map[string]any); ok {
		parsed, err := c.parseSchema(thenSchema, path+"/then", visiting)
		if err != nil {
			return nil, err
		}
		schema.Then = parsed
	}

	if elseSchema, ok := raw["else"].(map[string]any); ok {
		parsed, err := c.parseSchema(elseSchema, path+"/else", visiting)
		if err != nil {
			return nil, err
		}
		schema.Else = parsed
	}

	if ref, ok := raw["$ref"].(string); ok {
		schema.Ref = ref
	}

	for _, key := range []string{"definitions", "$defs"} {
		if defs, ok := raw[key].(map[string]any); ok {
			for name, def := range defs {
				defMap, ok := def.(map[string]any)
				if !ok {
					continue
				}
				_, err := c.parseSchema(defMap, path+"/"+key+"/"+name, visiting)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	if title, ok := raw["title"].(string); ok {
		schema.Title = title
	}
	if desc, ok := raw["description"].(string); ok {
		schema.Description = desc
	}
	if def, ok := raw["default"]; ok {
		schema.Default = def
	}
	if ro, ok := raw["readOnly"].(bool); ok {
		schema.ReadOnly = ro
	}
	if wo, ok := raw["writeOnly"].(bool); ok {
		schema.WriteOnly = wo
	}
	if examples, ok := raw["examples"].([]any); ok {
		schema.Examples = examples
	}

	return schema, nil
}

func (c *Compiler) getSchemaByRef(ref string) (*Schema, error) {
    if !strings.HasPrefix(ref, "#") {
        return nil, fmt.Errorf("external $ref not supported") //
    }

    // Attempt dynamic resolution
    return c.resolvePointer(ref)
}

func (c *Compiler) Validate(data []byte) error {
	var instance any
	if err := json.Unmarshal(data, &instance); err != nil {
		return fmt.Errorf("failed to parse JSON data: %w", err)
	}
	return c.ValidateValue(instance)
}

func (c *Compiler) Schema(ref string) (*Schema, bool) {
	sc, ok := c.refSchemas[ref]
	return sc, ok
}

func (c *Compiler) ValidateValue(instance any) error {
	if err := c.checkInstanceDepth(instance, 0, "#"); err != nil {
		return &ValidationError{
			Issues: []common.Issue{{
				Code:    "MAX_DEPTH_EXCEEDED",
				Message: err.Error(),
				Path:    "#",
			}},
		}
	}

	ctx := &validationContext{
		compiler: c,
		issues:   []common.Issue{},
		depth:    0,
		visited:  make(map[validationKey]bool),
	}

	root := c.refSchemas["#"]
	if root == nil {
		return fmt.Errorf("root schema not found")
	}

	c.validateSchema(ctx, root, instance, "")

	if len(ctx.issues) > 0 {
		return &ValidationError{Issues: ctx.issues}
	}
	return nil
}

type validationKey struct {
	schema *Schema
	path   string
}

type validationContext struct {
	compiler *Compiler
	issues   []common.Issue
	depth    int
	visited  map[validationKey]bool
}

type ValidationError struct {
	Issues []common.Issue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 1 {
		return fmt.Sprintf("validation failed at %s: %s", e.Issues[0].Path, e.Issues[0].Message)
	}
	return fmt.Sprintf("validation failed with %d error(s)", len(e.Issues))
}

func (ctx *validationContext) addIssue(code, message, path, description string) {
	ctx.issues = append(ctx.issues, common.Issue{
		Code:        code,
		Message:     message,
		Path:        path,
		Severity:    "error",
		Description: description,
	})
}

func (c *Compiler) validateSchema(ctx *validationContext, schema *Schema, instance any, path string) {
	// Count every entry into a schema
	ctx.depth++
	defer func() { ctx.depth-- }()

	if ctx.depth > MaxValidationDepth {
		ctx.addIssue("MAX_DEPTH_EXCEEDED", "maximum validation depth exceeded", path, "")
		return
	}

	key := validationKey{schema: schema, path: path}
	if ctx.visited[key] {
		return
	}
	ctx.visited[key] = true
	defer delete(ctx.visited, key)

	if schema == nil {
		return
	}

	if schema.Ref != "" {
		refSchema, err := c.getSchemaByRef(schema.Ref)
		if err != nil {
			ctx.addIssue("INVALID_REF", fmt.Sprintf("unresolvable reference: %s", schema.Ref), path, err.Error())
			return
		}

		c.validateSchema(ctx, refSchema, instance, path)
		return
	}

	if schema.Const != nil {
		if !deepEqual(instance, schema.Const) {
			ctx.addIssue("INVALID_CONST", fmt.Sprintf("must equal constant value: %v", schema.Const), path, "")
		}
	}

	if len(schema.Enum) > 0 {
		valid := false
		for _, ev := range schema.Enum {
			if deepEqual(instance, ev) {
				valid = true
				break
			}
		}
		if !valid {
			ctx.addIssue("INVALID_ENUM", fmt.Sprintf("must be one of: %v", schema.Enum), path, "")
		}
	}

	if schema.Type != nil {
		c.validateType(ctx, schema.Type, instance, path)
	}

	switch v := instance.(type) {
	case float64:
		c.validateNumber(ctx, schema, v, path)
	case string:
		c.validateString(ctx, schema, v, path)
	case []any:
		c.validateArray(ctx, schema, v, path)
	case map[string]any:
		c.validateObject(ctx, schema, v, path)
	}

	if len(schema.AllOf) > 0 {
		for _, sub := range schema.AllOf {
			c.validateSchema(ctx, sub, instance, path)
		}
	}

	if len(schema.AnyOf) > 0 {
		valid := false
		for _, sub := range schema.AnyOf {
			subCtx := &validationContext{
				compiler: ctx.compiler,
				issues:   []common.Issue{},
				depth:    ctx.depth,
				visited:  ctx.visited,
			}
			c.validateSchema(subCtx, sub, instance, path)
			if len(subCtx.issues) == 0 {
				valid = true
				break
			}
		}
		if !valid {
			ctx.addIssue("INVALID_ANY_OF", "must match at least one schema in anyOf", path, "")
		}
	}

	if len(schema.OneOf) > 0 {
		validCount := 0
		for _, sub := range schema.OneOf {
			subCtx := &validationContext{
				compiler: ctx.compiler,
				issues:   []common.Issue{},
				depth:    ctx.depth,
				visited:  ctx.visited,
			}
			c.validateSchema(subCtx, sub, instance, path)
			if len(subCtx.issues) == 0 {
				validCount++
			}
		}
		if validCount != 1 {
			ctx.addIssue("INVALID_ONE_OF", fmt.Sprintf("must match exactly one schema in oneOf, but matched %d", validCount), path, "")
		}
	}

	if schema.Not != nil {
		subCtx := &validationContext{
			compiler: ctx.compiler,
			issues:   []common.Issue{},
			depth:    ctx.depth,
			visited:  ctx.visited,
		}
		c.validateSchema(subCtx, schema.Not, instance, path)
		if len(subCtx.issues) == 0 {
			ctx.addIssue("INVALID_NOT", "must not match the schema in not", path, "")
		}
	}

	if schema.If != nil {
		subCtx := &validationContext{
			compiler: ctx.compiler,
			issues:   []common.Issue{},
			depth:    ctx.depth,
			visited:  ctx.visited,
		}
		c.validateSchema(subCtx, schema.If, instance, path)
		if len(subCtx.issues) == 0 {
			if schema.Then != nil {
				c.validateSchema(ctx, schema.Then, instance, path)
			}
		} else {
			if schema.Else != nil {
				c.validateSchema(ctx, schema.Else, instance, path)
			}
		}
	}
}

func (c *Compiler) validateType(ctx *validationContext, schemaType any, instance any, path string) {
	var types []string
	switch t := schemaType.(type) {
	case string:
		types = []string{t}
	case []any:
		for _, typ := range t {
			if s, ok := typ.(string); ok {
				types = append(types, s)
			}
		}
	default:
		return
	}

	instanceType := getInstanceType(instance)

	matched := false
	for _, typ := range types {
		if typ == instanceType {
			matched = true
			break
		}
		if (typ == "integer" && instanceType == "number") || (typ == "number" && instanceType == "integer") {
			if num, ok := instance.(float64); ok && isInteger(num) {
				matched = true
				break
			}
		}
	}

	if !matched {
		ctx.addIssue("INVALID_TYPE", fmt.Sprintf("expected type %v, got %s", types, instanceType), path, "")
	}
}

func (c *Compiler) validateNumber(ctx *validationContext, schema *Schema, value float64, path string) {
	if schema.MultipleOf != nil {
		if !isMultipleOf(value, *schema.MultipleOf) {
			ctx.addIssue("INVALID_MULTIPLE_OF", fmt.Sprintf("must be multiple of %v", *schema.MultipleOf), path, "")
		}
	}

	if schema.Maximum != nil && value > *schema.Maximum {
		ctx.addIssue("VALUE_TOO_LARGE", fmt.Sprintf("must be at most %v", *schema.Maximum), path, "")
	}

	if schema.ExclusiveMaximum != nil && value >= *schema.ExclusiveMaximum {
		ctx.addIssue("VALUE_TOO_LARGE", fmt.Sprintf("must be less than %v", *schema.ExclusiveMaximum), path, "")
	}

	if schema.Minimum != nil && value < *schema.Minimum {
		ctx.addIssue("VALUE_TOO_SMALL", fmt.Sprintf("must be at least %v", *schema.Minimum), path, "")
	}

	if schema.ExclusiveMinimum != nil && value <= *schema.ExclusiveMinimum {
		ctx.addIssue("VALUE_TOO_SMALL", fmt.Sprintf("must be greater than %v", *schema.ExclusiveMinimum), path, "")
	}
}

func (c *Compiler) validateString(ctx *validationContext, schema *Schema, value string, path string) {
	length := utf8.RuneCountInString(value)

	if schema.MaxLength != nil && length > *schema.MaxLength {
		ctx.addIssue("VALUE_TOO_LONG", fmt.Sprintf("must be at most %d characters", *schema.MaxLength), path, "")
	}

	if schema.MinLength != nil && length < *schema.MinLength {
		ctx.addIssue("VALUE_TOO_SHORT", fmt.Sprintf("must be at least %d characters", *schema.MinLength), path, "")
	}

	if schema.Pattern != nil && !schema.Pattern.MatchString(value) {
		ctx.addIssue("INVALID_PATTERN", fmt.Sprintf("must match pattern: %s", schema.Pattern.String()), path, "")
	}

	if schema.Format != "" {
		if err := validateFormat(schema.Format, value); err != nil {
			ctx.addIssue("INVALID_FORMAT", fmt.Sprintf("must be valid %s format", schema.Format), path, err.Error())
		}
	}
}

func (c *Compiler) validateArray(ctx *validationContext, schema *Schema, value []any, path string) {
	length := len(value)

	if schema.MaxItems != nil && length > *schema.MaxItems {
		ctx.addIssue("TOO_MANY_ITEMS", fmt.Sprintf("must have at most %d items", *schema.MaxItems), path, "")
	}

	if schema.MinItems != nil && length < *schema.MinItems {
		ctx.addIssue("TOO_FEW_ITEMS", fmt.Sprintf("must have at least %d items", *schema.MinItems), path, "")
	}

	if schema.UniqueItems && !areItemsUnique(value) {
		ctx.addIssue("DUPLICATE_ITEMS", "items must be unique", path, "")
	}

	if schema.Items != nil {
		switch items := schema.Items.(type) {
		case *Schema:
			for i, item := range value {
				c.validateSchema(ctx, items, item, fmt.Sprintf("%s/%d", path, i))
			}
		case []*Schema:
			for i, item := range value {
				if i < len(items) {
					c.validateSchema(ctx, items[i], item, fmt.Sprintf("%s/%d", path, i))
				} else if schema.AdditionalItems != nil {
					switch add := schema.AdditionalItems.(type) {
					case bool:
						if !add {
							ctx.addIssue("ADDITIONAL_ITEMS_NOT_ALLOWED", "additional items are not allowed", fmt.Sprintf("%s/%d", path, i), "")
						}
					case *Schema:
						c.validateSchema(ctx, add, item, fmt.Sprintf("%s/%d", path, i))
					}
				}
			}
		}
	}

	if schema.Contains != nil {
		found := false
		for _, item := range value {
			subCtx := &validationContext{
				compiler: ctx.compiler,
				issues:   []common.Issue{},
				depth:    ctx.depth,
				visited:  ctx.visited,
			}
			c.validateSchema(subCtx, schema.Contains, item, path)
			if len(subCtx.issues) == 0 {
				found = true
				break
			}
		}
		if !found {
			ctx.addIssue("NO_MATCHING_CONTAINS", "array must contain at least one item matching the contains schema", path, "")
		}
	}
}

func (c *Compiler) validateObject(ctx *validationContext, schema *Schema, value map[string]any, path string) {
	count := len(value)

	if schema.MaxProperties != nil && count > *schema.MaxProperties {
		ctx.addIssue("TOO_MANY_PROPERTIES", fmt.Sprintf("must have at most %d properties", *schema.MaxProperties), path, "")
	}

	if schema.MinProperties != nil && count < *schema.MinProperties {
		ctx.addIssue("TOO_FEW_PROPERTIES", fmt.Sprintf("must have at least %d properties", *schema.MinProperties), path, "")
	}

	for _, req := range schema.Required {
		if _, exists := value[req]; !exists {
			ctx.addIssue("REQUIRED_PROPERTY_MISSING", fmt.Sprintf("required property '%s' is missing", req), path+"/"+req, "")
		}
	}

	evaluated := make(map[string]bool)

	if schema.Properties != nil {
		for name, propSchema := range schema.Properties {
			if val, ok := value[name]; ok {
				c.validateSchema(ctx, propSchema, val, path+"/"+name)
				evaluated[name] = true
			}
		}
	}

	if schema.PatternProperties != nil {
		for re, propSchema := range schema.PatternProperties {
			for name, val := range value {
				if re.MatchString(name) {
					c.validateSchema(ctx, propSchema, val, path+"/"+name)
					evaluated[name] = true
				}
			}
		}
	}

	if schema.AdditionalProperties != nil {
		for name, val := range value {
			if !evaluated[name] {
				switch add := schema.AdditionalProperties.(type) {
				case bool:
					if !add {
						ctx.addIssue("ADDITIONAL_PROPERTY_NOT_ALLOWED", fmt.Sprintf("additional property '%s' is not allowed", name), path+"/"+name, "")
					}
				case *Schema:
					c.validateSchema(ctx, add, val, path+"/"+name)
				}
			}
		}
	}

	if schema.PropertyNames != nil {
		for name := range value {
			c.validateSchema(ctx, schema.PropertyNames, name, path+"/"+name)
		}
	}

	if schema.Dependencies != nil {
		for depName, dep := range schema.Dependencies {
			if _, present := value[depName]; present {
				switch d := dep.(type) {
				case []string:
					for _, required := range d {
						if _, ok := value[required]; !ok {
							ctx.addIssue("DEPENDENCY_PROPERTY_MISSING", fmt.Sprintf("property '%s' is required when '%s' is present", required, depName), path+"/"+required, "")
						}
					}
				case *Schema:
					c.validateSchema(ctx, d, value, path)
				}
			}
		}
	}
}

func (c *Compiler) resolvePointer(ptr string) (*Schema, error) {
    // 1. Check if we already parsed this exact path
    c.mu.RLock()
    if s, ok := c.refSchemas[ptr]; ok {
        c.mu.RUnlock()
        return s, nil
    }
    c.mu.RUnlock()

    // 2. If not found, manually traverse the raw map
    parts := strings.Split(strings.TrimPrefix(ptr, "#/"), "/")
    var current any = c.rawSchema

    for _, part := range parts {
        if part == "" {
            continue
        }
        // Decode JSON Pointer escapes: ~1 -> / and ~0 -> ~
        part = strings.ReplaceAll(part, "~1", "/")
        part = strings.ReplaceAll(part, "~0", "~")

        switch v := current.(type) {
        case map[string]any:
            next, ok := v[part]
            if !ok {
                return nil, fmt.Errorf("pointer error: key '%s' not found", part)
            }
            current = next
        case []any:
            idx, err := strconv.Atoi(part)
            if err != nil || idx < 0 || idx >= len(v) {
                return nil, fmt.Errorf("pointer error: invalid array index '%s'", part)
            }
            current = v[idx]
        default:
            return nil, fmt.Errorf("pointer error: cannot traverse into %T", v)
        }
    }

    // 3. Convert the found raw fragment into a Schema
    rawMap, ok := current.(map[string]any)
    if !ok {
        return nil, fmt.Errorf("pointer did not lead to a schema object")
    }

    return c.parseSchema(rawMap, ptr, make(map[string]bool))
}

func (c *Compiler) checkInstanceDepth(instance any, depth int, path string) error {
	if depth > MaxValidationDepth {
		return fmt.Errorf("maximum validation depth exceeded at %s", path)
	}

	switch v := instance.(type) {
	case map[string]any:
		for key, val := range v {
			// Construct path for reporting
			newPath := path + "/" + key
			if err := c.checkInstanceDepth(val, depth+1, newPath); err != nil {
				return err
			}
		}
	case []any:
		for i, val := range v {
			// Construct path for reporting
			newPath := path + "/" + strconv.Itoa(i)
			if err := c.checkInstanceDepth(val, depth+1, newPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func getFloat(m map[string]any, key string) (float64, bool) {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return f, true
		}
	}
	return 0, false
}

func getInt(m map[string]any, key string) (int, bool) {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			if f == math.Trunc(f) {
				return int(f), true
			}
		}
	}
	return 0, false
}

func getInstanceType(instance any) string {
	if instance == nil {
		return "null"
	}
	switch v := instance.(type) {
	case bool:
		return "boolean"
	case float64:
		if isInteger(v) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func isInteger(v float64) bool {
	return math.Floor(v) == v && !math.IsInf(v, 0) && !math.IsNaN(v)
}

func isMultipleOf(value, divisor float64) bool {
	if divisor == 0 {
		return false
	}
	if math.Abs(value) < 1e10 && math.Abs(divisor) < 1e10 {
		q := value / divisor
		return math.Abs(q-math.Round(q)) < FloatEpsilon
	}
	v := big.NewFloat(value)
	d := big.NewFloat(divisor)
	q := new(big.Float).Quo(v, d)
	i, _ := q.Int(nil)
	diff := new(big.Float).Sub(q, new(big.Float).SetInt(i))
	f, _ := diff.Float64()
	return math.Abs(f) < FloatEpsilon
}

func deepEqual(a, b any) bool {
	return reflect.DeepEqual(a, b)
}

func areItemsUnique(items []any) bool {
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if reflect.DeepEqual(items[i], items[j]) {
				return false
			}
		}
	}
	return true
}

func validateFormat(format, value string) error {
	switch format {
	case "date-time":
		return validateDateTime(value)
	case "date":
		return validateDate(value)
	case "time":
		return validateTime(value)
	case "email", "idn-email":
		return validateEmailStrict(value)
	case "hostname", "idn-hostname":
		return validateHostname(value)
	case "ipv4":
		return validateIPv4(value)
	case "ipv6":
		return validateIPv6(value)
	case "uri", "iri":
		return validateURI(value)
	case "uri-reference", "iri-reference":
		return validateURIReference(value)
	case "uri-template":
		return validateURITemplate(value)
	case "json-pointer":
		return validateJSONPointer(value)
	case "relative-json-pointer":
		return validateRelativeJSONPointer(value)
	case "regex":
		return validateRegex(value)
	default:
		return nil
	}
}

func validateDateTime(value string) error {
	_, err := time.Parse(time.RFC3339, value)
	if err != nil {
		_, err = time.Parse(time.RFC3339Nano, value)
	}
	return err
}

func validateDate(value string) error {
	_, err := time.Parse("2006-01-02", value)
	return err
}

func validateTime(value string) error {
	formats := []string{"15:04:05", "15:04:05.999999999", "15:04:05Z07:00", "15:04:05.999999999Z07:00"}
	for _, f := range formats {
		if _, err := time.Parse(f, value); err == nil {
			return nil
		}
	}
	return fmt.Errorf("invalid time format")
}

func validateEmailStrict(value string) error {
	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value {
		return fmt.Errorf("invalid email")
	}
	return nil
}

var hostnameRegex = regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)

func validateHostname(value string) error {
	v := strings.TrimSuffix(value, ".")
	if len(v) == 0 || len(v) > 253 || !hostnameRegex.MatchString(v) {
		return fmt.Errorf("invalid hostname")
	}
	for label := range strings.SplitSeq(v, ".") {
		if len(label) > 63 {
			return fmt.Errorf("hostname label too long")
		}
	}
	return nil
}

func validateIPv4(value string) error {
	if ip := net.ParseIP(value); ip == nil || ip.To4() == nil {
		return fmt.Errorf("invalid IPv4")
	}
	return nil
}

func validateIPv6(value string) error {
	if ip := net.ParseIP(value); ip == nil || ip.To4() != nil {
		return fmt.Errorf("invalid IPv6")
	}
	return nil
}

func validateURI(value string) error {
	u, err := url.ParseRequestURI(value)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("invalid URI")
	}
	return nil
}

func validateURIReference(value string) error {
	_, err := url.Parse(value)
	return err
}

func validateURITemplate(value string) error {
	depth := 0
	for i, r := range value {
		switch r {
		case '{':
			depth++
			if depth > 1 {
				return fmt.Errorf("nested braces at %d", i)
			}
		case '}':
			depth--
			if depth < 0 {
				return fmt.Errorf("unmatched } at %d", i)
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unmatched {")
	}
	return nil
}

func validateJSONPointer(value string) error {
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("must start with /")
	}
	parts := strings.SplitSeq(value[1:], "/")
	for p := range parts {
		i := 0
		for i < len(p) {
			if p[i] == '~' {
				if i+1 >= len(p) || (p[i+1] != '0' && p[i+1] != '1') {
					return fmt.Errorf("invalid ~ escape")
				}
				i += 2
			} else {
				i++
			}
		}
	}
	return nil
}

func validateRelativeJSONPointer(value string) error {
	if len(value) == 0 {
		return fmt.Errorf("empty")
	}
	i := 0
	for i < len(value) && value[i] >= '0' && value[i] <= '9' {
		i++
	}
	if i == 0 {
		return fmt.Errorf("must start with number")
	}
	if i == len(value) || value[i] == '#' {
		return nil
	}
	return validateJSONPointer(value[i:])
}

func validateRegex(value string) error {
	_, err := regexp.Compile(value)
	return err
}

func normalizeUnicodeEscapes(p string) string {
	re := regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)
	return re.ReplaceAllStringFunc(p, func(m string) string {
		r, _ := strconv.ParseInt(m[2:], 16, 32)
		return string(rune(r))
	})
}
