package agents

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"codereviewagent/internal/llm"
	"codereviewagent/internal/models"
	"codereviewagent/internal/tools"
)

const findingsJSONSchema = `{
  "score": <0-100>,
  "summary": "<brief assessment for your domain>",
  "findings": [
    {
      "category": "<your-domain-category>",
      "severity": "<critical|high|medium|low|info>",
      "title": "<short title>",
      "description": "<detailed explanation>",
      "file_path": "<optional>",
      "line": <optional line number or 0>,
      "suggestion": "<actionable fix>"
    }
  ],
  "strengths": ["<positive aspects in your domain>"],
  "suggestions": ["<improvement suggestions in your domain>"]
}`

type BaseAgent struct {
	name         AgentName
	category     string
	systemPrompt string
	client       *llm.Client
	log          *zap.Logger
}

func newBaseAgent(name AgentName, category, roleDescription string, client *llm.Client, log *zap.Logger) *BaseAgent {
	return &BaseAgent{
		name:     name,
		category: category,
		systemPrompt: fmt.Sprintf(`You are a specialist %s agent in a multi-agent code review system.
Your role: %s

Analyze ONLY your domain. Return ONLY valid JSON matching this schema:
%s

Be specific, actionable, and prioritize critical issues. If no issues found, return an empty findings array and a high score.`,
			name, roleDescription, findingsJSONSchema),
		client: client,
		log:    log.Named(string(name)),
	}
}

func (a *BaseAgent) Name() AgentName {
	return a.name
}

func (a *BaseAgent) Analyze(ctx context.Context, input ReviewInput) (*AgentOutput, error) {
	userPrompt := buildUserPrompt(input)
	a.log.Info("agent started",
		zap.String("source", input.Source),
		zap.Int("chunk", input.ChunkIndex),
	)

	var raw rawAgentResponse
	if err := a.client.ChatJSON(ctx, string(a.name), a.systemPrompt, userPrompt, &raw); err != nil {
		a.log.Error("agent failed", zap.Error(err))
		return nil, err
	}

	normalizeFindings(raw.Findings, a.category)
	output := &AgentOutput{
		Agent:       a.name,
		Score:       clampScore(raw.Score),
		Summary:     raw.Summary,
		Findings:    orEmptyFindings(raw.Findings),
		Strengths:   orEmptyStrings(raw.Strengths),
		Suggestions: orEmptyStrings(raw.Suggestions),
	}

	a.log.Info("agent completed",
		zap.Int("score", output.Score),
		zap.Int("findings", len(output.Findings)),
	)
	return output, nil
}

func buildUserPrompt(input ReviewInput) string {
	var b strings.Builder

	if input.TotalChunks > 1 {
		b.WriteString(fmt.Sprintf("Review chunk %d of %d.\n\n", input.ChunkIndex+1, input.TotalChunks))
	}

	if input.ContextBrief != "" {
		b.WriteString(input.ContextBrief)
		b.WriteString("\n")
	}
	if input.ExtraContext != "" {
		b.WriteString(fmt.Sprintf("Additional context:\n%s\n\n", input.ExtraContext))
	}

	appendToolFindings(&b, input.ToolFindings)

	if len(input.Files) > 0 {
		b.WriteString("Review the following files:\n\n")
		for _, f := range input.Files {
			b.WriteString(fmt.Sprintf("### File: %s (%s)\n", f.Path, f.Language))
			b.WriteString("```")
			b.WriteString(f.Language)
			b.WriteString("\n")
			b.WriteString(llm.Truncate(f.Content, 40000))
			b.WriteString("\n```\n\n")
		}
		if input.Diff != "" {
			b.WriteString("### Associated diff\n```diff\n")
			b.WriteString(llm.Truncate(input.Diff, 20000))
			b.WriteString("\n```\n")
		}
		return b.String()
	}

	if input.IsDiffReview() {
		repo := input.RepoFullName
		if repo == "" {
			repo = "unknown"
		}
		b.WriteString(fmt.Sprintf(`Review this pull request diff from a specialist agent perspective.

Repository: %s
PR: #%d
Source: %s

Diff:
%s`, repo, input.PRNumber, input.Source, llm.Truncate(input.Diff, 80000)))
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Review the following %s code", input.Language))
	if input.FilePath != "" {
		b.WriteString(fmt.Sprintf(" from file: %s", input.FilePath))
	}
	b.WriteString(".\n\n")
	b.WriteString("```")
	b.WriteString(input.Language)
	b.WriteString("\n")
	b.WriteString(llm.Truncate(input.Code, 80000))
	b.WriteString("\n```")
	return b.String()
}

func appendToolFindings(b *strings.Builder, findings []tools.Finding) {
	if len(findings) == 0 {
		return
	}
	b.WriteString("## Static analysis tool results (factual — prioritize these):\n")
	for _, f := range findings {
		b.WriteString(fmt.Sprintf("- [%s/%s] %s (%s", f.Tool, f.Severity, f.Title, f.FilePath))
		if f.Line > 0 {
			b.WriteString(fmt.Sprintf(":%d", f.Line))
		}
		b.WriteString(fmt.Sprintf("): %s\n", f.Description))
	}
	b.WriteString("\n")
}

func toolFindingsToModel(findings []tools.Finding) []models.Finding {
	result := make([]models.Finding, 0, len(findings))
	for _, f := range findings {
		result = append(result, models.Finding{
			Category:    "security",
			Severity:    models.Severity(f.Severity),
			Title:       fmt.Sprintf("[%s] %s", f.Tool, f.Title),
			Description: f.Description,
			FilePath:    f.FilePath,
			Line:        f.Line,
			Suggestion:  "Fix the issue reported by " + f.Tool,
		})
	}
	return result
}

func normalizeFindings(findings []models.Finding, defaultCategory string) {
	for i := range findings {
		if findings[i].Category == "" {
			findings[i].Category = defaultCategory
		}
	}
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func orEmptyFindings(f []models.Finding) []models.Finding {
	if f == nil {
		return []models.Finding{}
	}
	return f
}

func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// MergeOutputs combines outputs from multiple chunks by agent name.
func MergeOutputs(outputs []*AgentOutput) []*AgentOutput {
	byAgent := map[AgentName]*AgentOutput{}
	for _, out := range outputs {
		existing, ok := byAgent[out.Agent]
		if !ok {
			copy := *out
			byAgent[out.Agent] = &copy
			continue
		}
		existing.Findings = append(existing.Findings, out.Findings...)
		existing.Strengths = appendUniqueStrings(existing.Strengths, out.Strengths...)
		existing.Suggestions = appendUniqueStrings(existing.Suggestions, out.Suggestions...)
		if out.Score < existing.Score {
			existing.Score = out.Score
		}
		if out.Summary != "" {
			existing.Summary = existing.Summary + " " + out.Summary
		}
	}

	merged := make([]*AgentOutput, 0, len(byAgent))
	for _, out := range byAgent {
		merged = append(merged, out)
	}
	return merged
}

func appendUniqueStrings(base []string, items ...string) []string {
	for _, item := range items {
		found := false
		for _, b := range base {
			if b == item {
				found = true
				break
			}
		}
		if !found {
			base = append(base, item)
		}
	}
	return base
}
