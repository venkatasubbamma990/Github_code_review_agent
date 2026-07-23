package reviewer

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"codereviewagent/internal/agents"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/llm"
	"codereviewagent/internal/models"
	"codereviewagent/internal/orchestrator"
	"codereviewagent/internal/tools"
)

// MultiAgentReviewer coordinates specialist agents via the orchestrator.
type MultiAgentReviewer struct {
	orchestrator *orchestrator.Orchestrator
	log          *zap.Logger
}

func NewMultiAgentReviewer(
	apiKey, baseURL, model string,
	useJSONMode bool,
	maxChunkBytes int,
	gosecPath, semgrepPath string,
	log *zap.Logger,
) *MultiAgentReviewer {
	client := llm.NewClient(apiKey, baseURL, model, useJSONMode, log)
	toolRunner := tools.NewRunner(gosecPath, semgrepPath, log)
	log.Info("static analysis tools",
		zap.String("gosec_path", gosecPath),
		zap.Bool("gosec_available", toolRunner.GosecAvailable()),
		zap.String("semgrep_path", semgrepPath),
		zap.Bool("semgrep_available", toolRunner.SemgrepAvailable()),
	)
	agentList := agents.NewDefaultAgents(client, toolRunner, log)
	aggregator := agents.NewAggregator(client, log)
	orch := orchestrator.New(agentList, aggregator, maxChunkBytes, log)

	return &MultiAgentReviewer{
		orchestrator: orch,
		log:          log.Named("multi_agent_reviewer"),
	}
}

func (r *MultiAgentReviewer) ReviewCode(ctx context.Context, code, language, filePath, extraContext string) (*models.ReviewResult, error) {
	input := agents.ReviewInput{
		Code:         code,
		Language:     language,
		FilePath:     filePath,
		ExtraContext: extraContext,
		Source:       "manual",
	}
	r.log.Info("reviewing code", zap.String("language", language))
	return r.orchestrator.RunReview(ctx, input)
}

func (r *MultiAgentReviewer) ReviewDiff(ctx context.Context, diff, repoFullName string, prNumber int) (*models.ReviewResult, error) {
	input := agents.ReviewInput{
		Diff:         diff,
		RepoFullName: repoFullName,
		PRNumber:     prNumber,
		Source:       fmt.Sprintf("github:%s#%d", repoFullName, prNumber),
	}
	r.log.Info("reviewing PR diff", zap.String("repo", repoFullName), zap.Int("pr", prNumber))
	return r.orchestrator.RunReview(ctx, input)
}

func (r *MultiAgentReviewer) ReviewFiles(ctx context.Context, source string, repoFullName string, files []ghclient.SourceFile) (*models.ReviewResult, error) {
	input := agents.ReviewInput{
		Source:       source,
		RepoFullName: repoFullName,
		Files:        toAgentFiles(files),
		Diff:         buildCombinedDiff(files),
	}
	if prNumber := parsePRNumber(source); prNumber > 0 {
		input.PRNumber = prNumber
	}
	if len(files) == 1 {
		input.Language = files[0].Language
		input.FilePath = files[0].Path
	}
	r.log.Info("reviewing files",
		zap.String("source", source),
		zap.Int("files", len(files)),
		zap.Bool("has_diff", input.Diff != ""),
	)
	return r.orchestrator.RunReview(ctx, input)
}

func toAgentFiles(files []ghclient.SourceFile) []agents.SourceFile {
	result := make([]agents.SourceFile, len(files))
	for i, f := range files {
		result[i] = agents.SourceFile{
			Path:     f.Path,
			Language: f.Language,
			Content:  f.Content,
		}
	}
	return result
}

// buildCombinedDiff joins per-file patches so agents see both full files and the change set.
func buildCombinedDiff(files []ghclient.SourceFile) string {
	var b strings.Builder
	for _, f := range files {
		patch := f.Patch
		if patch == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("--- %s\n", f.Path))
		b.WriteString(patch)
		if !strings.HasSuffix(patch, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func parsePRNumber(source string) int {
	// source format: github:owner/repo#123
	idx := strings.LastIndex(source, "#")
	if idx < 0 || idx+1 >= len(source) {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(source[idx+1:], "%d", &n)
	return n
}
