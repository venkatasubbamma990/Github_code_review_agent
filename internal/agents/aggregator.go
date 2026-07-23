package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"codereviewagent/internal/llm"
	"codereviewagent/internal/models"
)

const aggregatorSystemPrompt = `You are the Aggregator Agent in a multi-agent code review system.
You receive findings from specialist agents (security, bug, dependency, quality, performance, style, test).

Your job:
1. Merge and deduplicate findings (remove duplicates, keep the highest severity)
2. Produce a unified quality assessment
3. Prioritize the most important issues (prefer security, bug, and dependency supply-chain findings when severity ties)
4. Return ONLY valid JSON matching this schema:
{
  "quality": {
    "overall": <0-100>,
    "maintainability": <0-100>,
    "readability": <0-100>,
    "security": <0-100>,
    "performance": <0-100>,
    "summary": "<executive summary of the full review>"
  },
  "findings": [
    {
      "category": "<category>",
      "severity": "<critical|high|medium|low|info>",
      "title": "<short title>",
      "description": "<detailed explanation>",
      "file_path": "<optional>",
      "line": <optional>,
      "suggestion": "<actionable fix>"
    }
  ],
  "strengths": ["<top positive aspects across all agents>"],
  "suggestions": ["<top prioritized improvement suggestions>"]
}

Sort findings by severity (critical first). Limit to the 20 most important findings.
Weight correctness bugs, security issues, and risky dependency changes heavily in the overall score.`

type Aggregator struct {
	client *llm.Client
	log    *zap.Logger
}

func NewAggregator(client *llm.Client, log *zap.Logger) *Aggregator {
	return &Aggregator{
		client: client,
		log:    log.Named("aggregator"),
	}
}

type aggregatedResponse struct {
	Quality     models.QualityScore `json:"quality"`
	Findings    []models.Finding    `json:"findings"`
	Strengths   []string            `json:"strengths"`
	Suggestions []string            `json:"suggestions"`
}

func (a *Aggregator) Aggregate(
	ctx context.Context,
	input ReviewInput,
	outputs []*AgentOutput,
) (*models.ReviewResult, error) {
	a.log.Info("aggregating agent results", zap.Int("agent_count", len(outputs)))

	if len(outputs) == 0 {
		return nil, fmt.Errorf("no agent outputs to aggregate")
	}

	// Rule-based pre-merge as fallback context for LLM
	premerged := ruleBasedMerge(outputs)

	userPrompt := buildAggregatorPrompt(input, outputs, premerged)

	var raw aggregatedResponse
	if err := a.client.ChatJSON(ctx, "aggregator", aggregatorSystemPrompt, userPrompt, &raw); err != nil {
		a.log.Warn("LLM aggregation failed, using rule-based merge", zap.Error(err))
		return ruleBasedResult(input, premerged, outputs), nil
	}

	result := &models.ReviewResult{
		ID:           uuid.New().String(),
		Source:       input.Source,
		Language:     input.Language,
		Quality:      raw.Quality,
		Findings:     orEmptyFindings(raw.Findings),
		Strengths:    orEmptyStrings(raw.Strengths),
		Suggestions:  orEmptyStrings(raw.Suggestions),
		AgentReports: toAgentReports(outputs),
		ReviewedAt:   time.Now().UTC(),
	}

	a.log.Info("aggregation completed",
		zap.String("review_id", result.ID),
		zap.Int("overall_score", result.Quality.Overall),
		zap.Int("findings", len(result.Findings)),
	)
	return result, nil
}

func buildAggregatorPrompt(input ReviewInput, outputs []*AgentOutput, premerged *mergedState) string {
	var b strings.Builder
	b.WriteString("Merge the following specialist agent reports into a unified code review.\n\n")

	if input.IsDiffReview() {
		b.WriteString(fmt.Sprintf("Source: PR %s #%d\n\n", input.RepoFullName, input.PRNumber))
	} else {
		b.WriteString(fmt.Sprintf("Source: manual review (%s, file: %s)\n\n", input.Language, input.FilePath))
	}

	for _, out := range outputs {
		data, _ := json.MarshalIndent(out, "", "  ")
		b.WriteString(fmt.Sprintf("--- %s agent (score: %d) ---\n%s\n\n", out.Agent, out.Score, string(data)))
	}

	b.WriteString(fmt.Sprintf("Pre-merged stats: %d findings, %d strengths, %d suggestions\n",
		len(premerged.findings), len(premerged.strengths), len(premerged.suggestions)))

	return b.String()
}

type mergedState struct {
	findings    []models.Finding
	strengths   []string
	suggestions []string
	scores      map[string]int
}

func ruleBasedMerge(outputs []*AgentOutput) *mergedState {
	state := &mergedState{
		findings:    []models.Finding{},
		strengths:   []string{},
		suggestions: []string{},
		scores:      map[string]int{},
	}

	seenIdx := map[string]int{}
	for _, out := range outputs {
		state.scores[string(out.Agent)] = out.Score
		for _, f := range out.Findings {
			key := strings.ToLower(f.Title + "|" + f.FilePath)
			if idx, ok := seenIdx[key]; ok {
				if findingSeverityRank(f.Severity) < findingSeverityRank(state.findings[idx].Severity) {
					state.findings[idx] = f
				}
				continue
			}
			seenIdx[key] = len(state.findings)
			state.findings = append(state.findings, f)
		}
		for _, s := range out.Strengths {
			state.strengths = appendUnique(state.strengths, s)
		}
		for _, s := range out.Suggestions {
			state.suggestions = appendUnique(state.suggestions, s)
		}
	}
	return state
}

func findingSeverityRank(s models.Severity) int {
	switch s {
	case models.SeverityCritical:
		return 0
	case models.SeverityHigh:
		return 1
	case models.SeverityMedium:
		return 2
	case models.SeverityLow:
		return 3
	default:
		return 4
	}
}

func ruleBasedResult(input ReviewInput, merged *mergedState, outputs []*AgentOutput) *models.ReviewResult {
	security := merged.scores[string(AgentSecurity)]
	quality := merged.scores[string(AgentQuality)]
	performance := merged.scores[string(AgentPerformance)]
	style := merged.scores[string(AgentStyle)]
	bug := merged.scores[string(AgentBug)]

	overall := averageScore(merged.scores)
	// Correctness bugs pull overall down when present.
	if bug > 0 {
		overall = averageOf(overall, bug)
	}
	maintainability := averageOf(quality, style)
	readability := style
	if readability == 0 {
		readability = quality
	}

	return &models.ReviewResult{
		ID:           uuid.New().String(),
		Source:       input.Source,
		Language:     input.Language,
		Quality: models.QualityScore{
			Overall:         overall,
			Maintainability: maintainability,
			Readability:     readability,
			Security:        security,
			Performance:     performance,
			Summary:         "Review completed by multi-agent system (rule-based aggregation).",
		},
		Findings:     merged.findings,
		Strengths:    merged.strengths,
		Suggestions:  merged.suggestions,
		AgentReports: toAgentReports(outputs),
		ReviewedAt:   time.Now().UTC(),
	}
}

func toAgentReports(outputs []*AgentOutput) []models.AgentReport {
	reports := make([]models.AgentReport, 0, len(outputs))
	for _, o := range outputs {
		reports = append(reports, models.AgentReport{
			Agent:         string(o.Agent),
			Score:         o.Score,
			Summary:       o.Summary,
			FindingsCount: len(o.Findings),
		})
	}
	return reports
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func averageScore(scores map[string]int) int {
	if len(scores) == 0 {
		return 0
	}
	total := 0
	for _, v := range scores {
		total += v
	}
	return total / len(scores)
}

func averageOf(a, b int) int {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	return (a + b) / 2
}
