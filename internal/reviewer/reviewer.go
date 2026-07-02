package reviewer

import (
	"context"

	ghclient "codereviewagent/internal/github"
	"codereviewagent/internal/models"
)

// Reviewer analyzes source code and returns structured review results.
type Reviewer interface {
	ReviewCode(ctx context.Context, code, language, filePath, extraContext string) (*models.ReviewResult, error)
	ReviewDiff(ctx context.Context, diff, repoFullName string, prNumber int) (*models.ReviewResult, error)
	ReviewFiles(ctx context.Context, source, repoFullName string, files []ghclient.SourceFile) (*models.ReviewResult, error)
}
