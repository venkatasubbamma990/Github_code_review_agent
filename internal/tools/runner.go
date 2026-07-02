package tools

import (
	"context"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// Runner executes static analysis tools against source files.
type Runner struct {
	gosecPath   string
	semgrepPath string
	log         *zap.Logger
}

func NewRunner(gosecPath, semgrepPath string, log *zap.Logger) *Runner {
	if gosecPath == "" {
		gosecPath = "gosec"
	}
	if semgrepPath == "" {
		semgrepPath = "semgrep"
	}
	return &Runner{
		gosecPath:   gosecPath,
		semgrepPath: semgrepPath,
		log:         log.Named("tools"),
	}
}

// Scan runs available tools against the given files (path → content).
func (r *Runner) Scan(ctx context.Context, files map[string][]byte) []Finding {
	if len(files) == 0 {
		return nil
	}

	dir, err := os.MkdirTemp("", "codereview-scan-*")
	if err != nil {
		r.log.Warn("failed to create temp dir for scanning", zap.Error(err))
		return nil
	}
	defer os.RemoveAll(dir)

	for path, content := range files {
		fullPath := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(fullPath, content, 0o644)
	}

	var findings []Finding

	if gosecFindings, err := r.runGosec(ctx, dir); err != nil {
		r.log.Debug("gosec skipped or failed", zap.Error(err))
	} else {
		findings = append(findings, gosecFindings...)
	}

	if semgrepFindings, err := r.runSemgrep(ctx, dir); err != nil {
		r.log.Debug("semgrep skipped or failed", zap.Error(err))
	} else {
		findings = append(findings, semgrepFindings...)
	}

	r.log.Info("tool scan completed", zap.Int("findings", len(findings)))
	return findings
}

func (r *Runner) GosecAvailable() bool {
	_, err := execLookPath(r.gosecPath)
	return err == nil
}

func (r *Runner) SemgrepAvailable() bool {
	_, err := execLookPath(r.semgrepPath)
	return err == nil
}
