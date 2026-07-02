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

// Orchestrator runs specialist agents in parallel and aggregates their results.
type Orchestrator struct {
	agents        []agents.Agent
	aggregator    *agents.Aggregator
	maxChunkBytes int
	log           *zap.Logger
}

func New(agentList []agents.Agent, aggregator *agents.Aggregator, maxChunkBytes int, log *zap.Logger) *Orchestrator {
	if maxChunkBytes <= 0 {
		maxChunkBytes = 50000
	}
	return &Orchestrator{
		agents:        agentList,
		aggregator:    aggregator,
		maxChunkBytes: maxChunkBytes,
		log:           log.Named("orchestrator"),
	}
}

func (o *Orchestrator) RunReview(ctx context.Context, input agents.ReviewInput) (*models.ReviewResult, error) {
	chunks := o.buildChunks(input)
	groups := chunker.Group(chunks, o.maxChunkBytes)

	o.log.Info("starting multi-agent review",
		zap.String("source", input.Source),
		zap.Int("agent_count", len(o.agents)),
		zap.Int("chunks", len(chunks)),
		zap.Int("chunk_groups", len(groups)),
	)

	if len(groups) <= 1 {
		outputs, err := o.runAgents(ctx, input)
		if err != nil {
			return nil, err
		}
		return o.aggregator.Aggregate(ctx, input, outputs)
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
	return o.aggregator.Aggregate(ctx, input, merged)
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
