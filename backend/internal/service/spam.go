package service

import (
	"strings"
)

type SpamResult struct {
	IsSpam  bool     `json:"is_spam"`
	Score   float64  `json:"score"`
	Reasons []string `json:"reasons"`
}

// ClassifySpam scores an inbound email for spam likelihood.
// Uses header + heuristic analysis. Threshold: score >= 0.7 = spam.
func ClassifySpam(headers map[string]string, fromAddress, subject, bodyPlain string) SpamResult {
	var score float64
	var reasons []string

	// Check authentication results
	authResults := headers["Authentication-Results"]
	if authResults != "" {
		if strings.Contains(authResults, "spf=fail") || strings.Contains(authResults, "spf=softfail") {
			score += 0.2
			reasons = append(reasons, "SPF check failed")
		}
		if strings.Contains(authResults, "dkim=fail") {
			score += 0.2
			reasons = append(reasons, "DKIM check failed")
		}
		if strings.Contains(authResults, "dmarc=fail") {
			score += 0.2
			reasons = append(reasons, "DMARC check failed")
		}
	}

	// Check upstream spam headers
	if spamStatus := headers["X-Spam-Status"]; spamStatus != "" {
		if strings.HasPrefix(strings.ToLower(spamStatus), "yes") {
			score += 0.3
			reasons = append(reasons, "Upstream spam filter flagged")
		}
	}
	if spamFlag := headers["X-Spam-Flag"]; strings.ToLower(spamFlag) == "yes" {
		score += 0.3
		reasons = append(reasons, "X-Spam-Flag: YES")
	}

	// Missing or malformed headers
	if headers["Message-ID"] == "" {
		score += 0.1
		reasons = append(reasons, "Missing Message-ID header")
	}
	if headers["Date"] == "" {
		score += 0.1
		reasons = append(reasons, "Missing Date header")
	}

	// Suspicious From patterns
	fromLower := strings.ToLower(fromAddress)
	suspiciousFromPatterns := []string{
		"noreply@", "no-reply@", "mailer-daemon@",
	}
	for _, pattern := range suspiciousFromPatterns {
		if strings.Contains(fromLower, pattern) {
			// These are common for automated mail but not necessarily spam
			// Only mild score bump
			score += 0.05
			break
		}
	}

	// Subject-based heuristics
	subjectLower := strings.ToLower(subject)
	spamSubjectKeywords := []string{
		"urgent", "act now", "limited time", "congratulations",
		"winner", "claim your", "free money", "$$", "100% free",
		"click here",
	}
	for _, kw := range spamSubjectKeywords {
		if strings.Contains(subjectLower, kw) {
			score += 0.1
			reasons = append(reasons, "Suspicious keyword in subject: "+kw)
			break // Only count once
		}
	}

	// All caps subject
	if len(subject) > 10 && subject == strings.ToUpper(subject) {
		score += 0.15
		reasons = append(reasons, "Subject is all caps")
	}

	// Body-based heuristics
	bodyLower := strings.ToLower(bodyPlain)
	spamBodyIndicators := []string{
		"this is not spam",
		"you have been selected",
		"act immediately",
		"dear winner",
		"nigerian prince",
	}
	for _, indicator := range spamBodyIndicators {
		if strings.Contains(bodyLower, indicator) {
			score += 0.15
			reasons = append(reasons, "Suspicious content in body")
			break
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return SpamResult{
		IsSpam:  score >= 0.7,
		Score:   score,
		Reasons: reasons,
	}
}

// IsBounceNotification checks whether an email is an automated bounce/delivery failure notification.
func IsBounceNotification(from string, headers map[string]string) bool {
	fromLower := strings.ToLower(from)

	// Check From address for bounce senders
	bouncePatterns := []string{"mailer-daemon@", "postmaster@"}
	for _, pattern := range bouncePatterns {
		if strings.Contains(fromLower, pattern) {
			return true
		}
	}

	// Check bounce-specific headers
	if headers != nil {
		if v, ok := headers["auto-submitted"]; ok {
			vLower := strings.ToLower(v)
			if vLower == "auto-replied" || vLower == "auto-generated" {
				return true
			}
		}
		if _, ok := headers["x-failed-recipients"]; ok {
			return true
		}
	}

	return false
}
