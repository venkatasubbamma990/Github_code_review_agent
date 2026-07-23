package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"

	"codereviewagent/internal/errors"
)

type Client struct {
	gh  *github.Client
	log *zap.Logger
}

func NewClient(token string, log *zap.Logger) *Client {
	if token == "" {
		return &Client{log: log.Named("github")}
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return &Client{gh: github.NewClient(tc), log: log.Named("github")}
}

func (c *Client) Enabled() bool {
	return c != nil && c.gh != nil
}

func (c *Client) PostReviewComment(ctx context.Context, owner, repo string, prNumber int, body string) error {
	if !c.Enabled() {
		return errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}

	comment := &github.IssueComment{Body: github.String(body)}
	_, _, err := c.gh.Issues.CreateComment(ctx, owner, repo, prNumber, comment)
	if err != nil {
		c.log.Error("failed to create PR comment",
			zap.String("repo", owner+"/"+repo),
			zap.Int("pr", prNumber),
			zap.Error(err),
		)
		return errors.WithCause(errors.ErrInternal, fmt.Errorf("post PR comment: %w", err))
	}
	c.log.Info("PR comment created",
		zap.String("repo", owner+"/"+repo),
		zap.Int("pr", prNumber),
	)
	return nil
}
