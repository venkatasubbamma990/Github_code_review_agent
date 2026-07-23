package github

import (
	"strings"
	"testing"
)

func TestExtractIssueRefs(t *testing.T) {
	body := "Fixes #12 and relates to #34. Also see close #12 again."
	got := ExtractIssueRefs(body)
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 unique refs", got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "#12") || !strings.Contains(joined, "#34") {
		t.Fatalf("unexpected refs: %v", got)
	}
}

func TestPRContextFormatRaw(t *testing.T) {
	p := &PRContext{
		Number:       3,
		Title:        "Add caching",
		Author:       "alice",
		BaseBranch:   "main",
		HeadBranch:   "feat/cache",
		Labels:       []string{"enhancement"},
		IssueRefs:    []string{"#9"},
		ChangedFiles: []string{"cache.go"},
		Body:         "Implements Redis cache.",
		Comments:     []string{"bob: LGTM idea"},
	}
	raw := p.FormatRaw()
	for _, want := range []string{"PR #3", "alice", "enhancement", "#9", "cache.go", "Redis", "bob:"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("missing %q in:\n%s", want, raw)
		}
	}
}
