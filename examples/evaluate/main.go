// Example: run a test suite against a prompt (without calling the LLM).
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/klejdi94/loom"
	"github.com/klejdi94/loom/evaluator"
)

func main() {
	engine := loom.DefaultEngine()
	prompt := loom.New("sentiment-tests").
		WithSystem("You are a sentiment analyzer. Reply with one word: positive, negative, or neutral.").
		WithTemplate("Sentiment of: {{.text}}").
		WithVariable("text", loom.String(loom.Required())).
		Build(engine)

	// Without an executor, the suite tests rendering only (actual = rendered template).
	// With executor.WithExecutor(exec), actual would be the LLM response.
	suite := evaluator.NewTestSuite("sentiment-tests").
		WithPrompt(prompt, "v1.0.0").
		AddCase("positive", map[string]interface{}{"text": "I love this product!"}, evaluator.Expected{Output: "Sentiment of: I love this product!"}).
		AddCase("negative", map[string]interface{}{"text": "Terrible experience"}, evaluator.Expected{Output: "Sentiment of: Terrible experience"}).
		AddCase("neutral", map[string]interface{}{"text": "It's okay"}, evaluator.Expected{Output: "Sentiment of: It's okay"}).
		WithEvaluator(evaluator.ExactMatch{})

	report, err := suite.Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Suite: %s, Passed: %d, Failed: %d, Duration: %v\n",
		report.Suite, report.Passed, report.Failed, report.Duration)
	for _, r := range report.Results {
		fmt.Printf("  %s: pass=%v\n", r.CaseName, r.Pass)
	}
}
