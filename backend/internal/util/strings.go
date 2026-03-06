package util

import (
	"html"
	"strings"
)

// TruncateRunes HTML-unescapes s, then truncates to maxRunes runes.
func TruncateRunes(s string, maxRunes int) string {
	s = html.UnescapeString(s)
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return s
}

// CleanSubjectLine strips Re:/Fwd: prefixes (case-insensitive) and collapses whitespace.
func CleanSubjectLine(subject string) string {
	s := subject
	for {
		lower := strings.ToLower(s)
		if strings.HasPrefix(lower, "re: ") {
			s = s[4:]
		} else if strings.HasPrefix(lower, "fwd: ") {
			s = s[5:]
		} else {
			break
		}
	}
	parts := strings.Fields(strings.TrimSpace(s))
	return strings.Join(parts, " ")
}
