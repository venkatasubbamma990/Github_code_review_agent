package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"codereviewagent/internal/errors"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/models"
	"codereviewagent/internal/queue"
	"codereviewagent/internal/reviewer"
)

type ReviewService struct {
	reviewer     reviewer.Reviewer
	github       *ghclient.Client
	queue        *queue.Client
	postComments bool
	postChecks   bool
	maxRepoFiles int
	log          *zap.Logger
}

func NewReviewService(
	rev reviewer.Reviewer,
	gh *ghclient.Client,
	q *queue.Client,
	postComments bool,
	postChecks bool,
	maxRepoFiles int,
	log *zap.Logger,
) *ReviewService {
	return &ReviewService{
		reviewer:     rev,
		github:       gh,
		queue:        q,
		postComments: postComments,
		postChecks:   postChecks,
		maxRepoFiles: maxRepoFiles,
		log:          log.Named("service"),
	}
}

func (s *ReviewService) ReviewCode(ctx context.Context, req models.ReviewRequest) (*models.ReviewResult, error) {
	if strings.TrimSpace(req.Code) == "" {
		return nil, errors.WithMessage(errors.ErrInvalidRequest, "code cannot be empty")
	}
	return s.reviewer.ReviewCode(ctx, req.Code, req.Language, req.FilePath, req.Context)
}

func (s *ReviewService) ReviewPullRequest(ctx context.Context, owner, repo string, prNumber int) (*models.ReviewResult, error) {
	repoFullName := fmt.Sprintf("%s/%s", owner, repo)
	if !s.github.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "GitHub token is not configured")
	}

	files, err := s.github.GetPRFileChunks(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.WithMessage(errors.ErrInvalidRequest, "pull request has no reviewable changes")
	}

	result, err := s.reviewer.ReviewFiles(ctx, fmt.Sprintf("github:%s#%d", repoFullName, prNumber), repoFullName, files)
	if err != nil {
		return nil, err
	}

	sha, shaErr := s.github.GetPRHeadSHA(ctx, owner, repo, prNumber)
	if shaErr != nil {
		s.log.Warn("could not resolve PR head SHA", zap.Error(shaErr))
	}

	if s.postComments {
		if postErr := s.postPRReview(ctx, owner, repo, prNumber, files, result, sha); postErr != nil {
			return result, postErr
		}
	}

	if s.postChecks {
		if checkErr := s.postPRCheck(ctx, owner, repo, prNumber, result, sha); checkErr != nil {
			// Checks are best-effort: do not fail the review if posting status fails.
			s.log.Warn("failed to post PR check/status", zap.Error(checkErr))
		}
	}
	return result, nil
}

// postPRReview posts a GitHub pull request review with inline comments on
// findings that map to diff lines, plus a summary body. Falls back to a plain
// issue comment if the review API rejects the request.
func (s *ReviewService) postPRReview(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	files []ghclient.SourceFile,
	result *models.ReviewResult,
	sha string,
) error {
	summary := buildDetailedPRComment(result)

	patches := make(map[string]string, len(files))
	for _, f := range files {
		patches[f.Path] = f.Content
	}
	inline := ghclient.MapFindingsToInlineComments(result.Findings, patches)

	if sha == "" {
		var shaErr error
		sha, shaErr = s.github.GetPRHeadSHA(ctx, owner, repo, prNumber)
		if shaErr != nil {
			s.log.Warn("could not get PR head SHA; falling back to issue comment", zap.Error(shaErr))
			return s.github.PostReviewComment(ctx, owner, repo, prNumber, summary)
		}
	}

	if err := s.github.PostPullRequestReview(ctx, owner, repo, prNumber, sha, summary, inline); err != nil {
		s.log.Warn("PR review with inline comments failed; falling back to issue comment",
			zap.Int("inline_comments", len(inline)),
			zap.Error(err),
		)
		return s.github.PostReviewComment(ctx, owner, repo, prNumber, summary)
	}

	s.log.Info("posted PR review",
		zap.String("repo", owner+"/"+repo),
		zap.Int("pr", prNumber),
		zap.Int("inline_comments", len(inline)),
	)
	return nil
}

func (s *ReviewService) postPRCheck(
	ctx context.Context,
	owner, repo string,
	prNumber int,
	result *models.ReviewResult,
	sha string,
) error {
	if sha == "" {
		var err error
		sha, err = s.github.GetPRHeadSHA(ctx, owner, repo, prNumber)
		if err != nil {
			return err
		}
	}
	return s.github.PostReviewCheck(ctx, owner, repo, sha, result, prNumber)
}

func (s *ReviewService) ReviewRepository(ctx context.Context, req models.RepoReviewRequest) (*models.ReviewResult, error) {
	owner, repo, err := ghclient.ParseRepoURL(req.URL)
	if err != nil {
		return nil, errors.WithMessage(errors.ErrInvalidRequest, err.Error())
	}
	if !s.github.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "GitHub token is not configured")
	}

	maxFiles := req.MaxFiles
	if maxFiles <= 0 {
		maxFiles = s.maxRepoFiles
	}

	files, err := s.github.GetRepositoryFiles(ctx, owner, repo, req.Branch, maxFiles)
	if err != nil {
		return nil, err
	}

	repoFullName := owner + "/" + repo
	source := fmt.Sprintf("repo:%s@%s", repoFullName, req.Branch)
	if req.Branch == "" {
		source = fmt.Sprintf("repo:%s", repoFullName)
	}

	return s.reviewer.ReviewFiles(ctx, source, repoFullName, files)
}

func (s *ReviewService) QueueEnabled() bool {
	return s.queue != nil && s.queue.Enabled()
}

func (s *ReviewService) EnqueuePRReview(owner, repo string, prNumber int, headSHA string) (*queue.EnqueueResult, error) {
	if !s.QueueEnabled() {
		return nil, errors.WithMessage(errors.ErrServiceUnavailable, "async queue is not enabled (set REDIS_ADDR)")
	}
	return s.queue.EnqueuePRReview(owner, repo, prNumber, headSHA)
}

func (s *ReviewService) EnqueueRepoReview(owner, repo, branch string, maxFiles int) (*queue.EnqueueResult, error) {
	if !s.QueueEnabled() {
		return nil, errors.WithMessage(errors.ErrServiceUnavailable, "async queue is not enabled (set REDIS_ADDR)")
	}
	return s.queue.EnqueueRepoReview(owner, repo, branch, maxFiles)
}

func (s *ReviewService) GetJobStatus(jobID string) (*models.JobStatusResponse, error) {
	if s.queue == nil || !s.queue.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "async queue is not enabled")
	}
	status, err := s.queue.GetJobStatus(jobID)
	if err != nil {
		return nil, errors.WithMessage(errors.ErrNotFound, "job not found")
	}
	return &models.JobStatusResponse{
		ID:            status.ID,
		Type:          status.Type,
		State:         status.State,
		Queue:         status.Queue,
		Result:        status.Result,
		Error:         status.Error,
		Retried:       status.Retried,
		MaxRetry:      status.MaxRetry,
		NextProcessAt: status.NextProcessAt,
		CompletedAt:   status.CompletedAt,
		LastFailedAt:  status.LastFailedAt,
	}, nil
}

func (s *ReviewService) HandleWebhookPR(ctx context.Context, owner, repo string, prNumber int) (*models.ReviewResult, error) {
	return s.ReviewPullRequest(ctx, owner, repo, prNumber)
}

func (s *ReviewService) ProcessPRReviewTask(ctx context.Context, owner, repo string, prNumber int) ([]byte, error) {
	result, err := s.ReviewPullRequest(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (s *ReviewService) ProcessRepoReviewTask(ctx context.Context, owner, repo, branch string, maxFiles int) ([]byte, error) {
	result, err := s.ReviewRepository(ctx, models.RepoReviewRequest{
		URL:      fmt.Sprintf("https://github.com/%s/%s", owner, repo),
		Branch:   branch,
		MaxFiles: maxFiles,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func buildDetailedPRComment(r *models.ReviewResult) string {
	var b strings.Builder
	b.WriteString("## 🤖 Code Review Agent Report\n\n")
	b.WriteString(fmt.Sprintf("**Overall Quality:** %d/100\n\n", r.Quality.Overall))
	b.WriteString("| Maintainability | Readability | Security | Performance |\n")
	b.WriteString("|:---:|:---:|:---:|:---:|\n")
	b.WriteString(fmt.Sprintf("| %d | %d | %d | %d |\n\n",
		r.Quality.Maintainability, r.Quality.Readability, r.Quality.Security, r.Quality.Performance))
	b.WriteString(fmt.Sprintf("**Summary:** %s\n\n", r.Quality.Summary))

	if len(r.AgentReports) > 0 {
		b.WriteString("### Agent Scores\n\n")
		for _, ar := range r.AgentReports {
			b.WriteString(fmt.Sprintf("- **%s**: %d/100 (%d findings)\n", ar.Agent, ar.Score, ar.FindingsCount))
		}
		b.WriteString("\n")
	}

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
		for _, item := range r.Strengths {
			b.WriteString(fmt.Sprintf("- ✅ %s\n", item))
		}
		b.WriteString("\n")
	}

	if len(r.Suggestions) > 0 {
		b.WriteString("### Suggestions\n\n")
		for _, item := range r.Suggestions {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
		b.WriteString("\n")
	}

	b.WriteString("_Generated by Multi-Agent Code Review Agent_\n")
	return b.String()
}
