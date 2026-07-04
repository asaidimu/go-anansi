package schema

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
	"github.com/asaidimu/go-anansi/v7/core/schema/meta"
)

// --- Types ---

type Schema = definition.Schema
type BaseSchema = definition.BaseSchema
type NestedSchema = definition.NestedSchema

type Field = definition.Field
type FieldId = definition.FieldId
type FieldName = definition.FieldName
type FieldProperties = definition.FieldProperties
type FieldType = definition.FieldType

type Index = definition.Index
type IndexId = definition.IndexId
type IndexType = definition.IndexType

type Constraint = definition.Constraint
type ConstraintId = definition.ConstraintId
type Predicate = definition.Predicate
type PredicateMap = definition.PredicateMap
type PredicateName = definition.PredicateName
type PredicateParams = definition.PredicateParams

type LiteralValue = definition.LiteralValue
type LiteralType = definition.LiteralType

type SchemaId = definition.SchemaId
type SchemaReference = definition.SchemaReference
type FieldSchemaReference = definition.FieldSchemaReference

type DocumentValidator = definition.DocumentValidator
type ValidationConfig = definition.ValidationConfig

type CompiledSchema = definition.CompiledSchema
type FieldDescriptor = definition.FieldDescriptor
type FieldKind = definition.FieldKind
type FieldMeta = definition.FieldMeta
type SchemaMeta = definition.SchemaMeta

// --- Constants ---

const (
	FieldTypeUnknown   = definition.FieldTypeUnknown
	FieldTypeString    = definition.FieldTypeString
	FieldTypeNumber    = definition.FieldTypeNumber
	FieldTypeInteger   = definition.FieldTypeInteger
	FieldTypeDecimal   = definition.FieldTypeDecimal
	FieldTypeBoolean   = definition.FieldTypeBoolean
	FieldTypeArray     = definition.FieldTypeArray

	FieldTypeEnum      = definition.FieldTypeEnum
	FieldTypeObject    = definition.FieldTypeObject
	FieldTypeRecord    = definition.FieldTypeRecord
	FieldTypeUnion     = definition.FieldTypeUnion
	FieldTypeComposite = definition.FieldTypeComposite
	FieldTypeGeometry  = definition.FieldTypeGeometry
	FieldTypeBytes     = definition.FieldTypeBytes

	IndexTypeNormal   = definition.IndexTypeNormal
	IndexTypeUnique   = definition.IndexTypeUnique
	IndexTypePrimary  = definition.IndexTypePrimary
	IndexTypeSpatial  = definition.IndexTypeSpatial
	IndexTypeFullText = definition.IndexTypeFullText

	LiteralTypeString  = definition.LiteralTypeString
	LiteralTypeInteger = definition.LiteralTypeInteger
	LiteralTypeFloat   = definition.LiteralTypeFloat
	LiteralTypeBoolean = definition.LiteralTypeBoolean
	LiteralTypeObject  = definition.LiteralTypeObject
	LiteralTypeArray   = definition.LiteralTypeArray
	LiteralTypeNull    = definition.LiteralTypeNull
)

// --- Variables & Meta ---

var (
	MetaSchema           = meta.MetaSchema
	MetaSchemaPredicates = meta.MetaSchemaPredicates
)

// --- Functions ---

func NewDocumentValidator(sc *Schema, fmap PredicateMap) (*DocumentValidator, error) {
	return definition.NewDocumentValidator(sc, fmap)
}

func NewDocumentValidatorWithConfig(sc *Schema, fmap PredicateMap, config ValidationConfig) (*DocumentValidator, error) {
	return definition.NewDocumentValidatorWithConfig(sc, fmap, config)
}

func NewSchemaReference[T definition.SchemaReferenceType](payload T) FieldSchemaReference {
	return definition.NewSchemaReference(payload)
}

func FieldSchemaAs[T definition.SchemaReferenceType](fr FieldSchemaReference) (T, error) {
	return definition.FieldSchemaAs[T](fr)
}

func NewLiteralValue[T definition.LiteralValueType](val T) (LiteralValue, error) {
	return definition.NewLiteralValue(val)
}

func MustNewLiteralValue[T definition.LiteralValueType](val T) LiteralValue {
	return definition.MustNewLiteralValue(val)
}

func NewNullLiteral() LiteralValue {
	return definition.NewNullLiteral()
}

func LiteralValueAs[T definition.LiteralValueType](lv LiteralValue) (T, error) {
	return definition.LiteralValueAs[T](lv)
}

var (
	productionValidatorOnce sync.Once
	productionValidator     *DocumentValidator
	devValidatorOnce       sync.Once
	devValidator           *DocumentValidator
)

func SchemaValidator() *DocumentValidator {
	if os.Getenv("ANANSI_ENV") == "development" {
		devValidatorOnce.Do(func() {
			devValidator = meta.DevelopmentSchemaValidator()
		})
		return devValidator
	}
	productionValidatorOnce.Do(func() {
		var err error
		productionValidator, err = NewDocumentValidator(&meta.MetaSchema, meta.MetaSchemaPredicates)
		if err != nil {
			panic(err)
		}
	})
	return productionValidator
}

// ValidateSchema validates any schema and returns validation issues and a boolean
func ValidateSchema(sc *Schema) ([]common.Issue, error) {
	docMap := sc.AsMap()

	if issues, ok := SchemaValidator().Validate(docMap); !ok {
		return issues, common.NewSystemError("SCHEMA_VALIDATION_FAILED").WithIssues(issues)
	}
	return sentinelIssues, nil
}

var sentinelIssues []common.Issue = []common.Issue{}

func ValidateSchemaJson(sc []byte) ([]common.Issue, error) {
	var schemaData map[string]any
	err := json.Unmarshal(sc, &schemaData)

	if err != nil {
		return nil, err
	}

	if issues, ok := SchemaValidator().Validate(schemaData); !ok {
		return issues, common.NewSystemError("SCHEMA_VALIDATION_FAILED").WithIssues(issues)
	}
	return sentinelIssues, nil
}
