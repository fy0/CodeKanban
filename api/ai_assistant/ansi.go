package ai_assistant

import (
	"regexp"
	"strings"
)

// ANSI escape sequence patterns
var (
	// Matches ANSI escape sequences including CSI, OSC, and simple escapes
	ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[>=\[\]]|\x1b[()][AB012]`)

	// Matches control characters (except newline, tab, and carriage return)
	controlCharsPattern = regexp.MustCompile(`[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]`)

	// Matches carriage return followed by text (overwrites previous line)
	crOverwritePattern = regexp.MustCompile(`\r[^\n]`)
)

// StripANSI removes ANSI escape sequences and control characters from text.
// This is essential for reliable text matching in terminal output.
func StripANSI(text string) string {
	if text == "" {
		return ""
	}

	// Remove ANSI escape sequences
	cleaned := ansiPattern.ReplaceAllString(text, "")

	// Handle carriage returns (keep the last overwritten text)
	cleaned = handleCarriageReturns(cleaned)

	// Remove other control characters
	cleaned = controlCharsPattern.ReplaceAllString(cleaned, "")

	return cleaned
}

// handleCarriageReturns simulates terminal behavior where \r overwrites the current line
func handleCarriageReturns(text string) string {
	if !strings.Contains(text, "\r") {
		return text
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.Contains(line, "\r") {
			// Split by \r and keep only the last segment (simulating overwrite)
			segments := strings.Split(line, "\r")
			lines[i] = segments[len(segments)-1]
		}
	}

	return strings.Join(lines, "\n")
}

// CleanLine removes ANSI sequences and trims whitespace from a single line
func CleanLine(line string) string {
	cleaned := StripANSI(line)
	return strings.TrimSpace(cleaned)
}

// ContainsClean checks if the cleaned version of text contains the substring
func ContainsClean(text, substr string) bool {
	cleanText := strings.ToLower(StripANSI(text))
	cleanSubstr := strings.ToLower(substr)
	return strings.Contains(cleanText, cleanSubstr)
}
