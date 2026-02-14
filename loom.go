// Package loom provides a production-ready Go library for managing, testing,
// optimizing, and versioning prompts for Large Language Models (LLMs).
//
// Quick start:
//
//	engine := loom.DefaultEngine()
//	prompt := loom.New("sentiment-analyzer").
//		WithSystem("You are an expert sentiment analyzer.").
//		WithTemplate("Analyze the sentiment of: {{.text}}").
//		WithVariable("text", loom.String(loom.Required())).
//		WithExample(map[string]interface{}{"text": "I love this!"}, "positive").
//		Build(engine)
//
//	result, err := prompt.Render(context.Background(), loom.Input{
//		"text": "This product is amazing!",
//	})
package loom

import (
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/template"
)

var defaultEngine *template.Engine

func init() {
	defaultEngine = template.NewEngine()
}

// DefaultEngine returns the shared default template engine (used by Build when nil is passed).
func DefaultEngine() *template.Engine {
	return defaultEngine
}

// Builder constructs a Prompt via a fluent API.
type Builder struct {
	id          string
	version     string
	name        string
	description string
	system      string
	tpl         string
	variables   []core.Variable
	examples    []core.Example
	metadata    map[string]interface{}
}

// New starts a new prompt builder with the given id.
func New(id string) *Builder {
	return &Builder{
		id:        id,
		version:   "1.0.0",
		metadata:  make(map[string]interface{}),
		variables: nil,
		examples:  nil,
	}
}

// WithVersion sets the prompt version (semantic versioning).
func (b *Builder) WithVersion(v string) *Builder {
	b.version = v
	return b
}

// WithName sets the human-readable name.
func (b *Builder) WithName(name string) *Builder {
	b.name = name
	return b
}

// WithDescription sets the description.
func (b *Builder) WithDescription(desc string) *Builder {
	b.description = desc
	return b
}

// WithSystem sets the system message template.
func (b *Builder) WithSystem(system string) *Builder {
	b.system = system
	return b
}

// WithTemplate sets the user message template (supports Go text/template syntax).
func (b *Builder) WithTemplate(tpl string) *Builder {
	b.tpl = tpl
	return b
}

// WithVariable adds a variable definition. Use core.String(), core.Int(), etc. with options.
func (b *Builder) WithVariable(name string, v core.Variable) *Builder {
	v.Name = name
	b.variables = append(b.variables, v)
	return b
}

// WithExample adds a few-shot example (input map -> output string).
func (b *Builder) WithExample(input map[string]interface{}, output string) *Builder {
	b.examples = append(b.examples, core.Example{Input: input, Output: output, Weight: 1.0})
	return b
}

// WithExampleWeight adds a few-shot example with a custom weight.
func (b *Builder) WithExampleWeight(input map[string]interface{}, output string, weight float64) *Builder {
	b.examples = append(b.examples, core.Example{Input: input, Output: output, Weight: weight})
	return b
}

// WithMetadata sets or merges metadata key-value pairs.
func (b *Builder) WithMetadata(m map[string]interface{}) *Builder {
	for k, v := range m {
		b.metadata[k] = v
	}
	return b
}

// Build produces the Prompt and attaches the given engine as its renderer.
// If eng is nil, the default engine is used.
func (b *Builder) Build(eng *template.Engine) *core.Prompt {
	if eng == nil {
		eng = defaultEngine
	}
	now := time.Now()
	p := &core.Prompt{
		ID:          b.id,
		Version:     b.version,
		Name:        b.name,
		Description: b.description,
		System:      b.system,
		Template:    b.tpl,
		Variables:   append([]core.Variable(nil), b.variables...),
		Examples:    append([]core.Example(nil), b.examples...),
		Metadata:    make(map[string]interface{}),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	for k, v := range b.metadata {
		p.Metadata[k] = v
	}
	p.SetRenderer(eng)
	return p
}

// Re-export core types for convenience.
type (
	// Input is the map of variable names to values for rendering.
	Input = core.Input
	// Rendered is the result of rendering a prompt.
	Rendered = core.Rendered
	// Variable is a prompt variable definition.
	Variable = core.Variable
	// Example is a few-shot example.
	Example = core.Example
)

// Variable constructors (re-export from core).
var (
	String         = core.String
	Int            = core.Int
	Float          = core.Float
	Bool           = core.Bool
	Any            = core.Any
	Required       = core.Required
	Default        = core.Default
	WithValidation = core.WithValidation
	WithDescription = core.WithDescription
)
