// Package evaluator provides testing and evaluation for prompts.
package evaluator

import (
	"context"
	"strings"
)

// Case represents a single test case: input, expected output, and optional constraints.
type Case struct {
	Name     string
	Input    map[string]interface{}
	Expected Expected
}

// Expected describes expected outcome (output and optional checks).
type Expected struct {
	Output      string
	Contains    []string
	NotContains []string
	Evaluators  []Evaluator
}

// Evaluator scores or validates an actual output against expectations.
type Evaluator interface {
	Evaluate(ctx context.Context, actual string, expected Expected) (Score, error)
}

// Score represents an evaluation score (0-1 or pass/fail).
type Score struct {
	Pass  bool
	Value float64
	Reason string
}

// ExactMatch evaluates that actual equals expected output (trimmed).
type ExactMatch struct{}

// Evaluate implements Evaluator.
func (ExactMatch) Evaluate(ctx context.Context, actual string, expected Expected) (Score, error) {
	actual = strings.TrimSpace(actual)
	exp := strings.TrimSpace(expected.Output)
	pass := actual == exp
	val := 0.0
	if pass {
		val = 1.0
	}
	return Score{Pass: pass, Value: val, Reason: "exact match"}, nil
}

// ContainsAll checks that actual contains all of the expected substrings.
type ContainsAll struct {
	Substrings []string
}

// Evaluate implements Evaluator.
func (c ContainsAll) Evaluate(ctx context.Context, actual string, expected Expected) (Score, error) {
	check := c.Substrings
	if len(check) == 0 {
		check = expected.Contains
	}
	for _, sub := range check {
		if !strings.Contains(actual, sub) {
			return Score{Pass: false, Value: 0, Reason: "missing: " + sub}, nil
		}
	}
	return Score{Pass: true, Value: 1.0, Reason: "contains all"}, nil
}

// FuncEvaluator adapts a function to Evaluator.
type FuncEvaluator func(ctx context.Context, actual string, expected Expected) (Score, error)

func (f FuncEvaluator) Evaluate(ctx context.Context, actual string, expected Expected) (Score, error) {
	return f(ctx, actual, expected)
}
