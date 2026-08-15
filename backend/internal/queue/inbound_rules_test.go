package queue

import (
	"strings"
	"testing"
)

func TestIsAutomatedSender(t *testing.T) {
	t.Parallel()
	auto := []string{
		"mailer-daemon@example.com",
		"MAILER-DAEMON@googlemail.com",
		"postmaster@example.com",
		"no-reply@github.com",
		"noreply@stripe.com",
		"donotreply@bank.com",
		"bounce-123@lists.example.com",
	}
	for _, a := range auto {
		if !isAutomatedSender(a) {
			t.Errorf("expected %q detected as automated", a)
		}
	}
	human := []string{"harrison@example.com", "sales@company.com", "reply@shop.com"}
	for _, h := range human {
		if isAutomatedSender(h) {
			t.Errorf("expected %q detected as human", h)
		}
	}
}

func TestForwardBodyHTML(t *testing.T) {
	t.Parallel()
	body := forwardBodyHTML("alice@example.com", "Hello <world>", "<p>hi</p>", "")
	if !strings.Contains(body, "Forwarded message") {
		t.Error("expected forwarded banner")
	}
	if !strings.Contains(body, "Hello &lt;world&gt;") {
		t.Error("expected subject HTML-escaped in the banner")
	}
	if !strings.Contains(body, "<p>hi</p>") {
		t.Error("expected original body preserved")
	}

	plainOnly := forwardBodyHTML("a@b.c", "S", "", "line1\n<script>")
	if !strings.Contains(plainOnly, "&lt;script&gt;") {
		t.Error("expected plain body HTML-escaped")
	}
}
