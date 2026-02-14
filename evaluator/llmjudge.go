// Package evaluator LLM-as-judge: use an LLM to score actual vs expected output.
package evaluator

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/klejdi94/loom/provider"
)

// LLMJudge is an evaluator that calls an LLM to judge whether actual output meets the expected/criteria.
type LLMJudge struct {
	Provider provider.Provider
	Model    string
	Criteria string
	// System prompt for the judge; if empty, a default is used.
	System string
}

// DefaultJudgeSystem is the default system prompt for the judge model.
const DefaultJudgeSystem = `You are an impartial judge. Evaluate the actual output against the expected output and criteria. Reply with exactly two lines:
Line 1: SCORE: <number from 0.0 to 1.0>
Line 2: PASS or FAIL
Then optionally a brief reason on the next line.`

// Evaluate implements Evaluator. It calls the provider with a prompt containing expected, actual, and criteria, then parses SCORE and PASS/FAIL.
func (j *LLMJudge) Evaluate(ctx context.Context, actual string, expected Expected) (Score, error) {
	system := j.System
	if system == "" {
		system = DefaultJudgeSystem
	}
	criteria := j.Criteria
	if criteria == "" {
		criteria = "Relevance and correctness compared to expected output."
	}
	prompt := fmt.Sprintf("Expected output:\n%s\n\nActual output:\n%s\n\nCriteria: %s\n\nProvide SCORE (0.0-1.0) and PASS or FAIL.",
		expected.Output, actual, criteria)
	model := j.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	req := provider.CompletionRequest{
		Model:  model,
		System: system,
		Prompt: prompt,
	}
	resp, err := j.Provider.Complete(ctx, req)
	if err != nil {
		return Score{Pass: false, Value: 0, Reason: "judge call failed: " + err.Error()}, nil
	}
	content := strings.TrimSpace(resp.Content)
	score, pass, reason := parseJudgeResponse(content)
	return Score{Pass: pass, Value: score, Reason: reason}, nil
}

var (
	scoreLineRe = regexp.MustCompile(`(?i)score:\s*([0-9.]+)`)
	passFailRe  = regexp.MustCompile(`(?i)\b(PASS|FAIL)\b`)
)

func parseJudgeResponse(content string) (value float64, pass bool, reason string) {
	lines := strings.Split(content, "\n")
	value = 0
	reason = content
	explicitPass := false
	hasExplicit := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if m := scoreLineRe.FindStringSubmatch(line); len(m) >= 2 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil && v >= 0 && v <= 1 {
				value = v
			}
		}
		if passFailRe.MatchString(line) {
			hasExplicit = true
			explicitPass = strings.Contains(strings.ToUpper(line), "PASS")
		}
	}
	if hasExplicit {
		pass = explicitPass
	} else {
		pass = value >= 0.7
	}
	return value, pass, reason
}
