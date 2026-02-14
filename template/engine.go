// Package template provides the template rendering engine for prompts.
package template

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/klejdi94/loom/core"
)

// Engine renders prompt templates using Go text/template with custom functions.
type Engine struct {
	leftDelim  string
	rightDelim string
	funcMap    template.FuncMap
}

// EngineOption configures the engine.
type EngineOption func(*Engine)

// WithDelims sets custom delimiters (default "{{" and "}}").
func WithDelims(left, right string) EngineOption {
	return func(e *Engine) {
		e.leftDelim = left
		e.rightDelim = right
	}
}

// WithFuncMap adds custom template functions.
func WithFuncMap(fm template.FuncMap) EngineOption {
	return func(e *Engine) {
		for k, v := range fm {
			e.funcMap[k] = v
		}
	}
}

// NewEngine creates a new template engine with default or custom options.
func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		leftDelim:  "{{",
		rightDelim: "}}",
		funcMap:    defaultFuncMap(),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		"join":    strings.Join,
		"upper":   strings.ToUpper,
		"lower":   strings.ToLower,
		"trim":    strings.TrimSpace,
		"default": defaultFunc,
		"json":    jsonFunc,
	}
}

func defaultFunc(def, val interface{}) interface{} {
	if val == nil || val == "" {
		return def
	}
	return val
}

func jsonFunc(v interface{}) string {
	// Simple JSON-like representation for template debugging
	return fmt.Sprint(v)
}

// Render implements core.Renderer. It validates input, then renders system and template.
func (e *Engine) Render(ctx context.Context, p *core.Prompt, input core.Input) (*core.Rendered, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if err := p.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("%w: %w", core.ErrValidationFailed, err)
	}
	// Apply defaults
	data := make(map[string]interface{}, len(input))
	for _, v := range p.Variables {
		if val, ok := input[v.Name]; ok {
			data[v.Name] = val
		} else if v.Default != nil {
			data[v.Name] = v.Default
		}
	}
	for k, v := range input {
		data[k] = v
	}
	system, err := e.execute(p.System, data)
	if err != nil {
		return nil, fmt.Errorf("%w system: %w", core.ErrRenderFailed, err)
	}
	user, err := e.execute(p.Template, data)
	if err != nil {
		return nil, fmt.Errorf("%w template: %w", core.ErrRenderFailed, err)
	}
	return &core.Rendered{
		System: system,
		User:   user,
		Input:  input,
	}, nil
}

// execute parses and executes a single template string with data.
func (e *Engine) execute(tpl string, data map[string]interface{}) (string, error) {
	if tpl == "" {
		return "", nil
	}
	t, err := template.New("").Delims(e.leftDelim, e.rightDelim).Funcs(e.funcMap).Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
