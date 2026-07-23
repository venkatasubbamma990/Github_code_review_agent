package github

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"

	"codereviewagent/internal/errors"
)

var issueRefPattern = regexp.MustCompile(`(?i)(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+#(\d+)|(?:^|\s)#(\d+)\b`)

// PRContext is factual metadata gathered for the Context Agent.
type PRContext struct {
	Owner        string   `json:"owner"`
	Repo         string   `json:"repo"`
	Number       int      `json:"number"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	Author       string   `json:"author"`
	BaseBranch   string   `json:"base_branch"`
	HeadBranch   string   `json:"head_branch"`
	Labels       []string `json:"labels"`
	ChangedFiles []string `json:"changed_files"`
	IssueRefs    []string `json:"issue_refs"`
	Comments     []string `json:"comments"`
	Draft        bool     `json:"draft"`
}

// GetPRContext loads PR title/body/labels/comments and extracts issue references.
func (c *Client) GetPRContext(ctx context.Context, owner, repo string, prNumber int) (*PRContext, error) {
	if !c.Enabled() {
		return nil, errors.WithMessage(errors.ErrInternal, "GitHub client is not configured")
	}

	pr, _, err := c.gh.PullRequests.Get(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, errors.WithCause(errors.ErrInternal, fmt.Errorf("get pull request: %w", err))
	}

	out := &PRContext{
		Owner:      owner,
		Repo:       repo,
		Number:     prNumber,
		Title:      pr.GetTitle(),
		Body:       pr.GetBody(),
		Author:     pr.GetUser().GetLogin(),
		BaseBranch: pr.GetBase().GetRef(),
		HeadBranch: pr.GetHead().GetRef(),
		Draft:      pr.GetDraft(),
		Labels:     make([]string, 0, len(pr.Labels)),
	}
	for _, label := range pr.Labels {
		if label.GetName() != "" {
			out.Labels = append(out.Labels, label.GetName())
		}
	}
	out.IssueRefs = ExtractIssueRefs(out.Body)

	files, _, err := c.gh.PullRequests.ListFiles(ctx, owner, repo, prNumber, &github.ListOptions{PerPage: 100})
	if err != nil {
		c.log.Warn("failed to list PR files for context", zap.Error(err))
	} else {
		for _, f := range files {
			out.ChangedFiles = append(out.ChangedFiles, f.GetFilename())
		}
	}

	comments, _, err := c.gh.Issues.ListComments(ctx, owner, repo, prNumber, &github.IssueListCommentsOptions{
		ListOptions: github.ListOptions{PerPage: 10},
	})
	if err != nil {
		c.log.Warn("failed to list PR comments for context", zap.Error(err))
	} else {
		for _, comment := range comments {
			body := strings.TrimSpace(comment.GetBody())
			if body == "" {
				continue
			}
			author := comment.GetUser().GetLogin()
			out.Comments = append(out.Comments, fmt.Sprintf("%s: %s", author, truncateRunes(body, 500)))
		}
	}

	c.log.Info("PR context loaded",
		zap.String("repo", owner+"/"+repo),
		zap.Int("pr", prNumber),
		zap.Int("labels", len(out.Labels)),
		zap.Int("files", len(out.ChangedFiles)),
		zap.Int("comments", len(out.Comments)),
		zap.Int("issue_refs", len(out.IssueRefs)),
	)
	return out, nil
}

// ExtractIssueRefs finds #N and "fixes #N" style references in text.
func ExtractIssueRefs(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var refs []string
	for _, m := range issueRefPattern.FindAllStringSubmatch(body, -1) {
		num := m[1]
		if num == "" {
			num = m[2]
		}
		if num == "" {
			continue
		}
		ref := "#" + num
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func (p *PRContext) FormatRaw() string {
	if p == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("PR #%d: %s\n", p.Number, p.Title))
	b.WriteString(fmt.Sprintf("Author: %s\n", p.Author))
	b.WriteString(fmt.Sprintf("Branches: %s → %s\n", p.HeadBranch, p.BaseBranch))
	if p.Draft {
		b.WriteString("Draft: true\n")
	}
	if len(p.Labels) > 0 {
		b.WriteString("Labels: " + strings.Join(p.Labels, ", ") + "\n")
	}
	if len(p.IssueRefs) > 0 {
		b.WriteString("Linked issues: " + strings.Join(p.IssueRefs, ", ") + "\n")
	}
	if len(p.ChangedFiles) > 0 {
		b.WriteString("Changed files:\n")
		for i, f := range p.ChangedFiles {
			if i >= 40 {
				b.WriteString(fmt.Sprintf("  ...and %d more\n", len(p.ChangedFiles)-40))
				break
			}
			b.WriteString("  - " + f + "\n")
		}
	}
	if strings.TrimSpace(p.Body) != "" {
		b.WriteString("\nDescription:\n")
		b.WriteString(truncateRunes(p.Body, 4000))
		b.WriteString("\n")
	}
	if len(p.Comments) > 0 {
		b.WriteString("\nRecent discussion:\n")
		for _, cmt := range p.Comments {
			b.WriteString("- " + cmt + "\n")
		}
	}
	return b.String()
}
