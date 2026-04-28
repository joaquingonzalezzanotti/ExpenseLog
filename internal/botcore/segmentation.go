package botcore

import "strings"

// SplitBatchTextCandidates splits a chat message into candidate movement lines.
// The strategy is conservative to avoid accidental over-splitting.
func SplitBatchTextCandidates(raw string) []string {
	text := strings.ReplaceAll(raw, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	lines := splitAndClean(text, "\n")
	if len(lines) >= 2 {
		return lines
	}

	semicolon := splitAndClean(text, ";")
	if len(semicolon) >= 2 {
		return semicolon
	}

	return []string{text}
}

func splitAndClean(text string, sep string) []string {
	parts := strings.Split(text, sep)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := normalizeCandidateLine(part)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	return out
}

func normalizeCandidateLine(raw string) string {
	line := strings.TrimSpace(raw)
	line = strings.TrimPrefix(line, "-")
	line = strings.TrimPrefix(line, "•")
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	return line
}
