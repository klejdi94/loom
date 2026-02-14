package evaluator

import (
	"context"
	"fmt"
	"time"

	"github.com/klejdi94/loom/core"
	"github.com/klejdi94/loom/executor"
)

// Suite runs a set of test cases against a prompt (or executor).
type Suite struct {
	name    string
	prompt  *core.Prompt
	exec    *executor.Executor
	cases   []Case
	evals   []Evaluator
	version string
}

// NewTestSuite creates a new test suite with the given name.
func NewTestSuite(name string) *Suite {
	return &Suite{name: name, evals: []Evaluator{ExactMatch{}}}
}

// WithPrompt sets the prompt to test (and optional version label).
func (s *Suite) WithPrompt(p *core.Prompt, version string) *Suite {
	s.prompt = p
	s.version = version
	return s
}

// WithExecutor sets the executor (used to run prompt and get actual output).
func (s *Suite) WithExecutor(e *executor.Executor) *Suite {
	s.exec = e
	return s
}

// AddCase adds a test case.
func (s *Suite) AddCase(name string, input map[string]interface{}, expected Expected) *Suite {
	s.cases = append(s.cases, Case{Name: name, Input: input, Expected: expected})
	return s
}

// WithEvaluator adds an evaluator (e.g. ExactMatch, ContainsAll).
func (s *Suite) WithEvaluator(ev Evaluator) *Suite {
	s.evals = append(s.evals, ev)
	return s
}

// Report holds the results of running a suite.
type Report struct {
	Suite    string
	PromptID string
	Version  string
	Total    int
	Passed   int
	Failed   int
	Results  []CaseResult
	Duration time.Duration
}

// CaseResult is the result of one test case.
type CaseResult struct {
	CaseName string
	Pass     bool
	Actual   string
	Expected Expected
	Scores   []Score
	Error    error
}

// Run executes all cases and returns a report. If no executor is set, only rendering is tested.
func (s *Suite) Run(ctx context.Context) (*Report, error) {
	if s.prompt == nil {
		return nil, fmt.Errorf("evaluator: prompt is required")
	}
	start := time.Now()
	report := &Report{
		Suite:    s.name,
		PromptID: s.prompt.ID,
		Version:  s.version,
		Total:    len(s.cases),
		Results:  make([]CaseResult, 0, len(s.cases)),
	}
	for _, c := range s.cases {
		res := s.runCase(ctx, c)
		report.Results = append(report.Results, res)
		if res.Pass {
			report.Passed++
		} else {
			report.Failed++
		}
	}
	report.Duration = time.Since(start)
	return report, nil
}

func (s *Suite) runCase(ctx context.Context, c Case) CaseResult {
	out := CaseResult{CaseName: c.Name, Expected: c.Expected}
	var actual string
	if s.exec != nil {
		result, err := s.exec.Execute(ctx, executor.ExecuteRequest{
			Prompt: s.prompt,
			Input:  c.Input,
		})
		if err != nil {
			out.Error = err
			out.Pass = false
			return out
		}
		actual = result.Content
	} else {
		rendered, err := s.prompt.Render(ctx, c.Input)
		if err != nil {
			out.Error = err
			out.Pass = false
			return out
		}
		actual = rendered.User
	}
	out.Actual = actual
	allPass := true
	for _, ev := range s.evals {
		score, err := ev.Evaluate(ctx, actual, c.Expected)
		if err != nil {
			out.Error = err
			out.Pass = false
			return out
		}
		out.Scores = append(out.Scores, score)
		if !score.Pass {
			allPass = false
		}
	}
	out.Pass = allPass
	return out
}
