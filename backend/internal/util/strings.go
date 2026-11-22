package util

import (
	"html"
	"regexp"
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

// subjectPrefixRe matches one reply or forward prefix at the start of a subject.
// It accepts a counter ("Re[2]:"), a missing space after the colon, and the
// usual prefixes of non-English mail clients.
var subjectPrefixRe = regexp.MustCompile(`(?i)^\s*(re|aw|sv|vs|antw|odp|res|fwd?|wg|tr|rv|enc)\s*(?:\[\d+\]|\(\d+\))?\s*[:：]\s*`)

var replyPrefixes = map[string]bool{
	"re": true, "aw": true, "sv": true, "vs": true, "antw": true, "odp": true, "res": true,
}

// CleanSubjectLine strips reply and forward prefixes and collapses whitespace.
// Migration 034 uses the same rule in SQL. Keep the two in step.
func CleanSubjectLine(subject string) string {
	s := subject
	for {
		loc := subjectPrefixRe.FindStringIndex(s)
		if loc == nil {
			break
		}
		s = s[loc[1]:]
	}
	return strings.Join(strings.Fields(s), " ")
}

// HasReplyPrefix reports whether the subject starts with a reply prefix.
// It looks through leading forward prefixes ("Fwd: Re: x" is a reply).
func HasReplyPrefix(subject string) bool {
	s := subject
	for {
		m := subjectPrefixRe.FindStringSubmatchIndex(s)
		if m == nil {
			return false
		}
		if replyPrefixes[strings.ToLower(s[m[2]:m[3]])] {
			return true
		}
		s = s[m[1]:]
	}
}

var msgIDRe = regexp.MustCompile(`<[^<>]+>`)

// NormalizeMsgID returns one Message-ID in the form "<id>", or "" if s has
// no id. It removes folding whitespace and takes the first id when s has
// more than one.
func NormalizeMsgID(s string) string {
	ids := ParseMsgIDs(s)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// ParseMsgIDs returns all Message-IDs in a header value (In-Reply-To or
// References), each in the form "<id>", without duplicates.
func ParseMsgIDs(s string) []string {
	var raw []string
	if m := msgIDRe.FindAllString(s, -1); len(m) > 0 {
		raw = m
	} else {
		// No angle brackets: each token is one id.
		raw = strings.Fields(strings.NewReplacer("<", " ", ">", " ", ",", " ").Replace(s))
	}
	seen := make(map[string]bool, len(raw))
	ids := make([]string, 0, len(raw))
	for _, r := range raw {
		inner := strings.Join(strings.Fields(strings.Trim(r, "<>")), "")
		if inner == "" {
			continue
		}
		id := "<" + inner + ">"
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// NormalizeAddress returns the bare lowercase address from "Name <addr>" or
// from a bare address.
func NormalizeAddress(raw string) string {
	s := raw
	if i := strings.LastIndex(s, "<"); i != -1 {
		if j := strings.Index(s[i:], ">"); j > 0 {
			s = s[i+1 : i+j]
		}
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeAddresses returns the bare lowercase addresses of all lists,
// without duplicates and without empty values.
func NormalizeAddresses(lists ...[]string) []string {
	seen := make(map[string]bool)
	out := []string{}
	for _, list := range lists {
		for _, raw := range list {
			a := NormalizeAddress(raw)
			if a == "" || seen[a] {
				continue
			}
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}
