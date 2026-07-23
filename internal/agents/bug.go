package agents

import (
	"codereviewagent/internal/llm"

	"go.uber.org/zap"
)

// NewBugDetectionAgent finds correctness and logic bugs (not security/style/perf).
func NewBugDetectionAgent(client *llm.Client, log *zap.Logger) Agent {
	return newBaseAgent(
		AgentBug,
		"bug",
		`Detect correctness and logic bugs that can cause wrong behavior or runtime failures.

Focus ONLY on:
- Incorrect conditions, inverted logic, wrong operators
- Unhandled or incorrectly handled errors / exceptions
- Nil/null dereference, use-after-close, dangling references
- Off-by-one errors, empty collection assumptions, bounds mistakes
- Race conditions and unsafe shared mutable state
- Broken API contracts between caller and callee
- Silent failures, swallowed errors, wrong return values
- State machine / invariant violations
- Resource leaks that cause functional failure (not just performance)

Use the Context Agent briefing (when present) to prioritize bugs that break the stated PR intent.

Do NOT report:
- Security vulnerabilities (Security agent)
- Style, naming, formatting (Style agent)
- Pure performance / scalability issues (Performance agent)
- Missing tests (Test agent)
- General maintainability / refactoring advice (Quality agent)

Prefer concrete, reproducible bug hypotheses with file/line when possible.`,
		client,
		log,
	)
}
