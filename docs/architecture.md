# Architecture

## Overview

loom is organized into focused packages that compose together:

- **core**: Fundamental types (`Prompt`, `Variable`, `Example`, `Input`, `Rendered`) and the `Renderer` interface. No external dependencies.
- **template**: Implements `Renderer` using Go `text/template`; validates input and applies defaults before rendering.
- **registry**: Memory, file-based, or PostgreSQL. All return copies of prompts; file and Postgres persist to disk/DB.
- **provider**: OpenAI and Ollama; `Complete`, `Stream`, `GetModelInfo`.
- **executor**: Renders a prompt and calls a `Provider` with retry and timeout.
- **evaluator**: Test suites (input + expected), optional executor, evaluators (exact match, contains).
- **chain**: Multi-step flows: sequential steps, parallel groups, per-step retry/timeout/fallback/condition; optional executor for LLM calls.
- **optimizer**: A/B experiments with weighted traffic split, success recording, min sample size, confidence, winner detection, and promotion.
- **middleware**: Logging, metrics, in-memory cache, rate limit, circuit breaker; chain with `middleware.Chain(p, mws...)`.
- **cost**: Token counting (heuristic), cost estimation per model, and tracker for recording usage/cost.
- **registry (Phase 3)**: Redis (distributed), S3 via BlobStore (registry/s3blob for AWS S3).
- **evaluator (Phase 3)**: LLMJudge calls an LLM to score actual vs expected and parse SCORE/PASS/FAIL.
- **analytics**: RunRecord (prompt id, version, latency, tokens, success); Store.Record and Query for aggregates (by prompt, version, day/hour). MemoryStore is the in-memory implementation.
- **optimizer (Phase 3)**: WithOnWinner(callback) invokes once when HasWinner becomes true for auto-promotion.

## Data flow

1. **Build**: `loom.New(id).WithTemplate(...).WithVariable(...).Build(engine)` produces a `*core.Prompt` with an attached `Renderer` (the template engine).
2. **Render**: `prompt.Render(ctx, input)` validates input, applies defaults, and renders system + user strings.
3. **Execute**: `executor.Execute(ctx, ExecuteRequest{Prompt, Input, ...})` renders then calls the provider; retries on failure.
4. **Store**: `registry.Store(ctx, prompt)` stores a copy (no renderer); `Get`/`GetProduction` return copies that need a renderer set again for `Render()`.

## Design decisions

- **Renderer injection**: Prompts don’t depend on the template package; the root package wires the default engine so `Build(engine)` sets the renderer. Registries return prompts without a renderer so the caller can attach the engine they use.
- **Context everywhere**: All I/O and execution paths accept `context.Context` for cancellation and timeouts.
- **Copy on read**: Registry implementations return copies of prompts so callers can’t mutate stored data and so each caller can set its own renderer.
- **Functional options**: Builders and configs use optional functions (e.g. `WithRetry`, `WithVariable(..., String(Required()))`) for clarity and extensibility.
