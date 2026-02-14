package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrompt_ValidateInput(t *testing.T) {
	p := &Prompt{
		Variables: []Variable{
			{Name: "a", Type: VariableTypeString, Required: true},
			{Name: "b", Type: VariableTypeInt, Default: 10},
		},
	}
	assert.Error(t, p.ValidateInput(Input{}))
	assert.NoError(t, p.ValidateInput(Input{"a": "x", "b": 5}))
	assert.NoError(t, p.ValidateInput(Input{"a": "x"}))
}

func TestPrompt_Render_NoRenderer(t *testing.T) {
	p := &Prompt{ID: "test", Template: "hello"}
	_, err := p.Render(context.Background(), Input{})
	assert.ErrorIs(t, err, ErrNoRenderer)
}

func TestPrompt_Copy(t *testing.T) {
	p := &Prompt{
		ID: "x", Version: "1",
		Variables: []Variable{{Name: "a", Type: VariableTypeString}},
		Examples:  []Example{{Output: "out"}},
		Metadata:  map[string]interface{}{"k": "v"},
	}
	q := p.Copy()
	require.NotSame(t, p, q)
	assert.Equal(t, p.ID, q.ID)
	assert.Equal(t, p.Version, q.Version)
	assert.NotSame(t, p.Variables, q.Variables)
	assert.NotSame(t, p.Examples, q.Examples)
	assert.NotSame(t, p.Metadata, q.Metadata)
	assert.Nil(t, q.renderer)
}
