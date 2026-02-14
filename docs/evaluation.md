# Evaluation

## Evaluators

- **ExactMatch**: Actual output must equal expected output (trimmed).
- **ContainsAll**: Actual must contain all of `Expected.Contains` or the evaluator’s `Substrings`.
- **FuncEvaluator**: Wrap a function `func(ctx, actual, expected) (Score, error)`.
- **LLMJudge** (Phase 3): Calls an LLM to compare actual vs expected. Set `Provider`, `Model` (e.g. `gpt-4o-mini`), and `Criteria`. The judge prompt asks for a line `SCORE: <0.0-1.0>` and `PASS` or `FAIL`; the response is parsed to produce a `Score`.

## Test suite

```go
suite := evaluator.NewTestSuite("my-suite").
    WithPrompt(prompt, "v1.0.0").
    WithExecutor(exec).  // optional: run through LLM
    AddCase("case1", input, evaluator.Expected{Output: "expected"}).
    WithEvaluator(evaluator.ExactMatch{}).
    WithEvaluator(evaluator.LLMJudge{Provider: openai, Model: "gpt-4o-mini", Criteria: "accuracy"})
report, _ := suite.Run(ctx)
```

## Custom evaluator

Implement the `Evaluator` interface:

```go
type Evaluator interface {
    Evaluate(ctx context.Context, actual string, expected Expected) (Score, error)
}
```

Or use `evaluator.FuncEvaluator(func(ctx context.Context, actual string, expected Expected) (evaluator.Score, error) { ... })`.
