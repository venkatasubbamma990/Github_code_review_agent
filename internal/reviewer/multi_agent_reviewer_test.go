package reviewer

import (
	"strings"
	"testing"

	ghclient "codereviewagent/internal/github"
)

func TestBuildCombinedDiff(t *testing.T) {
	files := []ghclient.SourceFile{
		{Path: "a.go", Content: "full file", Patch: "@@ -1 +1 @@\n+a"},
		{Path: "b.go", Content: "full only"},
	}
	diff := buildCombinedDiff(files)
	if diff == "" {
		t.Fatal("expected combined diff")
	}
	if !strings.Contains(diff, "--- a.go") || !strings.Contains(diff, "+a") {
		t.Fatalf("missing patch content: %s", diff)
	}
	if strings.Contains(diff, "--- b.go") {
		t.Fatal("files without Patch should be omitted from combined diff")
	}
}

func TestParsePRNumber(t *testing.T) {
	if parsePRNumber("github:acme/api#42") != 42 {
		t.Fatal("expected 42")
	}
	if parsePRNumber("repo:acme/api") != 0 {
		t.Fatal("expected 0")
	}
}
