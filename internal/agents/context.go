package agents

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"codereviewagent/internal/llm"
)

const contextSystemPrompt = `You are the Context Agent in a multi-agent code review system.
You do NOT review code for bugs. Your job is to understand WHY this change exists and what specialists should focus on.

Given PR metadata (title, description, labels, linked issues, changed files, discussion), return ONLY valid JSON:
{
  "intent": "<1-3 sentence summary of what this PR aims to do>",
  "risk_areas": ["<areas that need careful review>"],
  "constraints": ["<explicit requirements, acceptance criteria, or non-goals>"],
  "linked_issues": ["<issue refs or short descriptions>"],
  "focus_hints": ["<concrete hints for security/quality/performance/test agents>"],
  "summary": "<short briefing for downstream specialist agents>"
}

Be concise and factual. If information is missing, say so briefly instead of inventing requirements.`

// ContextBrief is the structured briefing produced for specialist agents.
type ContextBrief struct {
	Intent       string   `json:"intent"`
	RiskAreas    []string `json:"risk_areas"`
	Constraints  []string `json:"constraints"`
	LinkedIssues []string `json:"linked_issues"`
	FocusHints   []string `json:"focus_hints"`
	Summary      string   `json:"summary"`
}

// ContextAgent gathers PR intent and produces a briefing for specialists.
type ContextAgent struct {
	client *llm.Client
	log    *zap.Logger
}

func NewContextAgent(client *llm.Client, log *zap.Logger) *ContextAgent {
	return &ContextAgent{
		client: client,
		log:    log.Named("context"),
	}
}

func (a *ContextAgent) Name() AgentName {
	return AgentContext
}

// Analyze implements Agent for interface compatibility, but ContextAgent is
// normally invoked via BuildBrief as a pre-pass (not as a findings producer).
func (a *ContextAgent) Analyze(ctx context.Context, input ReviewInput) (*AgentOutput, error) {
	brief, err := a.BuildBrief(ctx, input)
	if err != nil {
		return nil, err
	}
	return &AgentOutput{
		Agent:   AgentContext,
		Score:   100,
		Summary: brief.Summary,
		Findings: nil,
		Strengths: orEmptyStrings(nil),
		Suggestions: brief.FocusHints,
	}, nil
}

// BuildBrief synthesizes a ContextBrief from PR metadata and optional file list.
func (a *ContextAgent) BuildBrief(ctx context.Context, input ReviewInput) (*ContextBrief, error) {
	a.log.Info("context agent started",
		zap.String("source", input.Source),
		zap.Int("pr", input.PRNumber),
	)

	userPrompt := buildContextPrompt(input)
	var brief ContextBrief
	if err := a.client.ChatJSON(ctx, string(AgentContext), contextSystemPrompt, userPrompt, &brief); err != nil {
		a.log.Warn("context LLM failed; using raw metadata fallback", zap.Error(err))
		return fallbackBrief(input), nil
	}

	if strings.TrimSpace(brief.Summary) == "" {
		brief.Summary = brief.Intent
	}
	brief.RiskAreas = orEmptyStrings(brief.RiskAreas)
	brief.Constraints = orEmptyStrings(brief.Constraints)
	brief.LinkedIssues = orEmptyStrings(brief.LinkedIssues)
	brief.FocusHints = orEmptyStrings(brief.FocusHints)

	a.log.Info("context agent completed",
		zap.String("intent", truncateForLog(brief.Intent, 120)),
		zap.Int("risk_areas", len(brief.RiskAreas)),
		zap.Int("focus_hints", len(brief.FocusHints)),
	)
	return &brief, nil
}

func buildContextPrompt(input ReviewInput) string {
	var b strings.Builder
	b.WriteString("Build a review briefing from the following change context.\n\n")

	if input.PRContext != "" {
		b.WriteString("## Pull request metadata\n")
		b.WriteString(llm.Truncate(input.PRContext, 12000))
		b.WriteString("\n\n")
	} else if input.ExtraContext != "" {
		b.WriteString("## Provided context\n")
		b.WriteString(llm.Truncate(input.ExtraContext, 8000))
		b.WriteString("\n\n")
	} else {
		b.WriteString("No PR description was provided. Infer intent only from changed files / diff hints.\n\n")
	}

	if input.RepoFullName != "" {
		b.WriteString(fmt.Sprintf("Repository: %s\n", input.RepoFullName))
	}
	if input.PRNumber > 0 {
		b.WriteString(fmt.Sprintf("PR number: #%d\n", input.PRNumber))
	}

	if len(input.Files) > 0 {
		b.WriteString("\nChanged / reviewed files:\n")
		for i, f := range input.Files {
			if i >= 50 {
				b.WriteString(fmt.Sprintf("- ...and %d more\n", len(input.Files)-50))
				break
			}
			b.WriteString(fmt.Sprintf("- %s (%s)\n", f.Path, f.Language))
		}
	}

	if input.Diff != "" {
		b.WriteString("\nDiff excerpt (for intent only, not a full code review):\n")
		b.WriteString("```diff\n")
		b.WriteString(llm.Truncate(input.Diff, 6000))
		b.WriteString("\n```\n")
	}

	return b.String()
}

func fallbackBrief(input ReviewInput) *ContextBrief {
	summary := "Limited context available; specialists should review the changed files carefully."
	if input.PRContext != "" {
		summary = "Using raw PR metadata (LLM context synthesis unavailable)."
	}
	return &ContextBrief{
		Intent:       summary,
		RiskAreas:    []string{},
		Constraints:  []string{},
		LinkedIssues: []string{},
		FocusHints:   []string{"Review changed files against the PR description when available."},
		Summary:      summary,
	}
}

// FormatBriefForAgents renders a briefing block for specialist prompts.
func FormatBriefForAgents(brief *ContextBrief) string {
	if brief == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Context Agent briefing (use this to prioritize your review)\n")
	if brief.Intent != "" {
		b.WriteString("Intent: " + brief.Intent + "\n")
	}
	if brief.Summary != "" && brief.Summary != brief.Intent {
		b.WriteString("Summary: " + brief.Summary + "\n")
	}
	writeList(&b, "Risk areas", brief.RiskAreas)
	writeList(&b, "Constraints", brief.Constraints)
	writeList(&b, "Linked issues", brief.LinkedIssues)
	writeList(&b, "Focus hints", brief.FocusHints)
	return b.String()
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(title + ":\n")
	for _, item := range items {
		b.WriteString("- " + item + "\n")
	}
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
