package chunker

import (
	"strings"

	"codereviewagent/internal/agents"
)

// FileChunk represents reviewable content for a single file or diff hunk group.
type FileChunk struct {
	FilePath string
	Language string
	Content  string
}

// Group splits chunks into batches that fit within maxBytes per review call.
func Group(chunks []FileChunk, maxBytes int) [][]FileChunk {
	if maxBytes <= 0 {
		maxBytes = 50000
	}
	if len(chunks) == 0 {
		return nil
	}

	var groups [][]FileChunk
	var current []FileChunk
	currentSize := 0

	for _, chunk := range chunks {
		size := len(chunk.Content)
		if size > maxBytes {
			if len(current) > 0 {
				groups = append(groups, current)
				current = nil
				currentSize = 0
			}
			for _, sub := range splitContent(chunk, maxBytes) {
				groups = append(groups, []FileChunk{sub})
			}
			continue
		}

		if currentSize+size > maxBytes && len(current) > 0 {
			groups = append(groups, current)
			current = nil
			currentSize = 0
		}
		current = append(current, chunk)
		currentSize += size
	}

	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// FromDiff parses a unified diff into per-file chunks.
func FromDiff(diff string) []FileChunk {
	sections := strings.Split(diff, "\n--- ")
	var chunks []FileChunk

	for i, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if i > 0 {
			section = "--- " + section
		}

		filePath := extractDiffFilePath(section)
		chunks = append(chunks, FileChunk{
			FilePath: filePath,
			Language: DetectLanguage(filePath),
			Content:  section,
		})
	}

	if len(chunks) == 0 && strings.TrimSpace(diff) != "" {
		chunks = append(chunks, FileChunk{
			FilePath: "diff",
			Language: "diff",
			Content:  diff,
		})
	}
	return chunks
}

// FromSourceFiles converts repository files to chunks.
func FromSourceFiles(files []agents.SourceFile) []FileChunk {
	chunks := make([]FileChunk, 0, len(files))
	for _, f := range files {
		chunks = append(chunks, FileChunk{
			FilePath: f.Path,
			Language: f.Language,
			Content:  f.Content,
		})
	}
	return chunks
}

// ToReviewInput builds a ReviewInput for a chunk group.
func ToReviewInput(base agents.ReviewInput, group []FileChunk, index, total int) agents.ReviewInput {
	input := base
	input.Files = nil
	input.ChunkIndex = index
	input.TotalChunks = total

	if len(group) == 1 && base.Diff == "" && base.Code == "" {
		input.FilePath = group[0].FilePath
		input.Language = group[0].Language
		input.Code = group[0].Content
	}

	for _, c := range group {
		input.Files = append(input.Files, agents.SourceFile{
			Path:     c.FilePath,
			Language: c.Language,
			Content:  c.Content,
		})
	}

	// Diff-only reviews rebuild Diff from the chunk group. When full files are
	// also present (PR reviews), keep the original combined patch diff.
	if base.Diff != "" && len(base.Files) == 0 {
		var b strings.Builder
		for _, c := range group {
			b.WriteString(c.Content)
			b.WriteString("\n")
		}
		input.Diff = b.String()
	} else if base.Diff != "" {
		input.Diff = base.Diff
	}

	return input
}

func splitContent(chunk FileChunk, maxBytes int) []FileChunk {
	lines := strings.Split(chunk.Content, "\n")
	var parts []FileChunk
	var b strings.Builder

	for _, line := range lines {
		if b.Len()+len(line)+1 > maxBytes && b.Len() > 0 {
			parts = append(parts, FileChunk{
				FilePath: chunk.FilePath,
				Language: chunk.Language,
				Content:  b.String(),
			})
			b.Reset()
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if b.Len() > 0 {
		parts = append(parts, FileChunk{
			FilePath: chunk.FilePath,
			Language: chunk.Language,
			Content:  b.String(),
		})
	}
	return parts
}

func extractDiffFilePath(section string) string {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--- ") {
			path := strings.TrimPrefix(line, "--- ")
			if idx := strings.Index(path, " ("); idx > 0 {
				path = path[:idx]
			}
			return strings.TrimSpace(path)
		}
	}
	return "unknown"
}

// DetectLanguage guesses language from file extension.
func DetectLanguage(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return "go"
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return "javascript"
	case strings.HasSuffix(lower, ".java"):
		return "java"
	case strings.HasSuffix(lower, ".rs"):
		return "rust"
	case strings.HasSuffix(lower, ".rb"):
		return "ruby"
	case strings.HasSuffix(lower, ".php"):
		return "php"
	case strings.HasSuffix(lower, ".cs"):
		return "csharp"
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return "yaml"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	case strings.HasSuffix(lower, ".sql"):
		return "sql"
	default:
		return "text"
	}
}

// IsReviewableExtension returns true for supported source file types.
func IsReviewableExtension(path string) bool {
	lang := DetectLanguage(path)
	return lang != "text" || strings.HasSuffix(strings.ToLower(path), ".md")
}
