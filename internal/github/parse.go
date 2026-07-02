package github

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var githubURLPattern = regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?github\.com/([\w.-]+)/([\w.-]+?)(?:\.git)?/?$`)

// ParseRepoURL extracts owner and repo from a GitHub URL or "owner/repo" shorthand.
func ParseRepoURL(raw string) (owner, repo string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("repository URL is empty")
	}

	if m := githubURLPattern.FindStringSubmatch(raw); len(m) == 3 {
		return m[1], strings.TrimSuffix(m[2], ".git"), nil
	}

	if u, parseErr := url.Parse(raw); parseErr == nil && u.Host != "" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			return parts[0], strings.TrimSuffix(parts[1], ".git"), nil
		}
	}

	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) == 2 {
		return parts[0], strings.TrimSuffix(parts[1], ".git"), nil
	}

	return "", "", fmt.Errorf("invalid GitHub repository URL: %s", raw)
}
