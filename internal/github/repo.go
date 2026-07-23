package github

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"

	"codereviewagent/internal/chunker"
	"codereviewagent/internal/errors"
)

// SourceFile is a file fetched from a GitHub repository.
type SourceFile struct {
	Path     string
	Language string
	Content  string
}

func (c *Client) GetRepositoryFiles(ctx context.Context, owner, repo, branch string, maxFiles int) ([]SourceFile, error) {
	if !c.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "GitHub token is not configured")
	}
	if maxFiles <= 0 {
		maxFiles = 20
	}

	repository, _, err := c.gh.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, errors.WithCause(errors.ErrInternal, fmt.Errorf("get repository: %w", err))
	}

	if branch == "" {
		branch = repository.GetDefaultBranch()
	}

	ref, _, err := c.gh.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		return nil, errors.WithCause(errors.ErrInternal, fmt.Errorf("get branch ref: %w", err))
	}

	tree, _, err := c.gh.Git.GetTree(ctx, owner, repo, ref.GetObject().GetSHA(), true)
	if err != nil {
		return nil, errors.WithCause(errors.ErrInternal, fmt.Errorf("get repo tree: %w", err))
	}

	var paths []string
	for _, entry := range tree.Entries {
		if entry.GetType() != "blob" {
			continue
		}
		path := entry.GetPath()
		if shouldSkipPath(path) {
			continue
		}
		if !chunker.IsReviewableExtension(path) {
			continue
		}
		paths = append(paths, path)
		if len(paths) >= maxFiles {
			break
		}
	}

	c.log.Info("fetching repository files",
		zap.String("repo", owner+"/"+repo),
		zap.String("branch", branch),
		zap.Int("file_count", len(paths)),
	)

	files := make([]SourceFile, 0, len(paths))
	for _, path := range paths {
		content, lang, fetchErr := c.getFileContent(ctx, owner, repo, path, branch)
		if fetchErr != nil {
			c.log.Warn("skip file", zap.String("path", path), zap.Error(fetchErr))
			continue
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		files = append(files, SourceFile{
			Path:     path,
			Language: lang,
			Content:  content,
		})
	}

	if len(files) == 0 {
		return nil, errors.WithMessage(errors.ErrInvalidRequest, "no reviewable files found in repository")
	}
	return files, nil
}

func (c *Client) GetPRFileChunks(ctx context.Context, owner, repo string, prNumber int) ([]SourceFile, error) {
	if !c.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}

	files, _, err := c.gh.PullRequests.ListFiles(ctx, owner, repo, prNumber, nil)
	if err != nil {
		return nil, errors.WithCause(errors.ErrInternal, fmt.Errorf("list PR files: %w", err))
	}

	result := make([]SourceFile, 0, len(files))
	for _, f := range files {
		content := ""
		if f.Patch != nil {
			content = *f.Patch
		}
		if content == "" {
			continue
		}
		result = append(result, SourceFile{
			Path:     f.GetFilename(),
			Language: chunker.DetectLanguage(f.GetFilename()),
			Content:  content,
		})
	}
	return result, nil
}

func (c *Client) getFileContent(ctx context.Context, owner, repo, path, ref string) (string, string, error) {
	file, _, _, err := c.gh.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return "", "", err
	}
	if file.Content == nil {
		return "", "", fmt.Errorf("empty file content")
	}

	decoded, err := base64.StdEncoding.DecodeString(*file.Content)
	if err != nil {
		return "", "", err
	}
	text := string(decoded)
	if len(text) > 100000 {
		text = text[:100000] + "\n... [truncated]"
	}
	return text, chunker.DetectLanguage(path), nil
}

func shouldSkipPath(path string) bool {
	lower := strings.ToLower(path)
	skipPrefixes := []string{
		"vendor/", "node_modules/", ".git/", "dist/", "build/",
		"bin/", "__pycache__/", ".idea/", ".vscode/", "testdata/",
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(lower, prefix) || strings.Contains(lower, "/"+prefix) {
			return true
		}
	}
	skipSuffixes := []string{".min.js", ".lock", ".sum", ".png", ".jpg", ".svg", ".ico", ".woff", ".ttf"}
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
