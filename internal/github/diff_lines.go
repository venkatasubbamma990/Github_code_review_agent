package github

import (
	"strconv"
	"strings"
)

// ParseReviewableLines returns the set of RIGHT-side line numbers that appear
// in a unified diff patch (additions and context). GitHub only accepts inline
// review comments on these lines.
func ParseReviewableLines(patch string) map[int]struct{} {
	lines := make(map[int]struct{})
	if patch == "" {
		return lines
	}

	newLine := 0
	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@") {
			newLine = parseHunkNewStart(raw)
			continue
		}
		if newLine == 0 {
			continue
		}
		if len(raw) == 0 {
			// Empty line in patch body is treated as context.
			lines[newLine] = struct{}{}
			newLine++
			continue
		}
		switch raw[0] {
		case '+':
			lines[newLine] = struct{}{}
			newLine++
		case ' ':
			lines[newLine] = struct{}{}
			newLine++
		case '-':
			// Deletion: does not advance the new-file line counter.
		case '\\':
			// "\ No newline at end of file"
		default:
			// Unknown — ignore.
		}
	}
	return lines
}

// parseHunkNewStart extracts the starting line on the new (RIGHT) side from a
// hunk header like "@@ -10,5 +12,7 @@".
func parseHunkNewStart(header string) int {
	// Find the '+' side: +<start> or +<start>,<count>
	plus := strings.Index(header, "+")
	if plus < 0 {
		return 0
	}
	rest := header[plus+1:]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return n
}
