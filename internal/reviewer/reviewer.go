package reviewer

import (
	"context"

	"codereviewagent/internal/models"
)

// Reviewer analyzes source code and returns structured review results.
type Reviewer interface {
	ReviewCode(ctx context.Context, code, language, filePath, extraContext string) (*models.ReviewResult, error)
	ReviewDiff(ctx context.Context, diff, repoFullName string, prNumber int) (*models.ReviewResult, error)
}
