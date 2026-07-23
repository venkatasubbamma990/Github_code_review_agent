package github

import (
	"strings"
	"testing"

	"codereviewagent/internal/models"
)

func TestBuildCheckReport_Success(t *testing.T) {
	r := &models.ReviewResult{
		Quality: models.QualityScore{Overall: 85, Summary: "Looks good"},
		Findings: []models.Finding{
			{Severity: models.SeverityLow, Title: "nit"},
		},
	}
	report := BuildCheckReport(r)
	if report.Conclusion != ConclusionSuccess {
		t.Fatalf("want success, got %s", report.Conclusion)
	}
	if report.StatusState != "success" {
		t.Fatalf("want status success, got %s", report.StatusState)
	}
	if !strings.Contains(report.Title, "Passed") {
		t.Errorf("title = %q", report.Title)
	}
}

func TestBuildCheckReport_NeutralOnHigh(t *testing.T) {
	r := &models.ReviewResult{
		Quality: models.QualityScore{Overall: 80, Summary: "ok"},
		Findings: []models.Finding{
			{Severity: models.SeverityHigh, Title: "issue"},
		},
	}
	report := BuildCheckReport(r)
	if report.Conclusion != ConclusionNeutral {
		t.Fatalf("want neutral, got %s", report.Conclusion)
	}
	if report.StatusState != "success" {
		t.Fatalf("neutral should map to success status, got %s", report.StatusState)
	}
}

func TestBuildCheckReport_NeutralOnLowScore(t *testing.T) {
	r := &models.ReviewResult{
		Quality: models.QualityScore{Overall: 60},
	}
	report := BuildCheckReport(r)
	if report.Conclusion != ConclusionNeutral {
		t.Fatalf("want neutral, got %s", report.Conclusion)
	}
}

func TestBuildCheckReport_FailureOnCritical(t *testing.T) {
	r := &models.ReviewResult{
		Quality: models.QualityScore{Overall: 90},
		Findings: []models.Finding{
			{Severity: models.SeverityCritical, Title: "vuln"},
		},
	}
	report := BuildCheckReport(r)
	if report.Conclusion != ConclusionFailure {
		t.Fatalf("want failure, got %s", report.Conclusion)
	}
	if report.StatusState != "failure" {
		t.Fatalf("want status failure, got %s", report.StatusState)
	}
}

func TestBuildCheckReport_FailureOnLowScore(t *testing.T) {
	r := &models.ReviewResult{
		Quality: models.QualityScore{Overall: 40},
	}
	report := BuildCheckReport(r)
	if report.Conclusion != ConclusionFailure {
		t.Fatalf("want failure, got %s", report.Conclusion)
	}
}

func TestTruncateASCII(t *testing.T) {
	got := truncateASCII(strings.Repeat("a", 200), 140)
	if len(got) != 140 {
		t.Fatalf("len=%d want 140", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ... suffix, got %q", got[len(got)-3:])
	}
}
