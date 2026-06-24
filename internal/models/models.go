package models

import "time"

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
	ID          string        `json:"id"`
	Source      string        `json:"source"`
	Language    string        `json:"language,omitempty"`
	Quality     QualityScore  `json:"quality"`
	Findings    []Finding     `json:"findings"`
	Strengths   []string      `json:"strengths"`
	Suggestions []string      `json:"suggestions"`
	ReviewedAt  time.Time     `json:"reviewed_at"`
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
