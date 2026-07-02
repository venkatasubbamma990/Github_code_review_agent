package models

import (
	"encoding/json"
	"time"
)

// ReviewRequest is used for manual code review submissions.
type ReviewRequest struct {
	Code     string `json:"code" binding:"required"`
	Language string `json:"language" binding:"required"`
	FilePath string `json:"file_path"`
	Context  string `json:"context"`
}

// PRReviewRequest triggers a review for a specific GitHub pull request.
type PRReviewRequest struct {
	Owner string `json:"owner" binding:"required"`
	Repo  string `json:"repo" binding:"required"`
	PR    int    `json:"pr" binding:"required,min=1"`
}

// RepoReviewRequest triggers a review for an entire GitHub repository.
type RepoReviewRequest struct {
	URL      string `json:"url" binding:"required"`
	Branch   string `json:"branch"`
	MaxFiles int    `json:"max_files"`
	Async    bool   `json:"async"`
}

// JobStatusResponse represents async job status.
type JobStatusResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	State   string          `json:"state"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ReviewJobAccepted is returned when a review is queued.
type ReviewJobAccepted struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
	Repo    string `json:"repo,omitempty"`
}

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Finding struct {
	Category    string   `json:"category"`
	Severity    Severity `json:"severity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	FilePath    string   `json:"file_path,omitempty"`
	Line        int      `json:"line,omitempty"`
	Suggestion  string   `json:"suggestion,omitempty"`
}

type QualityScore struct {
	Overall      int    `json:"overall"`
	Maintainability int `json:"maintainability"`
	Readability  int    `json:"readability"`
	Security     int    `json:"security"`
	Performance  int    `json:"performance"`
	Summary      string `json:"summary"`
}

type ReviewResult struct {
	ID           string        `json:"id"`
	Source       string        `json:"source"`
	Language     string        `json:"language,omitempty"`
	Quality      QualityScore  `json:"quality"`
	Findings     []Finding     `json:"findings"`
	Strengths    []string      `json:"strengths"`
	Suggestions  []string      `json:"suggestions"`
	AgentReports []AgentReport `json:"agent_reports,omitempty"`
	ReviewedAt   time.Time     `json:"reviewed_at"`
}

// AgentReport summarizes one specialist agent's contribution.
type AgentReport struct {
	Agent         string `json:"agent"`
	Score         int    `json:"score"`
	Summary       string `json:"summary"`
	FindingsCount int    `json:"findings_count"`
}

type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

type GitHubWebhookPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	Repository  struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	PullRequest struct {
		Number int    `json:"number"`
		DiffURL string `json:"diff_url"`
		Head   struct {
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}
