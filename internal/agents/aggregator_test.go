package agents

import (
	"testing"

	"codereviewagent/internal/models"
)

func TestRuleBasedMergeDedupesByTitleAndPath(t *testing.T) {
	outputs := []*AgentOutput{
		{
			Agent: AgentSecurity,
			Score: 40,
			Findings: []models.Finding{
				{Title: "SQL Injection", FilePath: "db.go", Severity: models.SeverityCritical},
			},
			Strengths:   []string{"good auth"},
			Suggestions: []string{"use prepared statements"},
		},
		{
			Agent: AgentQuality,
			Score: 70,
			Findings: []models.Finding{
				{Title: "SQL Injection", FilePath: "db.go", Severity: models.SeverityHigh},
				{Title: "Long function", FilePath: "db.go", Severity: models.SeverityMedium},
			},
			Strengths: []string{"good auth"},
		},
	}

	merged := ruleBasedMerge(outputs)
	if len(merged.findings) != 2 {
		t.Fatalf("expected 2 findings after dedupe, got %d", len(merged.findings))
	}
	if merged.findings[0].Severity != models.SeverityCritical {
		t.Fatalf("expected critical kept on duplicate, got %s", merged.findings[0].Severity)
	}
	if len(merged.strengths) != 1 {
		t.Fatalf("expected unique strengths, got %v", merged.strengths)
	}
	if merged.scores[string(AgentSecurity)] != 40 || merged.scores[string(AgentQuality)] != 70 {
		t.Fatalf("scores not preserved: %v", merged.scores)
	}
}

func TestRuleBasedMergeKeepsHigherSeverityFromBugAgent(t *testing.T) {
	outputs := []*AgentOutput{
		{
			Agent: AgentQuality,
			Score: 80,
			Findings: []models.Finding{
				{Title: "Nil deref", FilePath: "main.go", Severity: models.SeverityLow},
			},
		},
		{
			Agent: AgentBug,
			Score: 35,
			Findings: []models.Finding{
				{Title: "Nil deref", FilePath: "main.go", Severity: models.SeverityCritical},
			},
		},
	}
	merged := ruleBasedMerge(outputs)
	if len(merged.findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(merged.findings))
	}
	if merged.findings[0].Severity != models.SeverityCritical {
		t.Fatalf("bug agent critical should win, got %s", merged.findings[0].Severity)
	}
}

func TestRuleBasedResultScores(t *testing.T) {
	outputs := []*AgentOutput{
		{Agent: AgentSecurity, Score: 50},
		{Agent: AgentQuality, Score: 80},
		{Agent: AgentPerformance, Score: 60},
		{Agent: AgentStyle, Score: 90},
	}
	merged := ruleBasedMerge(outputs)
	result := ruleBasedResult(ReviewInput{Source: "manual", Language: "go"}, merged, outputs)
	if result.Quality.Security != 50 {
		t.Fatalf("security=%d", result.Quality.Security)
	}
	if result.Quality.Performance != 60 {
		t.Fatalf("performance=%d", result.Quality.Performance)
	}
	if result.Quality.Overall == 0 {
		t.Fatal("overall should be average of agent scores")
	}
	if len(result.AgentReports) != 4 {
		t.Fatalf("agent reports=%d", len(result.AgentReports))
	}
}
