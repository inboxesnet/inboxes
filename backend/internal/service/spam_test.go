package service

import (
	"math"
	"testing"
)

const scoreEpsilon = 0.001

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < scoreEpsilon
}

func TestClassifySpam_CleanEmail(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=pass dkim=pass dmarc=pass",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body text")
	if result.IsSpam {
		t.Errorf("ClassifySpam(clean): got IsSpam=true, want false")
	}
	if !approxEqual(result.Score, 0) {
		t.Errorf("ClassifySpam(clean): got score %f, want 0", result.Score)
	}
	if len(result.Reasons) != 0 {
		t.Errorf("ClassifySpam(clean): got %d reasons, want 0", len(result.Reasons))
	}
}

func TestClassifySpam_SPFSoftfail(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=softfail dkim=pass dmarc=pass",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.2) {
		t.Errorf("ClassifySpam(SPFSoftfail): got score %f, want 0.2", result.Score)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "SPF check failed" {
		t.Errorf("ClassifySpam(SPFSoftfail): got reasons %v, want [SPF check failed]", result.Reasons)
	}
}

func TestClassifySpam_SPFFail(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=fail dkim=pass dmarc=pass",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.2) {
		t.Errorf("ClassifySpam(SPFFail): got score %f, want 0.2", result.Score)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "SPF check failed" {
		t.Errorf("ClassifySpam(SPFFail): got reasons %v, want [SPF check failed]", result.Reasons)
	}
}

func TestClassifySpam_DKIMFail(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=pass dkim=fail dmarc=pass",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.2) {
		t.Errorf("ClassifySpam(DKIMFail): got score %f, want 0.2", result.Score)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "DKIM check failed" {
		t.Errorf("ClassifySpam(DKIMFail): got reasons %v, want [DKIM check failed]", result.Reasons)
	}
}

func TestClassifySpam_DMARCFail(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=pass dkim=pass dmarc=fail",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.2) {
		t.Errorf("ClassifySpam(DMARCFail): got score %f, want 0.2", result.Score)
	}
	if len(result.Reasons) != 1 || result.Reasons[0] != "DMARC check failed" {
		t.Errorf("ClassifySpam(DMARCFail): got reasons %v, want [DMARC check failed]", result.Reasons)
	}
}

func TestClassifySpam_AllAuthFail(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=fail dkim=fail dmarc=fail",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.6) {
		t.Errorf("ClassifySpam(AllAuthFail): got score %f, want 0.6", result.Score)
	}
	if len(result.Reasons) != 3 {
		t.Errorf("ClassifySpam(AllAuthFail): got %d reasons, want 3", len(result.Reasons))
	}
	if result.IsSpam {
		t.Errorf("ClassifySpam(AllAuthFail): got IsSpam=true, want false (0.6 < 0.7)")
	}
}

func TestClassifySpam_UpstreamSpamStatus(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"X-Spam-Status": "Yes",
		"Message-ID":    "<abc@example.com>",
		"Date":          "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.3) {
		t.Errorf("ClassifySpam(UpstreamSpamStatus): got score %f, want 0.3", result.Score)
	}
}

func TestClassifySpam_UpstreamSpamFlag(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"X-Spam-Flag": "YES",
		"Message-ID":  "<abc@example.com>",
		"Date":        "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.3) {
		t.Errorf("ClassifySpam(UpstreamSpamFlag): got score %f, want 0.3", result.Score)
	}
}

func TestClassifySpam_MissingMessageID(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Date": "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.1) {
		t.Errorf("ClassifySpam(MissingMessageID): got score %f, want 0.1", result.Score)
	}
}

func TestClassifySpam_MissingDate(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Message-ID": "<abc@example.com>",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.1) {
		t.Errorf("ClassifySpam(MissingDate): got score %f, want 0.1", result.Score)
	}
}

func TestClassifySpam_NoreplyFrom(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Message-ID": "<abc@example.com>",
		"Date":       "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "noreply@something.com", "Hello", "Normal body")
	if !approxEqual(result.Score, 0.05) {
		t.Errorf("ClassifySpam(NoreplyFrom): got score %f, want 0.05", result.Score)
	}
}

func TestClassifySpam_SuspiciousSubjectKeyword(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Message-ID": "<abc@example.com>",
		"Date":       "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "This is urgent please read", "Normal body")
	if !approxEqual(result.Score, 0.1) {
		t.Errorf("ClassifySpam(SuspiciousSubjectKeyword): got score %f, want 0.1", result.Score)
	}
	found := false
	for _, r := range result.Reasons {
		if r == "Suspicious keyword in subject: urgent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ClassifySpam(SuspiciousSubjectKeyword): expected reason containing 'Suspicious keyword in subject', got %v", result.Reasons)
	}
}

func TestClassifySpam_AllCapsSubject(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Message-ID": "<abc@example.com>",
		"Date":       "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "THIS IS A LONG SUBJECT LINE", "Normal body")
	if !approxEqual(result.Score, 0.15) {
		t.Errorf("ClassifySpam(AllCapsSubject): got score %f, want 0.15", result.Score)
	}
}

func TestClassifySpam_ShortAllCapsSubject(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Message-ID": "<abc@example.com>",
		"Date":       "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "HELLO", "Normal body")
	if !approxEqual(result.Score, 0) {
		t.Errorf("ClassifySpam(ShortAllCapsSubject): got score %f, want 0", result.Score)
	}
}

func TestClassifySpam_SuspiciousBodyContent(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Message-ID": "<abc@example.com>",
		"Date":       "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Hello", "Dear Winner, you have been selected")
	if !approxEqual(result.Score, 0.15) {
		t.Errorf("ClassifySpam(SuspiciousBodyContent): got score %f, want 0.15", result.Score)
	}
}

func TestClassifySpam_DefiniteSpam(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=fail dkim=fail dmarc=fail",
		"X-Spam-Status":         "Yes",
		"Message-ID":            "<abc@example.com>",
		"Date":                  "Mon, 1 Jan 2024 00:00:00 +0000",
	}
	result := ClassifySpam(headers, "user@example.com", "Act Now - urgent offer!", "Normal body")
	if result.Score < 0.7-scoreEpsilon {
		t.Errorf("ClassifySpam(DefiniteSpam): got score %f, want >= 0.7", result.Score)
	}
	if !result.IsSpam {
		t.Errorf("ClassifySpam(DefiniteSpam): got IsSpam=false, want true")
	}
}

func TestClassifySpam_ScoreCappedAt1(t *testing.T) {
	t.Parallel()
	headers := map[string]string{
		"Authentication-Results": "spf=fail dkim=fail dmarc=fail",
		"X-Spam-Status":         "Yes",
		"X-Spam-Flag":           "YES",
	}
	result := ClassifySpam(headers, "noreply@evil.com", "URGENT CLAIM YOUR PRIZE NOW", "Dear winner, you have been selected")
	if !approxEqual(result.Score, 1.0) {
		t.Errorf("ClassifySpam(ScoreCappedAt1): got score %f, want 1.0", result.Score)
	}
}

func TestClassifySpam_NilHeaders(t *testing.T) {
	t.Parallel()
	result := ClassifySpam(nil, "noreply@something.com", "urgent offer", "dear winner, act now")
	if approxEqual(result.Score, 0) {
		t.Errorf("ClassifySpam(NilHeaders): got score 0, want >0 from heuristics")
	}
}

func TestClassifySpam_EmptyEverything(t *testing.T) {
	t.Parallel()
	result := ClassifySpam(map[string]string{}, "", "", "")
	if result.IsSpam {
		t.Errorf("ClassifySpam(EmptyEverything): got IsSpam=true, want false")
	}
}
