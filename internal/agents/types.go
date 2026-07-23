package agents

import (
	"context"

	"codereviewagent/internal/models"
	"codereviewagent/internal/tools"
)

// AgentName identifies a specialist reviewer agent.
type AgentName string

const (
	AgentContext     AgentName = "context"
	AgentSecurity    AgentName = "security"
	AgentBug         AgentName = "bug"
	AgentDependency  AgentName = "dependency"
	AgentQuality     AgentName = "quality"
	AgentPerformance AgentName = "performance"
	AgentStyle       AgentName = "style"
	AgentTest        AgentName = "test"
	AgentAggregator  AgentName = "aggregator"
)

// SourceFile is a single file included in a review.
type SourceFile struct {
	Path     string
	Language string
	Content  string
}

// ReviewInput is the shared context passed to every specialist agent.
type ReviewInput struct {
	Code         string
	Language     string
	FilePath     string
	ExtraContext string
	// PRContext is raw GitHub PR metadata (title, body, labels, comments).
	PRContext string
	// ContextBrief is the Context Agent output injected for specialists.
	ContextBrief string
	Source       string
	Diff         string
	RepoFullName string
	PRNumber     int
	Files        []SourceFile
	ToolFindings []tools.Finding
	ChunkIndex   int
	TotalChunks  int
}

// IsDiffReview returns true when reviewing a pull request diff.
func (r ReviewInput) IsDiffReview() bool {
	return r.Diff != ""
}

// HasMultipleFiles returns true when reviewing multiple files/chunks.
func (r ReviewInput) HasMultipleFiles() bool {
	return len(r.Files) > 1
}

// FilesAsMap returns files as a path→content map for tool scanners.
func (r ReviewInput) FilesAsMap() map[string][]byte {
	files := make(map[string][]byte)
	for _, f := range r.Files {
		files[f.Path] = []byte(f.Content)
	}
	if len(files) == 0 && r.Code != "" {
		path := r.FilePath
		if path == "" {
			path = "main." + r.Language
		}
		files[path] = []byte(r.Code)
	}
	return files
}

// AgentOutput is the structured result from a single specialist agent.
type AgentOutput struct {
	Agent       AgentName         `json:"agent"`
	Score       int               `json:"score"`
	Summary     string            `json:"summary"`
	Findings    []models.Finding  `json:"findings"`
	Strengths   []string          `json:"strengths"`
	Suggestions []string          `json:"suggestions"`
}

// Agent analyzes code and returns domain-specific review output.
type Agent interface {
	Name() AgentName
	Analyze(ctx context.Context, input ReviewInput) (*AgentOutput, error)
}

// rawAgentResponse is the JSON schema each specialist LLM returns.
type rawAgentResponse struct {
	Score       int              `json:"score"`
	Summary     string           `json:"summary"`
	Findings    []models.Finding `json:"findings"`
	Strengths   []string         `json:"strengths"`
	Suggestions []string         `json:"suggestions"`
}
