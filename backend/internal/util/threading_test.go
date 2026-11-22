package util

import (
	"reflect"
	"testing"
)

func TestCleanSubjectLine_Table(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"Quote", "Quote"},
		{"Re: Quote", "Quote"},
		{"RE:Quote", "Quote"},
		{"Re : Quote", "Quote"},
		{"  Re: Quote  ", "Quote"},
		{"Re[2]: Quote", "Quote"},
		{"RE(3): Quote", "Quote"},
		{"AW: Quote", "Quote"},
		{"SV: Quote", "Quote"},
		{"Antw: Quote", "Quote"},
		{"WG: Quote", "Quote"},
		{"Fw: Quote", "Quote"},
		{"FW: Quote", "Quote"},
		{"Fwd: Quote", "Quote"},
		{"Re: Fwd: AW: Quote", "Quote"},
		{"Re: Fwd: Contract  draft", "Contract draft"},
		{"Re：Quote", "Quote"},
		{"Regarding: the offer", "Regarding: the offer"},
		{"Report: Q3", "Report: Q3"},
		{"Free: lunch", "Free: lunch"},
		{"Trade: update", "Trade: update"},
		{"Invoice Re: 42", "Invoice Re: 42"},
		{"Re:", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := CleanSubjectLine(c.in); got != c.want {
			t.Errorf("CleanSubjectLine(%q) = %q, want %q", c.in, got, c.want)
		}
		// A clean subject must stay the same when cleaned again.
		if again := CleanSubjectLine(c.want); again != c.want {
			t.Errorf("CleanSubjectLine(%q) is not stable: %q", c.want, again)
		}
	}
}

func TestHasReplyPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"Re: Quote", true},
		{"RE:Quote", true},
		{"AW: Quote", true},
		{"Re[2]: Quote", true},
		{"Fwd: Re: Quote", true},
		{"Fwd: Quote", false},
		{"FW: Quote", false},
		{"Quote", false},
		{"Invoice", false},
		{"Regarding: x", false},
		{"", false},
	}
	for _, c := range cases {
		if got := HasReplyPrefix(c.in); got != c.want {
			t.Errorf("HasReplyPrefix(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeMsgID(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"<abc@mail.example.com>", "<abc@mail.example.com>"},
		{"  <abc@mail.example.com>\r\n", "<abc@mail.example.com>"},
		{"abc@mail.example.com", "<abc@mail.example.com>"},
		{"<abc@mail.\r\n example.com>", "<abc@mail.example.com>"},
		{"<a@x> <b@x>", "<a@x>"},
		{"<CA+Mixed=Case@mail.gmail.com>", "<CA+Mixed=Case@mail.gmail.com>"},
		{"", ""},
		{"   ", ""},
		{"<>", ""},
	}
	for _, c := range cases {
		if got := NormalizeMsgID(c.in); got != c.want {
			t.Errorf("NormalizeMsgID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseMsgIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{"<a@x> <b@x>", []string{"<a@x>", "<b@x>"}},
		{"<a@x>\r\n\t<b@x>", []string{"<a@x>", "<b@x>"}},
		{"<a@x><b@x>", []string{"<a@x>", "<b@x>"}},
		{"<a@x>, <b@x>", []string{"<a@x>", "<b@x>"}},
		{"<a@x> <a@x>", []string{"<a@x>"}},
		{"a@x b@x", []string{"<a@x>", "<b@x>"}},
		{"", []string{}},
	}
	for _, c := range cases {
		if got := ParseMsgIDs(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseMsgIDs(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeAddress(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"alice@x.com", "alice@x.com"},
		{"Alice@X.COM", "alice@x.com"},
		{"  alice@x.com ", "alice@x.com"},
		{"Alice <Alice@X.com>", "alice@x.com"},
		{"\"Smith, Alice\" <alice@x.com>", "alice@x.com"},
		{"\"A <weird> name\" <alice@x.com>", "alice@x.com"},
		{"<alice@x.com>", "alice@x.com"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeAddress(c.in); got != c.want {
			t.Errorf("NormalizeAddress(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeAddresses(t *testing.T) {
	t.Parallel()
	got := NormalizeAddresses(
		[]string{"Alice <alice@x.com>", "BOB@x.com"},
		[]string{"alice@X.com", "", "carol@x.com"},
		nil,
	)
	want := []string{"alice@x.com", "bob@x.com", "carol@x.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizeAddresses = %v, want %v", got, want)
	}
	if got := NormalizeAddresses(); got == nil || len(got) != 0 {
		t.Errorf("NormalizeAddresses() = %v, want an empty non-nil slice", got)
	}
}
