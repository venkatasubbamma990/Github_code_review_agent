package agents

import (
	"codereviewagent/internal/llm"
	"codereviewagent/internal/tools"

	"go.uber.org/zap"
)

func NewQualityAgent(client *llm.Client, log *zap.Logger) Agent {
	return newBaseAgent(
		AgentQuality,
		"maintainability",
		`Evaluate code quality: complexity, readability, error handling, edge cases, coupling, cohesion,
naming, function length, and long-term maintainability.`,
		client,
		log,
	)
}

func NewPerformanceAgent(client *llm.Client, log *zap.Logger) Agent {
	return newBaseAgent(
		AgentPerformance,
		"performance",
		`Find performance issues: inefficient algorithms, unnecessary allocations, N+1 queries, blocking I/O,
missing caching, hot loops, and scalability concerns.`,
		client,
		log,
	)
}

func NewStyleAgent(client *llm.Client, log *zap.Logger) Agent {
	return newBaseAgent(
		AgentStyle,
		"style",
		`Review code style and conventions: formatting consistency, idiomatic patterns for the language,
documentation quality, naming conventions, and adherence to best practices.`,
		client,
		log,
	)
}

func NewTestAgent(client *llm.Client, log *zap.Logger) Agent {
	return newBaseAgent(
		AgentTest,
		"testing",
		`Assess test coverage and quality: missing unit tests, weak assertions, untested edge cases,
missing integration tests, and test anti-patterns.`,
		client,
		log,
	)
}

// NewDefaultAgents builds all specialist agents.
func NewDefaultAgents(client *llm.Client, toolRunner *tools.Runner, log *zap.Logger) []Agent {
	return []Agent{
		NewSecurityAgent(client, toolRunner, log),
		NewQualityAgent(client, log),
		NewPerformanceAgent(client, log),
		NewStyleAgent(client, log),
		NewTestAgent(client, log),
	}
}
