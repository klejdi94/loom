package core

import "fmt"

// VariableType represents the type of a template variable.
type VariableType string

const (
	VariableTypeString  VariableType = "string"
	VariableTypeInt     VariableType = "int"
	VariableTypeFloat   VariableType = "float"
	VariableTypeBool    VariableType = "bool"
	VariableTypeAny     VariableType = "any"
)

// ValidationFunc validates a variable value. Returns nil if valid.
type ValidationFunc func(value interface{}) error

// Variable defines expected input to a prompt template.
type Variable struct {
	Name        string
	Type        VariableType
	Required    bool
	Default     interface{}
	Validation  ValidationFunc
	Description string
}

// Validate checks the variable value against type and custom validation.
func (v *Variable) Validate(value interface{}) error {
	if value == nil {
		if v.Required {
			return &ValidationError{Field: v.Name, Value: value, Message: "required field is missing"}
		}
		return nil
	}
	switch v.Type {
	case VariableTypeString:
		if _, ok := value.(string); !ok {
			return &ValidationError{Field: v.Name, Value: value, Message: "expected string"}
		}
	case VariableTypeInt:
		switch value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			// ok
		default:
			return &ValidationError{Field: v.Name, Value: value, Message: "expected integer"}
		}
	case VariableTypeFloat:
		switch value.(type) {
		case float32, float64:
			// ok
		default:
			return &ValidationError{Field: v.Name, Value: value, Message: "expected float"}
		}
	case VariableTypeBool:
		if _, ok := value.(bool); !ok {
			return &ValidationError{Field: v.Name, Value: value, Message: "expected bool"}
		}
	case VariableTypeAny:
		// no type check
	}
	if v.Validation != nil {
		return v.Validation(value)
	}
	return nil
}

// VariableOption configures a Variable (functional option).
type VariableOption func(*Variable)

// Required marks the variable as required.
func Required() VariableOption {
	return func(v *Variable) {
		v.Required = true
	}
}

// Default sets the default value when not provided.
func Default(val interface{}) VariableOption {
	return func(v *Variable) {
		v.Default = val
	}
}

// WithValidation sets a custom validation function.
func WithValidation(fn ValidationFunc) VariableOption {
	return func(v *Variable) {
		v.Validation = fn
	}
}

// WithDescription sets the variable description.
func WithDescription(desc string) VariableOption {
	return func(v *Variable) {
		v.Description = desc
	}
}

// String returns a Variable configured as string type.
func String(opts ...VariableOption) Variable {
	v := Variable{Name: "", Type: VariableTypeString}
	for _, o := range opts {
		o(&v)
	}
	return v
}

// Int returns a Variable configured as int type.
func Int(opts ...VariableOption) Variable {
	v := Variable{Name: "", Type: VariableTypeInt}
	for _, o := range opts {
		o(&v)
	}
	return v
}

// Float returns a Variable configured as float type.
func Float(opts ...VariableOption) Variable {
	v := Variable{Name: "", Type: VariableTypeFloat}
	for _, o := range opts {
		o(&v)
	}
	return v
}

// Bool returns a Variable configured as bool type.
func Bool(opts ...VariableOption) Variable {
	v := Variable{Name: "", Type: VariableTypeBool}
	for _, o := range opts {
		o(&v)
	}
	return v
}

// Any returns a Variable that accepts any type.
func Any(opts ...VariableOption) Variable {
	v := Variable{Name: "", Type: VariableTypeAny}
	for _, o := range opts {
		o(&v)
	}
	return v
}

// variableWithName is used by the builder to set Name after creation.
func variableWithName(name string, base Variable) Variable {
	base.Name = name
	return base
}

// CoerceToString attempts to coerce value to string for template rendering.
func CoerceToString(value interface{}) (string, error) {
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	default:
		return fmt.Sprint(v), nil
	}
}
