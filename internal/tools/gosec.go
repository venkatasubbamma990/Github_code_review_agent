package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type gosecReport struct {
	Issues []gosecIssue `json:"Issues"`
}

type gosecIssue struct {
	Severity   string `json:"severity"`
	RuleID     string `json:"rule_id"`
	Details    string `json:"details"`
	File       string `json:"file"`
	Line       string `json:"line"`
	Code       string `json:"code"`
	Confidence string `json:"confidence"`
}

func (r *Runner) runGosec(ctx context.Context, dir string) ([]Finding, error) {
	if !r.GosecAvailable() {
		return nil, fmt.Errorf("gosec not found in PATH")
	}

	cmd := exec.CommandContext(ctx, r.gosecPath, "-fmt=json", "-quiet", "./...")
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		// gosec exits non-zero when issues are found
		if len(out) == 0 {
			return nil, err
		}
	}

	var report gosecReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, fmt.Errorf("parse gosec output: %w", err)
	}

	findings := make([]Finding, 0, len(report.Issues))
	for _, issue := range report.Issues {
		line := 0
		fmt.Sscanf(issue.Line, "%d", &line)
		findings = append(findings, Finding{
			Tool:        "gosec",
			Severity:    mapGosecSeverity(issue.Severity),
			Title:       issue.RuleID,
			Description: strings.TrimSpace(issue.Details + " " + issue.Code),
			FilePath:    trimTempPath(issue.File),
			Line:        line,
		})
	}
	return findings, nil
}

func mapGosecSeverity(s string) string {
	switch strings.ToUpper(s) {
	case "HIGH":
		return "high"
	case "MEDIUM":
		return "medium"
	case "LOW":
		return "low"
	default:
		return "info"
	}
}

func trimTempPath(path string) string {
	if idx := strings.Index(path, "codereview-scan-"); idx >= 0 {
		rest := path[idx:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			return rest[slash+1:]
		}
	}
	return path
}
