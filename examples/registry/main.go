// Example: store and retrieve prompts from the in-memory registry.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/klejdi94/loom"
	"github.com/klejdi94/loom/registry"
	"github.com/klejdi94/loom/template"
)

func main() {
	eng := template.NewEngine()
	reg := registry.NewMemoryRegistry()
	ctx := context.Background()

	prompt := loom.New("greeter").
		WithVersion("1.0.0").
		WithTemplate("Hello, {{.name}}!").
		WithVariable("name", loom.String(loom.Required())).
		Build(eng)

	if err := reg.Store(ctx, prompt); err != nil {
		log.Fatal(err)
	}
	if err := reg.Promote(ctx, "greeter", "1.0.0", registry.StageProduction); err != nil {
		log.Fatal(err)
	}

	prod, err := reg.GetProduction(ctx, "greeter")
	if err != nil {
		log.Fatal(err)
	}
	prod.SetRenderer(eng)
	rendered, _ := prod.Render(ctx, loom.Input{"name": "World"})
	fmt.Println(rendered.User)
}
