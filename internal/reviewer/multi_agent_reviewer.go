package reviewer

import (
	"context"
	"fmt"

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
	}
	if len(files) == 1 {
		input.Language = files[0].Language
		input.FilePath = files[0].Path
	}
	r.log.Info("reviewing files", zap.String("source", source), zap.Int("files", len(files)))
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
