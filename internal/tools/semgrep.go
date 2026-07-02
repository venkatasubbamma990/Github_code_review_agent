package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type semgrepReport struct {
	Results []semgrepResult `json:"results"`
}

type semgrepResult struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Extra   struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
		Lines    string `json:"lines"`
	} `json:"extra"`
	Start struct {
		Line int `json:"line"`
	} `json:"start"`
}

func (r *Runner) runSemgrep(ctx context.Context, dir string) ([]Finding, error) {
	if !r.SemgrepAvailable() {
		return nil, fmt.Errorf("semgrep not found in PATH")
	}

	cmd := exec.CommandContext(ctx, r.semgrepPath,
		"--json",
		"--quiet",
		"--config", "auto",
		".",
	)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return nil, err
	}

	var report semgrepReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, fmt.Errorf("parse semgrep output: %w", err)
	}

	findings := make([]Finding, 0, len(report.Results))
	for _, res := range report.Results {
		findings = append(findings, Finding{
			Tool:        "semgrep",
			Severity:    mapSemgrepSeverity(res.Extra.Severity),
			Title:       res.CheckID,
			Description: res.Extra.Message,
			FilePath:    trimTempPath(res.Path),
			Line:        res.Start.Line,
		})
	}
	return findings, nil
}

func mapSemgrepSeverity(s string) string {
	switch strings.ToUpper(s) {
	case "ERROR":
		return "high"
	case "WARNING":
		return "medium"
	case "INFO":
		return "low"
	default:
		return "medium"
	}
}
