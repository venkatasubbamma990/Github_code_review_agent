package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"codereviewagent/internal/errors"
)

type Client struct {
	apiKey      string
	baseURL     string
	model       string
	useJSONMode bool
	httpClient  *http.Client
	log         *zap.Logger
}

func NewClient(apiKey, baseURL, model string, useJSONMode bool, log *zap.Logger) *Client {
	return &Client{
		apiKey:      apiKey,
		baseURL:     strings.TrimRight(baseURL, "/"),
		model:       model,
		useJSONMode: useJSONMode,
		log:         log.Named("llm"),
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Client) ChatJSON(ctx context.Context, caller, systemPrompt, userPrompt string, dest any) error {
	if c.apiKey == "" {
		return errors.WithMessage(errors.ErrReviewFailed, "LLM API key is not configured")
	}

	start := time.Now()
	c.log.Debug("LLM request",
		zap.String("caller", caller),
		zap.String("model", c.model),
	)

	payload := chatRequest{
		Model: c.model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	}
	if c.useJSONMode {
		payload.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return errors.WithCause(errors.ErrReviewFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return errors.WithCause(errors.ErrReviewFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.WithCause(errors.ErrReviewFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.WithCause(errors.ErrReviewFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Error("LLM API error",
			zap.String("caller", caller),
			zap.Int("status", resp.StatusCode),
			zap.Duration("duration", time.Since(start)),
		)
		return errors.WithMessage(errors.ErrReviewFailed,
			fmt.Sprintf("LLM API returned status %d: %s", resp.StatusCode, truncate(string(respBody), 500)))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return errors.WithCause(errors.ErrReviewFailed, err)
	}
	if len(chatResp.Choices) == 0 {
		return errors.WithMessage(errors.ErrReviewFailed, "LLM returned empty response")
	}

	content := stripMarkdownFence(strings.TrimSpace(chatResp.Choices[0].Message.Content))
	if err := json.Unmarshal([]byte(content), dest); err != nil {
		c.log.Error("failed to parse LLM JSON",
			zap.String("caller", caller),
			zap.Error(err),
		)
		return errors.WithMessage(errors.ErrReviewFailed,
			fmt.Sprintf("failed to parse LLM JSON: %v", err))
	}

	c.log.Debug("LLM response received",
		zap.String("caller", caller),
		zap.Duration("duration", time.Since(start)),
	)
	return nil
}

func stripMarkdownFence(s string) string {
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			return strings.Join(lines, "\n")
		}
	}
	return s
}

func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}

func truncate(s string, max int) string {
	return Truncate(s, max)
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}
