package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"codereviewagent/internal/errors"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/models"
	"codereviewagent/internal/reviewer"
)

type ReviewService struct {
	reviewer     reviewer.Reviewer
	github       *ghclient.Client
	postComments bool
	log          *zap.Logger
}

func NewReviewService(rev reviewer.Reviewer, gh *ghclient.Client, postComments bool, log *zap.Logger) *ReviewService {
	return &ReviewService{
		reviewer:     rev,
		github:       gh,
		postComments: postComments,
		log:          log.Named("service"),
	}
}

func (s *ReviewService) ReviewCode(ctx context.Context, req models.ReviewRequest) (*models.ReviewResult, error) {
	if strings.TrimSpace(req.Code) == "" {
		return nil, errors.WithMessage(errors.ErrInvalidRequest, "code cannot be empty")
	}

	s.log.Debug("starting code review",
		zap.String("language", req.Language),
		zap.String("file_path", req.FilePath),
	)

	result, err := s.reviewer.ReviewCode(ctx, req.Code, req.Language, req.FilePath, req.Context)
	if err != nil {
		s.log.Error("code review failed",
			zap.String("language", req.Language),
			zap.Error(err),
		)
		return nil, err
	}

	s.log.Info("code review finished",
		zap.String("review_id", result.ID),
		zap.Int("overall_score", result.Quality.Overall),
		zap.Int("findings", len(result.Findings)),
	)
	return result, nil
}

func (s *ReviewService) ReviewPullRequest(ctx context.Context, owner, repo string, prNumber int) (*models.ReviewResult, error) {
	repoFullName := fmt.Sprintf("%s/%s", owner, repo)

	if !s.github.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "GitHub token is not configured")
	}

	s.log.Info("fetching PR diff", zap.String("repo", repoFullName), zap.Int("pr", prNumber))

	diff, err := s.github.GetPRDiff(ctx, owner, repo, prNumber)
	if err != nil {
		s.log.Error("failed to fetch PR diff",
			zap.String("repo", repoFullName),
			zap.Int("pr", prNumber),
			zap.Error(err),
		)
		return nil, err
	}
	if strings.TrimSpace(diff) == "" {
		return nil, errors.WithMessage(errors.ErrInvalidRequest, "pull request has no reviewable changes")
	}

	s.log.Debug("PR diff fetched",
		zap.String("repo", repoFullName),
		zap.Int("pr", prNumber),
		zap.Int("diff_length", len(diff)),
	)

	result, err := s.reviewer.ReviewDiff(ctx, diff, repoFullName, prNumber)
	if err != nil {
		s.log.Error("PR diff review failed",
			zap.String("repo", repoFullName),
			zap.Int("pr", prNumber),
			zap.Error(err),
		)
		return nil, err
	}

	if s.postComments {
		comment := buildDetailedPRComment(result)
		s.log.Info("posting review comment to PR",
			zap.String("repo", repoFullName),
			zap.Int("pr", prNumber),
		)
		if postErr := s.github.PostReviewComment(ctx, owner, repo, prNumber, comment); postErr != nil {
			s.log.Error("failed to post PR comment",
				zap.String("repo", repoFullName),
				zap.Int("pr", prNumber),
				zap.Error(postErr),
			)
			return result, postErr
		}
		s.log.Info("review comment posted",
			zap.String("repo", repoFullName),
			zap.Int("pr", prNumber),
		)
	}

	s.log.Info("PR review finished",
		zap.String("review_id", result.ID),
		zap.String("repo", repoFullName),
		zap.Int("pr", prNumber),
		zap.Int("overall_score", result.Quality.Overall),
	)
	return result, nil
}

func (s *ReviewService) HandleWebhookPR(ctx context.Context, owner, repo string, prNumber int) (*models.ReviewResult, error) {
	s.log.Info("processing webhook PR review",
		zap.String("repo", owner+"/"+repo),
		zap.Int("pr", prNumber),
	)
	return s.ReviewPullRequest(ctx, owner, repo, prNumber)
}

func buildDetailedPRComment(r *models.ReviewResult) string {
	var b strings.Builder
	b.WriteString("## 🤖 Code Review Agent Report\n\n")
	b.WriteString(fmt.Sprintf("**Overall Quality:** %d/100\n\n", r.Quality.Overall))
	b.WriteString(fmt.Sprintf("| Maintainability | Readability | Security | Performance |\n"))
	b.WriteString(fmt.Sprintf("|:---:|:---:|:---:|:---:|\n"))
	b.WriteString(fmt.Sprintf("| %d | %d | %d | %d |\n\n",
		r.Quality.Maintainability, r.Quality.Readability, r.Quality.Security, r.Quality.Performance))
	b.WriteString(fmt.Sprintf("**Summary:** %s\n\n", r.Quality.Summary))

	if len(r.Findings) > 0 {
		b.WriteString("### Findings\n\n")
		for i, f := range r.Findings {
			if i >= 15 {
				b.WriteString(fmt.Sprintf("\n_...and %d more findings_\n", len(r.Findings)-15))
				break
			}
			loc := ""
			if f.FilePath != "" {
				loc = fmt.Sprintf(" (`%s`", f.FilePath)
				if f.Line > 0 {
					loc += fmt.Sprintf(":%d", f.Line)
				}
				loc += ")"
			}
			b.WriteString(fmt.Sprintf("- **[%s/%s]** %s%s: %s\n",
				strings.ToUpper(string(f.Severity)), f.Category, f.Title, loc, f.Description))
			if f.Suggestion != "" {
				b.WriteString(fmt.Sprintf("  - 💡 _Suggestion:_ %s\n", f.Suggestion))
			}
		}
		b.WriteString("\n")
	}

	if len(r.Strengths) > 0 {
		b.WriteString("### Strengths\n\n")
		for _, s := range r.Strengths {
			b.WriteString(fmt.Sprintf("- ✅ %s\n", s))
		}
		b.WriteString("\n")
	}

	if len(r.Suggestions) > 0 {
		b.WriteString("### Suggestions\n\n")
		for _, s := range r.Suggestions {
			b.WriteString(fmt.Sprintf("- %s\n", s))
		}
		b.WriteString("\n")
	}

	b.WriteString("_Generated by Code Review Agent_\n")
	return b.String()
}
