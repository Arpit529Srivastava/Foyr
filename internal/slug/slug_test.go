package slug

import "testing"

func TestFrom(t *testing.T) {
	for in, want := range map[string]string{
		"octo-dev/foyr-hello": "foyr-hello",
		"Acme/My_Cool App":    "my-cool-app",
		"o/CRM.Dashboard--v2": "crm-dashboard-v2",
		"o/x":                 "x-app",
		"o/api":               "api-app",
		"o/--":                "my-app",
		"o/a-very-long-repository-name-that-keeps-going-on": "a-very-long-repository-name-that-keeps-g",
		"o/日本語": "my-app",
	} {
		got := From(in)
		if got != want {
			t.Errorf("From(%q) = %q, want %q", in, got, want)
		}
		if !Valid(got) {
			t.Errorf("From(%q) = %q is not a valid slug", in, got)
		}
	}
}

func TestValid(t *testing.T) {
	for s, want := range map[string]bool{
		"foyr-hello": true, "abc": true, "ab": false, "-abc": false, "abc-": false, "ABC": false,
		"a_b_c": false, "admin": false, "foyr": false, "auth": false,
		"a234567890123456789012345678901234567890": true, "a2345678901234567890123456789012345678901": false,
	} {
		if got := Valid(s); got != want {
			t.Errorf("Valid(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestCandidates(t *testing.T) {
	c := Candidates("o/foyr-hello")
	if c[0] != "foyr-hello" || c[1] != "foyr-hello-2" || len(c) != 9 {
		t.Fatalf("candidates = %v", c)
	}
	for _, s := range Candidates("o/a-very-long-repository-name-that-keeps-going-on") {
		if !Valid(s) {
			t.Errorf("candidate %q is not valid", s)
		}
	}
}
