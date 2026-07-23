package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"codereviewagent/internal/errors"
	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/models"
	"codereviewagent/internal/service"
)

type ReviewHandler struct {
	svc           *service.ReviewService
	webhookSecret string
	log           *zap.Logger
}

func NewReviewHandler(svc *service.ReviewService, webhookSecret string, log *zap.Logger) *ReviewHandler {
	return &ReviewHandler{svc: svc, webhookSecret: webhookSecret, log: log.Named("handler")}
}

func (h *ReviewHandler) ReviewCode(c *gin.Context) {
	var req models.ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("invalid review request", zap.Error(err))
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "invalid request body"))
		return
	}

	h.log.Info("code review requested",
		zap.String("language", req.Language),
		zap.String("file_path", req.FilePath),
		zap.Int("code_length", len(req.Code)),
	)

	result, err := h.svc.ReviewCode(c.Request.Context(), req)
	if err != nil {
		handleServiceError(c, h.log, err)
		return
	}

	h.log.Info("code review completed",
		zap.String("review_id", result.ID),
		zap.Int("overall_score", result.Quality.Overall),
		zap.Int("findings_count", len(result.Findings)),
	)

	c.JSON(http.StatusOK, models.APIResponse[models.ReviewResult]{
		Success: true,
		Data:    *result,
	})
}

func (h *ReviewHandler) ReviewPR(c *gin.Context) {
	var req models.PRReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("invalid PR review request", zap.Error(err))
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "invalid request body"))
		return
	}

	h.log.Info("PR review requested",
		zap.String("owner", req.Owner),
		zap.String("repo", req.Repo),
		zap.Int("pr", req.PR),
	)

	result, err := h.svc.ReviewPullRequest(c.Request.Context(), req.Owner, req.Repo, req.PR)
	if err != nil {
		handleServiceError(c, h.log, err)
		return
	}

	h.log.Info("PR review completed",
		zap.String("review_id", result.ID),
		zap.String("repo", req.Owner+"/"+req.Repo),
		zap.Int("pr", req.PR),
		zap.Int("overall_score", result.Quality.Overall),
	)

	c.JSON(http.StatusOK, models.APIResponse[models.ReviewResult]{
		Success: true,
		Data:    *result,
	})
}

func (h *ReviewHandler) ReviewRepo(c *gin.Context) {
	var req models.RepoReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("invalid repo review request", zap.Error(err))
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "invalid request body"))
		return
	}

	owner, repo, err := ghclient.ParseRepoURL(req.URL)
	if err != nil {
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, err.Error()))
		return
	}

	h.log.Info("repo review requested",
		zap.String("url", req.URL),
		zap.String("repo", owner+"/"+repo),
		zap.Bool("async", req.Async),
	)

	if req.Async {
		result, enqueueErr := h.svc.EnqueueRepoReview(owner, repo, req.Branch, req.MaxFiles)
		if enqueueErr != nil {
			handleServiceError(c, h.log, enqueueErr)
			return
		}
		c.JSON(http.StatusAccepted, models.APIResponse[models.ReviewJobAccepted]{
			Success: true,
			Data: models.ReviewJobAccepted{
				JobID:   result.JobID,
				Message: "repository review queued",
				Repo:    owner + "/" + repo,
			},
		})
		return
	}

	reviewResult, err := h.svc.ReviewRepository(c.Request.Context(), req)
	if err != nil {
		handleServiceError(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, models.APIResponse[models.ReviewResult]{
		Success: true,
		Data:    *reviewResult,
	})
}

func (h *ReviewHandler) GetJobStatus(c *gin.Context) {
	jobID := c.Param("id")
	if jobID == "" {
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "job id is required"))
		return
	}

	status, err := h.svc.GetJobStatus(jobID)
	if err != nil {
		handleServiceError(c, h.log, err)
		return
	}

	c.JSON(http.StatusOK, models.APIResponse[models.JobStatusResponse]{
		Success: true,
		Data:    *status,
	})
}

func (h *ReviewHandler) GitHubWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.log.Warn("failed to read webhook body", zap.Error(err))
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "failed to read webhook body"))
		return
	}

	if h.webhookSecret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, sig, h.webhookSecret) {
			h.log.Warn("webhook signature verification failed")
			respondError(c, h.log, errors.ErrUnauthorized)
			return
		}
	}

	event := c.GetHeader("X-GitHub-Event")
	if event != "pull_request" {
		h.log.Debug("webhook event ignored", zap.String("event", event))
		c.JSON(http.StatusOK, gin.H{"message": "event ignored"})
		return
	}

	var payload models.GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.log.Warn("invalid webhook payload", zap.Error(err))
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "invalid webhook payload"))
		return
	}

	action := payload.Action
	if action != "opened" && action != "synchronize" && action != "reopened" {
		h.log.Debug("webhook action ignored", zap.String("action", action))
		c.JSON(http.StatusOK, gin.H{"message": "action ignored", "action": action})
		return
	}

	owner := payload.Repository.Owner.Login
	repo := payload.Repository.Name
	prNumber := payload.PullRequest.Number
	if prNumber == 0 {
		prNumber = payload.Number
	}
	headSHA := payload.PullRequest.Head.SHA

	if owner == "" || repo == "" || prNumber <= 0 {
		respondError(c, h.log, errors.WithMessage(errors.ErrInvalidRequest, "webhook payload missing repository or PR number"))
		return
	}

	if !h.svc.QueueEnabled() {
		h.log.Error("webhook rejected: Redis queue is required",
			zap.String("repo", owner+"/"+repo),
			zap.Int("pr", prNumber),
		)
		respondError(c, h.log, errors.WithMessage(errors.ErrServiceUnavailable,
			"webhook processing requires Redis (set REDIS_ADDR)"))
		return
	}

	h.log.Info("webhook PR review enqueue",
		zap.String("action", action),
		zap.String("repo", owner+"/"+repo),
		zap.Int("pr", prNumber),
		zap.String("sha", headSHA),
	)

	enqueued, enqueueErr := h.svc.EnqueuePRReview(owner, repo, prNumber, headSHA)
	if enqueueErr != nil {
		handleServiceError(c, h.log, enqueueErr)
		return
	}

	msg := "review queued"
	if enqueued.Deduplicated {
		msg = "review already queued for this commit"
	}

	c.JSON(http.StatusAccepted, models.APIResponse[models.ReviewJobAccepted]{
		Success: true,
		Data: models.ReviewJobAccepted{
			JobID:        enqueued.JobID,
			Message:      msg,
			Repo:         owner + "/" + repo,
			PR:           prNumber,
			Deduplicated: enqueued.Deduplicated,
		},
	})
}

func (h *ReviewHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "codereviewagent",
	})
}

func handleServiceError(c *gin.Context, log *zap.Logger, err error) {
	if ae, ok := errors.AsAppError(err); ok {
		if ae.HTTPStatus >= 500 {
			log.Error("service error", zap.String("code", ae.Code), zap.Error(err))
		} else {
			log.Warn("service error", zap.String("code", ae.Code), zap.String("message", ae.ClientMessage()))
		}
		respondError(c, log, ae)
		return
	}
	log.Error("unexpected service error", zap.Error(err))
	respondError(c, log, errors.WithCause(errors.ErrInternal, err))
}

func respondError(c *gin.Context, log *zap.Logger, ae *errors.AppError) {
	c.JSON(ae.HTTPStatus, models.APIResponse[any]{
		Success: false,
		Error:   ae.ClientMessage(),
	})
}

func verifyGitHubSignature(payload []byte, signature, secret string) bool {
	if signature == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
