package template

import (
	"context"
	"testing"

	"github.com/klejdi94/loom/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_Render(t *testing.T) {
	eng := NewEngine()
	p := &core.Prompt{
		System:   "You are {{.role}}.",
		Template: "Hello, {{.name}}!",
		Variables: []core.Variable{
			{Name: "role", Type: core.VariableTypeString, Required: true},
			{Name: "name", Type: core.VariableTypeString, Required: true},
		},
	}
	rendered, err := eng.Render(context.Background(), p, core.Input{"role": "assistant", "name": "World"})
	require.NoError(t, err)
	assert.Equal(t, "You are assistant.", rendered.System)
	assert.Equal(t, "Hello, World!", rendered.User)
}

func TestEngine_Render_ValidationFails(t *testing.T) {
	eng := NewEngine()
	p := &core.Prompt{
		Template: "Hi {{.name}}",
		Variables: []core.Variable{
			{Name: "name", Type: core.VariableTypeString, Required: true},
		},
	}
	_, err := eng.Render(context.Background(), p, core.Input{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, core.ErrValidationFailed)
}

func TestEngine_Render_Default(t *testing.T) {
	eng := NewEngine()
	p := &core.Prompt{
		Template: "Hi {{.name}}",
		Variables: []core.Variable{
			{Name: "name", Type: core.VariableTypeString, Default: "Guest"},
		},
	}
	rendered, err := eng.Render(context.Background(), p, core.Input{})
	require.NoError(t, err)
	assert.Equal(t, "Hi Guest", rendered.User)
}
