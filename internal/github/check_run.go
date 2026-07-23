package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"

	"codereviewagent/internal/errors"
	"codereviewagent/internal/models"
)

const (
	checkRunName     = "Code Review Agent"
	commitStatusCtx  = "code-review-agent"
	maxCheckTextRunes = 65535
)

// CheckConclusion is a GitHub Checks API conclusion value.
type CheckConclusion string

const (
	ConclusionSuccess CheckConclusion = "success"
	ConclusionNeutral CheckConclusion = "neutral"
	ConclusionFailure CheckConclusion = "failure"
)

// CheckReport is the payload posted as a check run / commit status.
type CheckReport struct {
	Conclusion CheckConclusion
	Title      string
	Summary    string
	Text       string
	// StatusState is the commit-status equivalent (success|failure|error|pending).
	StatusState string
	StatusDesc  string
}

// BuildCheckReport maps a review result to a check/status report.
//
// Rules:
//   - any critical finding, or overall < 50 → failure
//   - any high finding, or overall < 70 → neutral
//   - otherwise → success
func BuildCheckReport(r *models.ReviewResult) CheckReport {
	overall := 0
	summary := ""
	if r != nil {
		overall = r.Quality.Overall
		summary = r.Quality.Summary
	}

	critical, high := countSeverities(r)
	conclusion := ConclusionSuccess
	switch {
	case critical > 0 || overall < 50:
		conclusion = ConclusionFailure
	case high > 0 || overall < 70:
		conclusion = ConclusionNeutral
	}

	title := fmt.Sprintf("Quality %d/100", overall)
	switch conclusion {
	case ConclusionFailure:
		title = fmt.Sprintf("Failed — Quality %d/100", overall)
	case ConclusionNeutral:
		title = fmt.Sprintf("Needs attention — Quality %d/100", overall)
	case ConclusionSuccess:
		title = fmt.Sprintf("Passed — Quality %d/100", overall)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Overall:** %d/100\n\n", overall))
	if summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	b.WriteString(fmt.Sprintf("Critical: %d · High: %d · Total findings: %d\n",
		critical, high, findingsLen(r)))

	if r != nil && len(r.AgentReports) > 0 {
		b.WriteString("\n### Agent scores\n")
		for _, ar := range r.AgentReports {
			b.WriteString(fmt.Sprintf("- **%s**: %d/100 (%d findings)\n",
				ar.Agent, ar.Score, ar.FindingsCount))
		}
	}

	text := buildCheckText(r)
	statusState, statusDesc := commitStatusFor(conclusion, overall, critical, high)

	return CheckReport{
		Conclusion:  conclusion,
		Title:       title,
		Summary:     b.String(),
		Text:        text,
		StatusState: statusState,
		StatusDesc:  statusDesc,
	}
}

// CreateCheckRun posts a completed GitHub Check Run for the given commit.
// Requires a GitHub App token with checks:write in most cases.
func (c *Client) CreateCheckRun(
	ctx context.Context,
	owner, repo, headSHA string,
	report CheckReport,
	detailsURL string,
) error {
	if !c.Enabled() {
		return errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}
	if headSHA == "" {
		return errors.WithMessage(errors.ErrInvalidRequest, "head SHA is required")
	}

	now := github.Timestamp{Time: time.Now().UTC()}
	opts := github.CreateCheckRunOptions{
		Name:        checkRunName,
		HeadSHA:     headSHA,
		Status:      github.String("completed"),
		Conclusion:  github.String(string(report.Conclusion)),
		CompletedAt: &now,
		Output: &github.CheckRunOutput{
			Title:   github.String(report.Title),
			Summary: github.String(report.Summary),
			Text:    github.String(truncateRunes(report.Text, maxCheckTextRunes)),
		},
	}
	if detailsURL != "" {
		opts.DetailsURL = github.String(detailsURL)
	}

	_, _, err := c.gh.Checks.CreateCheckRun(ctx, owner, repo, opts)
	if err != nil {
		c.log.Error("failed to create check run",
			zap.String("repo", owner+"/"+repo),
			zap.String("sha", headSHA),
			zap.String("conclusion", string(report.Conclusion)),
			zap.Error(err),
		)
		return errors.WithCause(errors.ErrInternal, fmt.Errorf("create check run: %w", err))
	}

	c.log.Info("check run created",
		zap.String("repo", owner+"/"+repo),
		zap.String("sha", headSHA),
		zap.String("conclusion", string(report.Conclusion)),
	)
	return nil
}

// CreateCommitStatus posts a classic commit status (works with PATs that have
// statuses write permission). Used as a fallback when Check Runs are unavailable.
func (c *Client) CreateCommitStatus(
	ctx context.Context,
	owner, repo, headSHA string,
	report CheckReport,
	detailsURL string,
) error {
	if !c.Enabled() {
		return errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}
	if headSHA == "" {
		return errors.WithMessage(errors.ErrInvalidRequest, "head SHA is required")
	}

	status := &github.RepoStatus{
		State:       github.String(report.StatusState),
		Description: github.String(truncateASCII(report.StatusDesc, 140)),
		Context:     github.String(commitStatusCtx),
	}
	if detailsURL != "" {
		status.TargetURL = github.String(detailsURL)
	}

	_, _, err := c.gh.Repositories.CreateStatus(ctx, owner, repo, headSHA, status)
	if err != nil {
		c.log.Error("failed to create commit status",
			zap.String("repo", owner+"/"+repo),
			zap.String("sha", headSHA),
			zap.String("state", report.StatusState),
			zap.Error(err),
		)
		return errors.WithCause(errors.ErrInternal, fmt.Errorf("create commit status: %w", err))
	}

	c.log.Info("commit status created",
		zap.String("repo", owner+"/"+repo),
		zap.String("sha", headSHA),
		zap.String("state", report.StatusState),
	)
	return nil
}

// PostReviewCheck prefers Check Runs, then falls back to commit status.
func (c *Client) PostReviewCheck(
	ctx context.Context,
	owner, repo, headSHA string,
	result *models.ReviewResult,
	prNumber int,
) error {
	report := BuildCheckReport(result)
	detailsURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, prNumber)

	if err := c.CreateCheckRun(ctx, owner, repo, headSHA, report, detailsURL); err == nil {
		return nil
	} else {
		c.log.Warn("check run failed; falling back to commit status", zap.Error(err))
	}

	return c.CreateCommitStatus(ctx, owner, repo, headSHA, report, detailsURL)
}

func countSeverities(r *models.ReviewResult) (critical, high int) {
	if r == nil {
		return 0, 0
	}
	for _, f := range r.Findings {
		switch f.Severity {
		case models.SeverityCritical:
			critical++
		case models.SeverityHigh:
			high++
		}
	}
	return critical, high
}

func findingsLen(r *models.ReviewResult) int {
	if r == nil {
		return 0
	}
	return len(r.Findings)
}

func commitStatusFor(c CheckConclusion, overall, critical, high int) (state, desc string) {
	switch c {
	case ConclusionFailure:
		return "failure", fmt.Sprintf("Quality %d/100 · %d critical · %d high", overall, critical, high)
	case ConclusionNeutral:
		// Commit statuses have no "neutral"; surface as success with a warning description.
		return "success", fmt.Sprintf("Needs attention · Quality %d/100 · %d high", overall, high)
	default:
		return "success", fmt.Sprintf("Passed · Quality %d/100", overall)
	}
}

func buildCheckText(r *models.ReviewResult) string {
	if r == nil || len(r.Findings) == 0 {
		return "No findings."
	}
	var b strings.Builder
	b.WriteString("### Findings\n\n")
	for i, f := range r.Findings {
		if i >= 30 {
			b.WriteString(fmt.Sprintf("\n_...and %d more_\n", len(r.Findings)-30))
			break
		}
		loc := f.FilePath
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.FilePath, f.Line)
		}
		b.WriteString(fmt.Sprintf("- **[%s]** %s", strings.ToUpper(string(f.Severity)), f.Title))
		if loc != "" {
			b.WriteString(fmt.Sprintf(" (`%s`)", loc))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func truncateASCII(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
