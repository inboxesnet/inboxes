package worker

import "testing"

func TestMapResendEvent_Sent(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent("email.sent"); got != "sent" {
		t.Errorf("mapResendEvent(email.sent) = %q, want %q", got, "sent")
	}
}

func TestMapResendEvent_Delivered(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent("email.delivered"); got != "delivered" {
		t.Errorf("mapResendEvent(email.delivered) = %q, want %q", got, "delivered")
	}
}

func TestMapResendEvent_Bounced(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent("email.bounced"); got != "bounced" {
		t.Errorf("mapResendEvent(email.bounced) = %q, want %q", got, "bounced")
	}
}

func TestMapResendEvent_Complained(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent("email.complained"); got != "complained" {
		t.Errorf("mapResendEvent(email.complained) = %q, want %q", got, "complained")
	}
}

func TestMapResendEvent_Empty(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent(""); got != "" {
		t.Errorf("mapResendEvent('') = %q, want empty", got)
	}
}

func TestMapResendEvent_Unknown(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent("email.clicked"); got != "" {
		t.Errorf("mapResendEvent(email.clicked) = %q, want empty", got)
	}
}

func TestMapResendEvent_Opened(t *testing.T) {
	t.Parallel()
	if got := mapResendEvent("email.opened"); got != "" {
		t.Errorf("mapResendEvent(email.opened) = %q, want empty", got)
	}
}
