package core

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Example represents a few-shot example (input -> output) with optional weight.
type Example struct {
	Input  map[string]interface{}
	Output string
	Weight float64
}

// Prompt represents a versioned prompt template.
// Renderer is set by the builder when using the template engine.
type Prompt struct {
	ID          string
	Version     string
	Name        string
	Description string
	System      string
	Template    string
	Variables   []Variable
	Examples    []Example
	Metadata    map[string]interface{}
	CreatedAt   time.Time
	UpdatedAt   time.Time
	renderer    Renderer // optional; set by builder for Render()
}

// ErrNoRenderer is returned when Render is called without a renderer configured.
var ErrNoRenderer = errors.New("no renderer configured on prompt")

// Input is the map of variable names to values passed when rendering a prompt.
type Input map[string]interface{}

// Rendered holds the result of rendering a prompt (system + user message).
type Rendered struct {
	System string
	User   string
	Input  Input
}

// Renderer is implemented by the template package to render prompts.
type Renderer interface {
	Render(ctx context.Context, p *Prompt, input Input) (*Rendered, error)
}

// SetRenderer sets the renderer used by Render. Used by the builder.
func (p *Prompt) SetRenderer(r Renderer) {
	p.renderer = r
}

// Render renders the prompt with the given input using the configured renderer.
func (p *Prompt) Render(ctx context.Context, input Input) (*Rendered, error) {
	if p.renderer == nil {
		return nil, ErrNoRenderer
	}
	return p.renderer.Render(ctx, p, input)
}

// ValidateInput checks that input satisfies all required variables and types.
func (p *Prompt) ValidateInput(input Input) error {
	vm := make(map[string]Variable)
	for _, v := range p.Variables {
		vm[v.Name] = v
	}
	for name, v := range vm {
		val, ok := input[name]
		if !ok {
			val = v.Default
		}
		if err := v.Validate(val); err != nil {
			return fmt.Errorf("variable %q: %w", name, err)
		}
	}
	return nil
}

// VariableMap returns a map of variable name to Variable for lookup.
func (p *Prompt) VariableMap() map[string]Variable {
	m := make(map[string]Variable, len(p.Variables))
	for _, v := range p.Variables {
		m[v.Name] = v
	}
	return m
}

// Copy returns a deep copy of the prompt with no renderer set.
func (p *Prompt) Copy() *Prompt {
	q := *p
	q.Variables = append([]Variable(nil), p.Variables...)
	q.Examples = append([]Example(nil), p.Examples...)
	q.Metadata = make(map[string]interface{})
	for k, v := range p.Metadata {
		q.Metadata[k] = v
	}
	q.renderer = nil
	return &q
}
