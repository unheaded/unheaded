// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package merge

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Validator is the interface for configuration validators.
type Validator interface {
	Validate(data map[string]any) error
}

// ValidatorFunc is a function adapter for Validator.
type ValidatorFunc func(map[string]any) error

// Validate implements Validator.
func (f ValidatorFunc) Validate(data map[string]any) error {
	return f(data)
}

// SchemaValidator validates configuration against a schema.
type SchemaValidator struct {
	schema *Schema
}

// Schema defines the expected structure of configuration.
type Schema struct {
	Fields   map[string]*FieldSchema
	Required []string
	Custom   []CustomRule
}

// FieldSchema defines the schema for a single field.
type FieldSchema struct {
	Type        FieldType
	Required    bool
	Default     any
	Min         *float64
	Max         *float64
	MinLength   *int
	MaxLength   *int
	Pattern     string
	Enum        []any
	Nested      *Schema
	ElementType FieldType
	Custom      func(any) error
	Description string
}

// FieldType represents the type of a field.
type FieldType int

const (
	// TypeAny accepts any type.
	TypeAny FieldType = iota
	// TypeString accepts string values.
	TypeString
	// TypeInt accepts integer values.
	TypeInt
	// TypeFloat accepts float values.
	TypeFloat
	// TypeBool accepts boolean values.
	TypeBool
	// TypeMap accepts map values.
	TypeMap
	// TypeArray accepts array values.
	TypeArray
	// TypeDuration accepts duration values.
	TypeDuration
)

// String returns a string representation of the field type.
func (ft FieldType) String() string {
	switch ft {
	case TypeAny:
		return "any"
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypeMap:
		return "map"
	case TypeArray:
		return "array"
	case TypeDuration:
		return "duration"
	default:
		return "unknown"
	}
}

// CustomRule is a custom validation rule.
type CustomRule struct {
	Name     string
	Validate func(map[string]any) error
}

// NewSchemaValidator creates a new schema validator.
func NewSchemaValidator(schema *Schema) *SchemaValidator {
	return &SchemaValidator{schema: schema}
}

// Validate validates the configuration against the schema.
func (v *SchemaValidator) Validate(data map[string]any) error {
	return v.validateSchema(data, v.schema, "")
}

// validateSchema validates data against a schema.
func (v *SchemaValidator) validateSchema(data map[string]any, schema *Schema, prefix string) error {
	// Check required fields
	for _, required := range schema.Required {
		fullKey := required
		if prefix != "" {
			fullKey = prefix + "." + required
		}

		if _, exists := data[required]; !exists {
			return &ValidationError{
				Field:   fullKey,
				Message: "required field is missing",
			}
		}
	}

	// Validate each field
	for name, fieldSchema := range schema.Fields {
		fullKey := name
		if prefix != "" {
			fullKey = prefix + "." + name
		}

		value, exists := data[name]
		if !exists {
			if fieldSchema.Required {
				return &ValidationError{
					Field:   fullKey,
					Message: "required field is missing",
				}
			}
			continue
		}

		if err := v.validateField(value, fieldSchema, fullKey); err != nil {
			return err
		}
	}

	// Run custom rules
	for _, rule := range schema.Custom {
		if err := rule.Validate(data); err != nil {
			return &ValidationError{
				Field:   prefix,
				Message: fmt.Sprintf("custom rule '%s' failed: %v", rule.Name, err),
			}
		}
	}

	return nil
}

// validateField validates a single field.
func (v *SchemaValidator) validateField(value any, schema *FieldSchema, key string) error {
	// Type validation
	if err := v.validateType(value, schema.Type, key); err != nil {
		return err
	}

	// Type-specific validation
	switch schema.Type {
	case TypeString:
		return v.validateString(value, schema, key)
	case TypeInt:
		return v.validateInt(value, schema, key)
	case TypeFloat:
		return v.validateFloat(value, schema, key)
	case TypeArray:
		return v.validateArray(value, schema, key)
	case TypeMap:
		return v.validateMap(value, schema, key)
	}

	// Enum validation
	if len(schema.Enum) > 0 {
		if err := v.validateEnum(value, schema.Enum, key); err != nil {
			return err
		}
	}

	// Custom validation
	if schema.Custom != nil {
		if err := schema.Custom(value); err != nil {
			return &ValidationError{
				Field:   key,
				Message: err.Error(),
			}
		}
	}

	return nil
}

// validateType validates the type of a value.
func (v *SchemaValidator) validateType(value any, expected FieldType, key string) error {
	if expected == TypeAny {
		return nil
	}

	var valid bool
	switch expected {
	case TypeString:
		_, valid = value.(string)
	case TypeInt:
		switch value.(type) {
		case int, int64, int32, float64:
			valid = true
		}
	case TypeFloat:
		switch value.(type) {
		case float64, float32, int, int64:
			valid = true
		}
	case TypeBool:
		_, valid = value.(bool)
	case TypeMap:
		_, valid = value.(map[string]any)
	case TypeArray:
		_, valid = value.([]any)
	case TypeDuration:
		_, valid = value.(string)
	default:
		valid = true
	}

	if !valid {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("expected type %s, got %T", expected, value),
		}
	}

	return nil
}

// validateString validates a string value.
func (v *SchemaValidator) validateString(value any, schema *FieldSchema, key string) error {
	str, ok := value.(string)
	if !ok {
		return nil
	}

	// Length validation
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("length must be at least %d", *schema.MinLength),
		}
	}

	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("length must be at most %d", *schema.MaxLength),
		}
	}

	// Pattern validation
	if schema.Pattern != "" {
		matched, err := regexp.MatchString(schema.Pattern, str)
		if err != nil {
			return &ValidationError{
				Field:   key,
				Message: fmt.Sprintf("invalid pattern: %v", err),
			}
		}
		if !matched {
			return &ValidationError{
				Field:   key,
				Message: fmt.Sprintf("value does not match pattern: %s", schema.Pattern),
			}
		}
	}

	return nil
}

// validateInt validates an integer value.
func (v *SchemaValidator) validateInt(value any, schema *FieldSchema, key string) error {
	var num float64
	switch n := value.(type) {
	case int:
		num = float64(n)
	case int64:
		num = float64(n)
	case float64:
		num = n
	default:
		return nil
	}

	if schema.Min != nil && num < *schema.Min {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("value must be at least %v", *schema.Min),
		}
	}

	if schema.Max != nil && num > *schema.Max {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("value must be at most %v", *schema.Max),
		}
	}

	return nil
}

// validateFloat validates a float value.
func (v *SchemaValidator) validateFloat(value any, schema *FieldSchema, key string) error {
	var num float64
	switch n := value.(type) {
	case float64:
		num = n
	case float32:
		num = float64(n)
	case int:
		num = float64(n)
	case int64:
		num = float64(n)
	default:
		return nil
	}

	if schema.Min != nil && num < *schema.Min {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("value must be at least %v", *schema.Min),
		}
	}

	if schema.Max != nil && num > *schema.Max {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("value must be at most %v", *schema.Max),
		}
	}

	return nil
}

// validateArray validates an array value.
func (v *SchemaValidator) validateArray(value any, schema *FieldSchema, key string) error {
	arr, ok := value.([]any)
	if !ok {
		return nil
	}

	// Length validation
	if schema.MinLength != nil && len(arr) < *schema.MinLength {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("array must have at least %d elements", *schema.MinLength),
		}
	}

	if schema.MaxLength != nil && len(arr) > *schema.MaxLength {
		return &ValidationError{
			Field:   key,
			Message: fmt.Sprintf("array must have at most %d elements", *schema.MaxLength),
		}
	}

	// Element type validation
	if schema.ElementType != TypeAny {
		for i, elem := range arr {
			elemKey := fmt.Sprintf("%s[%d]", key, i)
			if err := v.validateType(elem, schema.ElementType, elemKey); err != nil {
				return err
			}
		}
	}

	// Nested schema validation
	if schema.Nested != nil {
		for i, elem := range arr {
			elemKey := fmt.Sprintf("%s[%d]", key, i)
			if elemMap, ok := elem.(map[string]any); ok {
				if err := v.validateSchema(elemMap, schema.Nested, elemKey); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// validateMap validates a map value.
func (v *SchemaValidator) validateMap(value any, schema *FieldSchema, key string) error {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	// Nested schema validation
	if schema.Nested != nil {
		if err := v.validateSchema(m, schema.Nested, key); err != nil {
			return err
		}
	}

	return nil
}

// validateEnum validates a value against an enum.
func (v *SchemaValidator) validateEnum(value any, enum []any, key string) error {
	for _, allowed := range enum {
		if value == allowed {
			return nil
		}
	}

	return &ValidationError{
		Field:   key,
		Message: fmt.Sprintf("value must be one of: %v", enum),
	}
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements error.
func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors collects multiple validation errors.
type ValidationErrors struct {
	Errors []*ValidationError
}

// Error implements error.
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}

	messages := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		messages[i] = err.Error()
	}

	return strings.Join(messages, "; ")
}

// Add adds a validation error.
func (e *ValidationErrors) Add(field, message string) {
	e.Errors = append(e.Errors, &ValidationError{
		Field:   field,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors.
func (e *ValidationErrors) HasErrors() bool {
	return len(e.Errors) > 0
}

// SchemaBuilder provides a fluent interface for building schemas.
type SchemaBuilder struct {
	schema *Schema
}

// NewSchemaBuilder creates a new schema builder.
func NewSchemaBuilder() *SchemaBuilder {
	return &SchemaBuilder{
		schema: &Schema{
			Fields:   make(map[string]*FieldSchema),
			Required: make([]string, 0),
			Custom:   make([]CustomRule, 0),
		},
	}
}

// Field adds a field to the schema.
func (b *SchemaBuilder) Field(name string, fieldType FieldType) *FieldBuilder {
	fieldSchema := &FieldSchema{
		Type: fieldType,
	}
	b.schema.Fields[name] = fieldSchema
	return &FieldBuilder{
		parent: b,
		name:   name,
		schema: fieldSchema,
	}
}

// Required marks fields as required.
func (b *SchemaBuilder) Required(fields ...string) *SchemaBuilder {
	b.schema.Required = append(b.schema.Required, fields...)
	return b
}

// CustomRule adds a custom validation rule.
func (b *SchemaBuilder) CustomRule(name string, validate func(map[string]any) error) *SchemaBuilder {
	b.schema.Custom = append(b.schema.Custom, CustomRule{
		Name:     name,
		Validate: validate,
	})
	return b
}

// Build returns the built schema.
func (b *SchemaBuilder) Build() *Schema {
	return b.schema
}

// FieldBuilder provides a fluent interface for building field schemas.
type FieldBuilder struct {
	parent *SchemaBuilder
	name   string
	schema *FieldSchema
}

// Required marks the field as required.
func (fb *FieldBuilder) Required() *FieldBuilder {
	fb.schema.Required = true
	fb.parent.schema.Required = append(fb.parent.schema.Required, fb.name)
	return fb
}

// Default sets the default value.
func (fb *FieldBuilder) Default(value any) *FieldBuilder {
	fb.schema.Default = value
	return fb
}

// Min sets the minimum value.
func (fb *FieldBuilder) Min(value float64) *FieldBuilder {
	fb.schema.Min = &value
	return fb
}

// Max sets the maximum value.
func (fb *FieldBuilder) Max(value float64) *FieldBuilder {
	fb.schema.Max = &value
	return fb
}

// MinLength sets the minimum length.
func (fb *FieldBuilder) MinLength(value int) *FieldBuilder {
	fb.schema.MinLength = &value
	return fb
}

// MaxLength sets the maximum length.
func (fb *FieldBuilder) MaxLength(value int) *FieldBuilder {
	fb.schema.MaxLength = &value
	return fb
}

// Pattern sets the regex pattern.
func (fb *FieldBuilder) Pattern(pattern string) *FieldBuilder {
	fb.schema.Pattern = pattern
	return fb
}

// Enum sets the allowed values.
func (fb *FieldBuilder) Enum(values ...any) *FieldBuilder {
	fb.schema.Enum = values
	return fb
}

// Nested sets a nested schema.
func (fb *FieldBuilder) Nested(schema *Schema) *FieldBuilder {
	fb.schema.Nested = schema
	return fb
}

// ElementType sets the element type for arrays.
func (fb *FieldBuilder) ElementType(t FieldType) *FieldBuilder {
	fb.schema.ElementType = t
	return fb
}

// Custom sets a custom validator.
func (fb *FieldBuilder) Custom(validate func(any) error) *FieldBuilder {
	fb.schema.Custom = validate
	return fb
}

// Description sets the field description.
func (fb *FieldBuilder) Description(desc string) *FieldBuilder {
	fb.schema.Description = desc
	return fb
}

// And returns to the parent builder.
func (fb *FieldBuilder) And() *SchemaBuilder {
	return fb.parent
}

// Common validators

// RequiredValidator ensures required fields are present.
func RequiredValidator(fields ...string) Validator {
	return ValidatorFunc(func(data map[string]any) error {
		for _, field := range fields {
			if _, ok := Get(data, field); !ok {
				return &ValidationError{
					Field:   field,
					Message: "required field is missing",
				}
			}
		}
		return nil
	})
}

// TypeValidator ensures fields have the correct type.
func TypeValidator(types map[string]FieldType) Validator {
	return ValidatorFunc(func(data map[string]any) error {
		for field, expectedType := range types {
			value, exists := Get(data, field)
			if !exists {
				continue
			}

			if err := checkType(value, expectedType); err != nil {
				return &ValidationError{
					Field:   field,
					Message: err.Error(),
				}
			}
		}
		return nil
	})
}

// checkType checks if a value matches the expected type.
func checkType(value any, expected FieldType) error {
	if expected == TypeAny {
		return nil
	}

	var valid bool
	switch expected {
	case TypeString:
		_, valid = value.(string)
	case TypeInt:
		switch value.(type) {
		case int, int64, int32, float64:
			valid = true
		}
	case TypeFloat:
		switch value.(type) {
		case float64, float32, int, int64:
			valid = true
		}
	case TypeBool:
		_, valid = value.(bool)
	case TypeMap:
		_, valid = value.(map[string]any)
	case TypeArray:
		_, valid = value.([]any)
	}

	if !valid {
		return fmt.Errorf("expected type %s, got %T", expected, value)
	}

	return nil
}

// RangeValidator ensures numeric fields are within range.
func RangeValidator(field string, min, max float64) Validator {
	return ValidatorFunc(func(data map[string]any) error {
		value, exists := Get(data, field)
		if !exists {
			return nil
		}

		var num float64
		switch v := value.(type) {
		case int:
			num = float64(v)
		case int64:
			num = float64(v)
		case float64:
			num = v
		default:
			return nil
		}

		if num < min || num > max {
			return &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("value must be between %v and %v", min, max),
			}
		}

		return nil
	})
}

// PatternValidator ensures string fields match a pattern.
func PatternValidator(field, pattern string) Validator {
	re := regexp.MustCompile(pattern)
	return ValidatorFunc(func(data map[string]any) error {
		value, exists := Get(data, field)
		if !exists {
			return nil
		}

		str, ok := value.(string)
		if !ok {
			return nil
		}

		if !re.MatchString(str) {
			return &ValidationError{
				Field:   field,
				Message: fmt.Sprintf("value does not match pattern: %s", pattern),
			}
		}

		return nil
	})
}

// CompositeValidator combines multiple validators.
type CompositeValidator struct {
	validators []Validator
}

// NewCompositeValidator creates a new composite validator.
func NewCompositeValidator(validators ...Validator) *CompositeValidator {
	return &CompositeValidator{validators: validators}
}

// Validate runs all validators.
func (cv *CompositeValidator) Validate(data map[string]any) error {
	errors := &ValidationErrors{}

	for _, v := range cv.validators {
		if err := v.Validate(data); err != nil {
			if ve, ok := err.(*ValidationError); ok {
				errors.Errors = append(errors.Errors, ve)
			} else {
				errors.Add("", err.Error())
			}
		}
	}

	if errors.HasErrors() {
		return errors
	}

	return nil
}

// Add adds a validator.
func (cv *CompositeValidator) Add(v Validator) {
	cv.validators = append(cv.validators, v)
}

// PortValidator validates that a value is a valid port number.
func PortValidator(field string) Validator {
	return ValidatorFunc(func(data map[string]any) error {
		value, exists := Get(data, field)
		if !exists {
			return nil
		}

		var port int
		switch v := value.(type) {
		case int:
			port = v
		case int64:
			port = int(v)
		case float64:
			port = int(v)
		case string:
			p, err := strconv.Atoi(v)
			if err != nil {
				return &ValidationError{
					Field:   field,
					Message: "invalid port number",
				}
			}
			port = p
		default:
			return nil
		}

		if port < 1 || port > 65535 {
			return &ValidationError{
				Field:   field,
				Message: "port must be between 1 and 65535",
			}
		}

		return nil
	})
}

// URLValidator validates that a value is a valid URL.
func URLValidator(field string) Validator {
	pattern := `^(https?|ftp)://[^\s/$.?#].[^\s]*$`
	return PatternValidator(field, pattern)
}

// EmailValidator validates that a value is a valid email.
func EmailValidator(field string) Validator {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	return PatternValidator(field, pattern)
}

// IPValidator validates that a value is a valid IP address.
func IPValidator(field string) Validator {
	pattern := `^(\d{1,3}\.){3}\d{1,3}$`
	return PatternValidator(field, pattern)
}
