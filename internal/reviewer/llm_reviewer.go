package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"codereviewagent/internal/errors"
	"codereviewagent/internal/models"
)

type LLMReviewer struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewLLMReviewer(apiKey, baseURL, model string) *LLMReviewer {
	return &LLMReviewer{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

const systemPrompt = `You are an expert senior software engineer and security auditor acting as a GitHub code review agent.

Analyze the provided code thoroughly and return ONLY valid JSON matching this schema:
{
  "quality": {
    "overall": <0-100>,
    "maintainability": <0-100>,
    "readability": <0-100>,
    "security": <0-100>,
    "performance": <0-100>,
    "summary": "<brief quality assessment>"
  },
  "findings": [
    {
      "category": "<security|bug|performance|style|maintainability|best-practice>",
      "severity": "<critical|high|medium|low|info>",
      "title": "<short title>",
      "description": "<detailed explanation>",
      "file_path": "<optional file path>",
      "line": <optional line number or 0>,
      "suggestion": "<actionable fix>"
    }
  ],
  "strengths": ["<positive aspects>"],
  "suggestions": ["<general improvement suggestions>"]
}

Focus on:
- Security vulnerabilities (OWASP Top 10, injection, secrets, auth issues)
- Code quality and maintainability
- Performance concerns
- Error handling and edge cases
- Best practices for the language/framework
- Testing gaps

Be specific, actionable, and prioritize critical issues.`

func (r *LLMReviewer) ReviewCode(ctx context.Context, code, language, filePath, extraContext string) (*models.ReviewResult, error) {
	userPrompt := buildCodePrompt(code, language, filePath, extraContext)
	return r.executeReview(ctx, userPrompt, "manual", language)
}

func (r *LLMReviewer) ReviewDiff(ctx context.Context, diff, repoFullName string, prNumber int) (*models.ReviewResult, error) {
	userPrompt := fmt.Sprintf(`Review this GitHub pull request diff.

Repository: %s
PR Number: #%d

Diff:
%s

Analyze only the changed code. Flag security issues, bugs, and quality problems in the diff.`,
		repoFullName, prNumber, truncate(diff, 100000))
	return r.executeReview(ctx, userPrompt, fmt.Sprintf("github:%s#%d", repoFullName, prNumber), "")
}

func (r *LLMReviewer) executeReview(ctx context.Context, userPrompt, source, language string) (*models.ReviewResult, error) {
	if r.apiKey == "" {
		return nil, errors.WithMessage(errors.ErrReviewFailed, "LLM API key is not configured")
	}

	payload := chatRequest{
		Model: r.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.WithCause(errors.ErrReviewFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, errors.WithCause(errors.ErrReviewFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, errors.WithCause(errors.ErrReviewFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithCause(errors.ErrReviewFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.WithMessage(errors.ErrReviewFailed,
			fmt.Sprintf("LLM API returned status %d: %s", resp.StatusCode, truncate(string(respBody), 500)))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, errors.WithCause(errors.ErrReviewFailed, err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, errors.WithMessage(errors.ErrReviewFailed, "LLM returned empty response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	content = stripMarkdownFence(content)

	var parsed llmReviewOutput
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, errors.WithMessage(errors.ErrReviewFailed,
			fmt.Sprintf("failed to parse LLM review JSON: %v", err))
	}

	result := &models.ReviewResult{
		ID:          uuid.New().String(),
		Source:      source,
		Language:    language,
		Quality:     parsed.Quality,
		Findings:    parsed.Findings,
		Strengths:   parsed.Strengths,
		Suggestions: parsed.Suggestions,
		ReviewedAt:  time.Now().UTC(),
	}
	if result.Findings == nil {
		result.Findings = []models.Finding{}
	}
	if result.Strengths == nil {
		result.Strengths = []string{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}
	return result, nil
}

func buildCodePrompt(code, language, filePath, extraContext string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review the following %s code", language))
	if filePath != "" {
		b.WriteString(fmt.Sprintf(" from file: %s", filePath))
	}
	b.WriteString(".\n\n")
	if extraContext != "" {
		b.WriteString(fmt.Sprintf("Additional context: %s\n\n", extraContext))
	}
	b.WriteString("```")
	b.WriteString(language)
	b.WriteString("\n")
	b.WriteString(truncate(code, 100000))
	b.WriteString("\n```")
	return b.String()
}

func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type llmReviewOutput struct {
	Quality     models.QualityScore `json:"quality"`
	Findings    []models.Finding    `json:"findings"`
	Strengths   []string            `json:"strengths"`
	Suggestions []string            `json:"suggestions"`
}
