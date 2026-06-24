package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"codereviewagent/internal/errors"
	"codereviewagent/internal/models"
	"codereviewagent/internal/service"
)

type ReviewHandler struct {
	svc           *service.ReviewService
	webhookSecret string
}

func NewReviewHandler(svc *service.ReviewService, webhookSecret string) *ReviewHandler {
	return &ReviewHandler{svc: svc, webhookSecret: webhookSecret}
}

func (h *ReviewHandler) ReviewCode(c *gin.Context) {
	var req models.ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, errors.WithMessage(errors.ErrInvalidRequest, "invalid request body"))
		return
	}

	result, err := h.svc.ReviewCode(c.Request.Context(), req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, models.APIResponse[models.ReviewResult]{
		Success: true,
		Data:    *result,
	})
}

func (h *ReviewHandler) ReviewPR(c *gin.Context) {
	var req models.PRReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, errors.WithMessage(errors.ErrInvalidRequest, "invalid request body"))
		return
	}

	result, err := h.svc.ReviewPullRequest(c.Request.Context(), req.Owner, req.Repo, req.PR)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, models.APIResponse[models.ReviewResult]{
		Success: true,
		Data:    *result,
	})
}

func (h *ReviewHandler) GitHubWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, errors.WithMessage(errors.ErrInvalidRequest, "failed to read webhook body"))
		return
	}

	if h.webhookSecret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, sig, h.webhookSecret) {
			respondError(c, errors.ErrUnauthorized)
			return
		}
	}

	event := c.GetHeader("X-GitHub-Event")
	if event != "pull_request" {
		c.JSON(http.StatusOK, gin.H{"message": "event ignored"})
		return
	}

	var payload models.GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		respondError(c, errors.WithMessage(errors.ErrInvalidRequest, "invalid webhook payload"))
		return
	}

	action := payload.Action
	if action != "opened" && action != "synchronize" && action != "reopened" {
		c.JSON(http.StatusOK, gin.H{"message": "action ignored", "action": action})
		return
	}

	owner := payload.Repository.Owner.Login
	repo := payload.Repository.Name
	prNumber := payload.PullRequest.Number
	if prNumber == 0 {
		prNumber = payload.Number
	}

	go func() {
		ctx := context.Background()
		_, reviewErr := h.svc.HandleWebhookPR(ctx, owner, repo, prNumber)
		if reviewErr != nil {
			log.Printf("webhook PR review failed for %s/%s#%d: %v", owner, repo, prNumber, reviewErr)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "review queued",
		"pr":      prNumber,
		"repo":    owner + "/" + repo,
	})
}

func (h *ReviewHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "codereviewagent",
	})
}

func handleServiceError(c *gin.Context, err error) {
	if ae, ok := errors.AsAppError(err); ok {
		respondError(c, ae)
		return
	}
	respondError(c, errors.WithCause(errors.ErrInternal, err))
}

func respondError(c *gin.Context, ae *errors.AppError) {
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
