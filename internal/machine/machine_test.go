package machine

import "testing"

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"MacBook-Pro.local": "macbook-pro",
		"oracle-vm-1":       "oracle-vm-1",
		"  Host With Space ": "host-with-space",
		"weird@chars!":      "weird-chars",
		"Host_underscore":   "host_underscore",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
