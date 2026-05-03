package encoder

import "testing"

func TestEncode(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/private/tmp/test.dir/foo", "-private-tmp-test-dir-foo"},
		{"/Users/fikret/code/foo", "-Users-fikret-code-foo"},
		{"/home/ubuntu/dev/foo", "-home-ubuntu-dev-foo"},
		{"/Users/fikret", "-Users-fikret"},
		{"/a.b.c/d", "-a-b-c-d"},
		{"", ""},
	}
	for _, c := range cases {
		got := Encode(c.in)
		if got != c.want {
			t.Errorf("Encode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
