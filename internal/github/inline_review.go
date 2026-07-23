package github

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"

	"codereviewagent/internal/errors"
	"codereviewagent/internal/models"
)

const maxInlineComments = 20

// InlineComment is one line-level PR review comment.
type InlineComment struct {
	Path string
	Line int
	Body string
}

// GetPRHeadSHA returns the head commit SHA for a pull request.
func (c *Client) GetPRHeadSHA(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	if !c.Enabled() {
		return "", errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}

	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return "", errors.WithCause(errors.ErrInternal, fmt.Errorf("get pull request: %w", err))
	}
	sha := pr.GetHead().GetSHA()
	if sha == "" {
		return "", errors.WithMessage(errors.ErrInternal, "pull request head SHA is empty")
	}
	return sha, nil
}

// PostPullRequestReview creates a PR review with a summary body and optional
// inline comments. Event is always COMMENT (neither approve nor request changes).
func (c *Client) PostPullRequestReview(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	commitSHA, body string,
	comments []InlineComment,
) error {
	if !c.Enabled() {
		return errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}

	drafts := make([]*github.DraftReviewComment, 0, len(comments))
	for _, comment := range comments {
		path := comment.Path
		line := comment.Line
		text := comment.Body
		side := "RIGHT"
		drafts = append(drafts, &github.DraftReviewComment{
			Path: github.String(path),
			Line: github.Int(line),
			Side: github.String(side),
			Body: github.String(text),
		})
	}

	req := &github.PullRequestReviewRequest{
		CommitID: github.String(commitSHA),
		Body:     github.String(body),
		Event:    github.String("COMMENT"),
		Comments: drafts,
	}

	_, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, prNumber, req)
	if err != nil {
		c.log.Error("failed to create PR review",
			zap.String("repo", owner+"/"+repo),
			zap.Int("pr", prNumber),
			zap.Int("inline_comments", len(drafts)),
			zap.Error(err),
		)
		return errors.WithCause(errors.ErrInternal, fmt.Errorf("create PR review: %w", err))
	}

	c.log.Info("PR review created",
		zap.String("repo", owner+"/"+repo),
		zap.Int("pr", prNumber),
		zap.Int("inline_comments", len(drafts)),
	)
	return nil
}

// MapFindingsToInlineComments converts review findings into GitHub inline
// comments. Only findings whose file+line appear in the provided patches are
// included (GitHub rejects comments outside the diff). Critical/high first,
// capped at maxInlineComments.
func MapFindingsToInlineComments(findings []models.Finding, patches map[string]string) []InlineComment {
	reviewable := make(map[string]map[int]struct{}, len(patches))
	for path, patch := range patches {
		reviewable[path] = ParseReviewableLines(patch)
	}

	type ranked struct {
		finding models.Finding
		rank    int
	}
	candidates := make([]ranked, 0, len(findings))
	for _, f := range findings {
		if f.FilePath == "" || f.Line <= 0 {
			continue
		}
		lines, ok := reviewable[f.FilePath]
		if !ok {
			continue
		}
		if _, ok := lines[f.Line]; !ok {
			continue
		}
		candidates = append(candidates, ranked{finding: f, rank: severityRank(f.Severity)})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		if candidates[i].finding.FilePath != candidates[j].finding.FilePath {
			return candidates[i].finding.FilePath < candidates[j].finding.FilePath
		}
		return candidates[i].finding.Line < candidates[j].finding.Line
	})

	if len(candidates) > maxInlineComments {
		candidates = candidates[:maxInlineComments]
	}

	out := make([]InlineComment, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		key := fmt.Sprintf("%s:%d", c.finding.FilePath, c.finding.Line)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, InlineComment{
			Path: c.finding.FilePath,
			Line: c.finding.Line,
			Body: formatInlineCommentBody(c.finding),
		})
	}
	return out
}

func formatInlineCommentBody(f models.Finding) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**[%s / %s]** %s\n\n",
		strings.ToUpper(string(f.Severity)), f.Category, f.Title))
	if f.Description != "" {
		b.WriteString(f.Description)
		b.WriteString("\n")
	}
	if f.Suggestion != "" {
		b.WriteString("\n💡 **Suggestion:** ")
		b.WriteString(f.Suggestion)
	}
	return strings.TrimSpace(b.String())
}

func severityRank(s models.Severity) int {
	switch s {
	case models.SeverityCritical:
		return 0
	case models.SeverityHigh:
		return 1
	case models.SeverityMedium:
		return 2
	case models.SeverityLow:
		return 3
	default:
		return 4
	}
}
