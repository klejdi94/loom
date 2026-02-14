// Example: build a prompt, render it, and show the output.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/klejdi94/loom"
)

func main() {
	engine := loom.DefaultEngine()
	prompt := loom.New("sentiment-analyzer").
		WithSystem("You are an expert sentiment analyzer.").
		WithTemplate("Analyze the sentiment of: {{.text}}").
		WithVariable("text", loom.String(loom.Required())).
		WithExample(map[string]interface{}{"text": "I love this!"}, "positive").
		WithMetadata(map[string]interface{}{
			"domain": "customer-feedback",
			"model":  "gpt-4",
		}).
		Build(engine)

	result, err := prompt.Render(context.Background(), loom.Input{
		"text": "This product is amazing!",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("System:", result.System)
	fmt.Println("User:", result.User)
}
