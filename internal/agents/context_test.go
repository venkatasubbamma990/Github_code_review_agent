package agents

import (
	"strings"
	"testing"
)

func TestFormatBriefForAgents(t *testing.T) {
	brief := &ContextBrief{
		Intent:       "Add rate limiting to the API",
		Summary:      "Focus on middleware and config",
		RiskAreas:    []string{"auth bypass", "perf under load"},
		Constraints:  []string{"must remain backward compatible"},
		LinkedIssues: []string{"#42"},
		FocusHints:   []string{"Check redis failure modes"},
	}
	got := FormatBriefForAgents(brief)
	for _, want := range []string{
		"Context Agent briefing",
		"Add rate limiting",
		"auth bypass",
		"#42",
		"redis failure",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFallbackBrief(t *testing.T) {
	brief := fallbackBrief(ReviewInput{PRContext: "PR #1: hello"})
	if brief.Summary == "" || len(brief.FocusHints) == 0 {
		t.Fatalf("unexpected fallback: %+v", brief)
	}
}

func TestBuildContextPromptIncludesMetadata(t *testing.T) {
	prompt := buildContextPrompt(ReviewInput{
		RepoFullName: "acme/api",
		PRNumber:     7,
		PRContext:    "PR #7: Fix login\nLabels: security",
		Files:        []SourceFile{{Path: "auth.go", Language: "go"}},
		Diff:         "@@ -1 +1 @@\n+token",
	})
	for _, want := range []string{"acme/api", "#7", "auth.go", "Fix login", "token"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt:\n%s", want, prompt)
		}
	}
}
