package agents

import (
	"context"

	"go.uber.org/zap"

	"codereviewagent/internal/llm"
	"codereviewagent/internal/tools"
)

// SecurityAgent runs static analysis tools then LLM security review.
type SecurityAgent struct {
	base  *BaseAgent
	tools *tools.Runner
	log   *zap.Logger
}

func NewSecurityAgent(client *llm.Client, toolRunner *tools.Runner, log *zap.Logger) Agent {
	return &SecurityAgent{
		base: newBaseAgent(
			AgentSecurity,
			"security",
			`Identify security vulnerabilities including OWASP Top 10 issues, injection flaws, hardcoded secrets,
authentication/authorization problems, insecure cryptography, SSRF, XSS, CSRF, and unsafe deserialization.
Incorporate static analysis tool findings into your review.`,
			client,
			log,
		),
		tools: toolRunner,
		log:   log.Named("security"),
	}
}

func (a *SecurityAgent) Name() AgentName {
	return AgentSecurity
}

func (a *SecurityAgent) Analyze(ctx context.Context, input ReviewInput) (*AgentOutput, error) {
	scanInput := input
	if a.tools != nil {
		toolFindings := a.tools.Scan(ctx, input.FilesAsMap())
		scanInput.ToolFindings = append(scanInput.ToolFindings, toolFindings...)
		a.log.Info("security tools scanned", zap.Int("tool_findings", len(toolFindings)))
	}

	output, err := a.base.Analyze(ctx, scanInput)
	if err != nil {
		return nil, err
	}

	if len(scanInput.ToolFindings) > 0 {
		toolModels := toolFindingsToModel(scanInput.ToolFindings)
		output.Findings = append(toolModels, output.Findings...)
		if output.Score > 40 && len(toolModels) > 0 {
			output.Score = 40
		}
	}
	return output, nil
}
