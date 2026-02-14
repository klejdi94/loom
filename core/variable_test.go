package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariable_Validate_String(t *testing.T) {
	v := Variable{Name: "x", Type: VariableTypeString, Required: true}
	assert.Error(t, v.Validate(nil))
	assert.NoError(t, v.Validate("hello"))
	assert.Error(t, v.Validate(42))
}

func TestVariable_Validate_OptionalWithDefault(t *testing.T) {
	v := Variable{Name: "x", Type: VariableTypeString, Default: "default"}
	assert.NoError(t, v.Validate(nil))
	assert.NoError(t, v.Validate("custom"))
}

func TestString_Required(t *testing.T) {
	v := String(Required())
	v.Name = "text"
	require.True(t, v.Required)
	require.Equal(t, VariableTypeString, v.Type)
	assert.Error(t, v.Validate(nil))
}

func TestInt_Validate(t *testing.T) {
	v := Int()
	v.Name = "n"
	assert.NoError(t, v.Validate(1))
	assert.NoError(t, v.Validate(int64(2)))
	assert.Error(t, v.Validate("1"))
}

func TestVariable_Validate_CustomFunc(t *testing.T) {
	v := Variable{
		Name:       "x",
		Type:       VariableTypeString,
		Validation: func(value interface{}) error {
			if s, ok := value.(string); ok && len(s) < 3 {
				return &ValidationError{Field: "x", Message: "too short"}
			}
			return nil
		},
	}
	assert.Error(t, v.Validate("ab"))
	assert.NoError(t, v.Validate("abc"))
}
