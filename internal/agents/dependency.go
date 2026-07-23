package agents

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"codereviewagent/internal/llm"
)

// DependencyAgent reviews dependency manifests and dependency-related risk.
type DependencyAgent struct {
	base *BaseAgent
	log  *zap.Logger
}

func NewDependencyAgent(client *llm.Client, log *zap.Logger) Agent {
	return &DependencyAgent{
		base: newBaseAgent(
			AgentDependency,
			"dependency",
			`Review third-party dependencies and package manifests for risk and hygiene.

Focus ONLY on:
- New, removed, or upgraded dependencies in manifests (go.mod, package.json, requirements.txt, Cargo.toml, etc.)
- Suspicious, abandoned, or overly broad dependency additions
- Version pinning / floating version risks (*, latest, unpinned ranges)
- Duplicate or conflicting dependency declarations
- Unnecessary direct dependencies that duplicate stdlib or existing modules
- Supply-chain red flags (typosquatting-like names, unexpected publishers — flag as hypotheses)
- Missing lockfile updates when manifests change
- License / compliance concerns when obvious from the change

Use the Context Agent briefing when present to judge whether dependency changes match the PR intent.

Do NOT report:
- Application logic bugs (Bug agent)
- General code quality / style (Quality / Style agents)
- Runtime performance of app code (Performance agent)
- Missing unit tests (Test agent)
- Generic OWASP app vulnerabilities unrelated to packages (Security agent)

If no dependency manifests or dependency-related imports are in scope, return a high score,
empty findings, and note that no dependency changes were reviewed.`,
			client,
			log,
		),
		log: log.Named("dependency"),
	}
}

func (a *DependencyAgent) Name() AgentName {
	return AgentDependency
}

func (a *DependencyAgent) Analyze(ctx context.Context, input ReviewInput) (*AgentOutput, error) {
	manifests := collectDependencyManifests(input)
	enriched := input
	if len(manifests) > 0 {
		enriched.ExtraContext = joinContext(input.ExtraContext, formatManifestHints(manifests))
		a.log.Info("dependency manifests detected", zap.Int("count", len(manifests)))
	} else {
		a.log.Info("no dependency manifests in review scope")
	}
	return a.base.Analyze(ctx, enriched)
}

type dependencyFile struct {
	Path     string
	Language string
	Content  string
}

func collectDependencyManifests(input ReviewInput) []dependencyFile {
	var out []dependencyFile
	seen := map[string]struct{}{}

	add := func(path, language, content string) {
		path = strings.TrimSpace(path)
		if path == "" || content == "" {
			return
		}
		key := strings.ToLower(filepath.ToSlash(path))
		if _, ok := seen[key]; ok {
			return
		}
		if !isDependencyManifestPath(path) {
			return
		}
		seen[key] = struct{}{}
		out = append(out, dependencyFile{Path: path, Language: language, Content: content})
	}

	for _, f := range input.Files {
		add(f.Path, f.Language, f.Content)
	}
	if input.FilePath != "" && input.Code != "" {
		add(input.FilePath, input.Language, input.Code)
	}
	return out
}

func isDependencyManifestPath(path string) bool {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	switch base {
	case "go.mod", "go.sum",
		"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "npm-shrinkwrap.json",
		"requirements.txt", "pipfile", "pipfile.lock", "poetry.lock", "pyproject.toml",
		"cargo.toml", "cargo.lock",
		"gemfile", "gemfile.lock",
		"composer.json", "composer.lock",
		"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts":
		return true
	}
	if strings.HasSuffix(base, ".csproj") || strings.HasSuffix(base, ".fsproj") {
		return true
	}
	return false
}

func formatManifestHints(files []dependencyFile) string {
	var b strings.Builder
	b.WriteString("Dependency manifests detected in this review (prioritize these):\n")
	for _, f := range files {
		b.WriteString(fmt.Sprintf("\n### Manifest: %s\n```\n%s\n```\n", f.Path, llm.Truncate(f.Content, 12000)))
	}
	return b.String()
}

func joinContext(existing, extra string) string {
	existing = strings.TrimSpace(existing)
	extra = strings.TrimSpace(extra)
	if existing == "" {
		return extra
	}
	if extra == "" {
		return existing
	}
	return existing + "\n\n" + extra
}
