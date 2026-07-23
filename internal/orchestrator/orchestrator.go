package orchestrator

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"codereviewagent/internal/agents"
	"codereviewagent/internal/chunker"
	"codereviewagent/internal/models"
)

// Orchestrator runs the Context Agent as a pre-pass, then specialist agents in
// parallel, and finally aggregates their results.
type Orchestrator struct {
	contextAgent  *agents.ContextAgent
	agents        []agents.Agent
	aggregator    *agents.Aggregator
	maxChunkBytes int
	log           *zap.Logger
}

func New(
	contextAgent *agents.ContextAgent,
	agentList []agents.Agent,
	aggregator *agents.Aggregator,
	maxChunkBytes int,
	log *zap.Logger,
) *Orchestrator {
	if maxChunkBytes <= 0 {
		maxChunkBytes = 50000
	}
	return &Orchestrator{
		contextAgent:  contextAgent,
		agents:        agentList,
		aggregator:    aggregator,
		maxChunkBytes: maxChunkBytes,
		log:           log.Named("orchestrator"),
	}
}

func (o *Orchestrator) RunReview(ctx context.Context, input agents.ReviewInput) (*models.ReviewResult, error) {
	input = o.enrichWithContext(ctx, input)

	chunks := o.buildChunks(input)
	groups := chunker.Group(chunks, o.maxChunkBytes)

	o.log.Info("starting multi-agent review",
		zap.String("source", input.Source),
		zap.Int("agent_count", len(o.agents)),
		zap.Int("chunks", len(chunks)),
		zap.Int("chunk_groups", len(groups)),
		zap.Bool("has_context_brief", input.ContextBrief != ""),
	)

	if len(groups) <= 1 {
		outputs, err := o.runAgents(ctx, input)
		if err != nil {
			return nil, err
		}
		result, err := o.aggregator.Aggregate(ctx, input, outputs)
		if err != nil {
			return nil, err
		}
		return attachContextReport(result, input), nil
	}

	var allOutputs []*agents.AgentOutput
	for i, group := range groups {
		chunkInput := chunker.ToReviewInput(input, group, i, len(groups))
		outputs, err := o.runAgents(ctx, chunkInput)
		if err != nil {
			o.log.Warn("chunk review failed", zap.Int("chunk", i), zap.Error(err))
			continue
		}
		allOutputs = append(allOutputs, outputs...)
	}

	if len(allOutputs) == 0 {
		return nil, fmt.Errorf("all chunk reviews failed")
	}

	merged := agents.MergeOutputs(allOutputs)
	result, err := o.aggregator.Aggregate(ctx, input, merged)
	if err != nil {
		return nil, err
	}
	return attachContextReport(result, input), nil
}

func (o *Orchestrator) enrichWithContext(ctx context.Context, input agents.ReviewInput) agents.ReviewInput {
	if o.contextAgent == nil {
		return input
	}
	// Skip when there is nothing for the context agent to work with.
	if input.PRContext == "" && input.ExtraContext == "" && input.PRNumber == 0 && len(input.Files) == 0 {
		return input
	}

	brief, err := o.contextAgent.BuildBrief(ctx, input)
	if err != nil {
		o.log.Warn("context agent failed; continuing without briefing", zap.Error(err))
		return input
	}
	input.ContextBrief = agents.FormatBriefForAgents(brief)
	return input
}

func attachContextReport(result *models.ReviewResult, input agents.ReviewInput) *models.ReviewResult {
	if result == nil || input.ContextBrief == "" {
		return result
	}
	summary := input.ContextBrief
	if len(summary) > 300 {
		summary = summary[:297] + "..."
	}
	result.AgentReports = append([]models.AgentReport{{
		Agent:         string(agents.AgentContext),
		Score:         100,
		Summary:       summary,
		FindingsCount: 0,
	}}, result.AgentReports...)
	return result
}

func (o *Orchestrator) buildChunks(input agents.ReviewInput) []chunker.FileChunk {
	if len(input.Files) > 0 {
		sourceFiles := make([]agents.SourceFile, len(input.Files))
		copy(sourceFiles, input.Files)
		return chunker.FromSourceFiles(sourceFiles)
	}
	if input.IsDiffReview() {
		return chunker.FromDiff(input.Diff)
	}
	if input.Code != "" {
		return []chunker.FileChunk{{
			FilePath: input.FilePath,
			Language: input.Language,
			Content:  input.Code,
		}}
	}
	return nil
}

func (o *Orchestrator) runAgents(ctx context.Context, input agents.ReviewInput) ([]*agents.AgentOutput, error) {
	var (
		mu      sync.Mutex
		outputs []*agents.AgentOutput
	)

	g, gctx := errgroup.WithContext(ctx)
	for _, agent := range o.agents {
		a := agent
		g.Go(func() error {
			out, err := a.Analyze(gctx, input)
			if err != nil {
				o.log.Error("agent failed",
					zap.String("agent", string(a.Name())),
					zap.Error(err),
				)
				return nil
			}
			mu.Lock()
			outputs = append(outputs, out)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	if len(outputs) == 0 {
		return nil, fmt.Errorf("all agents failed to produce results")
	}

	o.log.Info("specialist agents completed", zap.Int("successful", len(outputs)))
	return outputs, nil
}
