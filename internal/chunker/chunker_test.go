package chunker

import (
	"strings"
	"testing"

	"codereviewagent/internal/agents"
)

func TestDetectLanguage(t *testing.T) {
	cases := map[string]string{
		"main.go":        "go",
		"app.py":         "python",
		"index.ts":       "javascript",
		"App.tsx":        "javascript",
		"Main.java":      "java",
		"lib.rs":         "rust",
		"config.yaml":    "yaml",
		"unknown.bin":    "text",
	}
	for path, want := range cases {
		if got := DetectLanguage(path); got != want {
			t.Errorf("DetectLanguage(%q)=%q want %q", path, got, want)
		}
	}
}

func TestGroupRespectsMaxBytes(t *testing.T) {
	chunks := []FileChunk{
		{FilePath: "a.go", Content: strings.Repeat("a", 40)},
		{FilePath: "b.go", Content: strings.Repeat("b", 40)},
		{FilePath: "c.go", Content: strings.Repeat("c", 40)},
	}
	groups := Group(chunks, 50)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
}

func TestGroupKeepsSmallFilesTogether(t *testing.T) {
	chunks := []FileChunk{
		{FilePath: "a.go", Content: "aaaa"},
		{FilePath: "b.go", Content: "bbbb"},
	}
	groups := Group(chunks, 50)
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("expected one group with 2 files, got %+v", groups)
	}
}

func TestToReviewInputPreservesDiffWithFiles(t *testing.T) {
	base := agents.ReviewInput{
		Diff: "--- a.go\n+func()",
		Files: []agents.SourceFile{
			{Path: "a.go", Content: "package main\nfunc()"},
		},
		Source: "github:acme/api#1",
	}
	group := []FileChunk{{FilePath: "a.go", Language: "go", Content: "package main\nfunc()"}}
	got := ToReviewInput(base, group, 0, 1)
	if got.Diff != base.Diff {
		t.Fatalf("diff overwritten: %q", got.Diff)
	}
	if len(got.Files) != 1 || got.Files[0].Content != "package main\nfunc()" {
		t.Fatalf("files not set: %+v", got.Files)
	}
}

func TestFromSourceFiles(t *testing.T) {
	chunks := FromSourceFiles([]agents.SourceFile{{Path: "x.py", Language: "python", Content: "print(1)"}})
	if len(chunks) != 1 || chunks[0].Language != "python" {
		t.Fatalf("unexpected chunks: %+v", chunks)
	}
}

func TestIsDependencyManifest(t *testing.T) {
	cases := map[string]bool{
		"go.mod":              true,
		"services/api/go.mod": true,
		"package.json":        true,
		"yarn.lock":           true,
		"requirements.txt":    true,
		"main.go":             false,
		"readme.md":           false,
	}
	for path, want := range cases {
		if got := IsDependencyManifest(path); got != want {
			t.Errorf("IsDependencyManifest(%q)=%v want %v", path, got, want)
		}
	}
}

func TestIsReviewableExtensionIncludesManifests(t *testing.T) {
	if !IsReviewableExtension("go.mod") {
		t.Fatal("go.mod should be reviewable")
	}
	if !IsReviewableExtension("package.json") {
		t.Fatal("package.json should be reviewable")
	}
}
