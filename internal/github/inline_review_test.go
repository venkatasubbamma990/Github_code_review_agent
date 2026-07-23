package github

import (
	"strings"
	"testing"

	"codereviewagent/internal/models"
)

func TestParseReviewableLines(t *testing.T) {
	patch := `@@ -1,3 +1,5 @@
 package main
 
+import "fmt"
+
 func main() {
-	old()
+	fmt.Println("hi")
 }`

	got := ParseReviewableLines(patch)

	// RIGHT-side lines: 1 (context), 2 (context blank), 3 (+import), 4 (+blank),
	// 5 (context func), 6 (+fmt.Println), 7 (context })
	want := []int{1, 2, 3, 4, 5, 6, 7}
	for _, line := range want {
		if _, ok := got[line]; !ok {
			t.Errorf("expected line %d to be reviewable; got %v", line, got)
		}
	}
	// Deleted "old()" was on old side only — should not invent a phantom line.
	if len(got) != len(want) {
		t.Errorf("got %d reviewable lines, want %d: %v", len(got), len(want), got)
	}
}

func TestParseReviewableLines_Empty(t *testing.T) {
	if got := ParseReviewableLines(""); len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestMapFindingsToInlineComments(t *testing.T) {
	patches := map[string]string{
		"main.go": `@@ -1,2 +1,3 @@
 package main
+func bad() {}
 func ok() {}
`,
	}

	findings := []models.Finding{
		{FilePath: "main.go", Line: 2, Severity: models.SeverityLow, Category: "style", Title: "naming", Description: "rename"},
		{FilePath: "main.go", Line: 2, Severity: models.SeverityCritical, Category: "security", Title: "vuln", Description: "fix", Suggestion: "fix it"},
		{FilePath: "main.go", Line: 99, Severity: models.SeverityHigh, Category: "quality", Title: "outside diff"},
		{FilePath: "other.go", Line: 1, Severity: models.SeverityHigh, Category: "quality", Title: "unknown file"},
		{FilePath: "", Line: 1, Severity: models.SeverityHigh, Category: "quality", Title: "no path"},
	}

	comments := MapFindingsToInlineComments(findings, patches)
	if len(comments) != 1 {
		t.Fatalf("expected 1 inline comment (deduped by path:line, critical wins sort then first kept), got %d: %+v", len(comments), comments)
	}
	if comments[0].Path != "main.go" || comments[0].Line != 2 {
		t.Errorf("unexpected comment location: %+v", comments[0])
	}
	if !strings.Contains(comments[0].Body, "CRITICAL") || !strings.Contains(comments[0].Body, "vuln") {
		t.Errorf("expected critical finding body, got: %s", comments[0].Body)
	}
	if !strings.Contains(comments[0].Body, "Suggestion") {
		t.Errorf("expected suggestion in body, got: %s", comments[0].Body)
	}
}

func TestMapFindingsToInlineComments_SeverityCap(t *testing.T) {
	var patchLines []string
	patchLines = append(patchLines, "@@ -1,0 +1,25 @@")
	for i := 0; i < 25; i++ {
		patchLines = append(patchLines, "+line")
	}
	patch := ""
	for i, l := range patchLines {
		if i > 0 {
			patch += "\n"
		}
		patch += l
	}

	findings := make([]models.Finding, 0, 25)
	for i := 1; i <= 25; i++ {
		sev := models.SeverityInfo
		if i <= 5 {
			sev = models.SeverityCritical
		}
		findings = append(findings, models.Finding{
			FilePath: "f.go", Line: i, Severity: sev, Category: "c", Title: "t",
		})
	}

	comments := MapFindingsToInlineComments(findings, map[string]string{"f.go": patch})
	if len(comments) != maxInlineComments {
		t.Fatalf("expected cap at %d, got %d", maxInlineComments, len(comments))
	}
	// First comments should be the critical ones (lines 1-5).
	for i := 0; i < 5; i++ {
		if comments[i].Line != i+1 {
			t.Errorf("comment %d: want line %d, got %d", i, i+1, comments[i].Line)
		}
	}
}
