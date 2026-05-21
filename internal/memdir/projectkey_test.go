package memdir

import "testing"

func TestProjectKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"/Users/johnny/lab/evva", "-Users-johnny-lab-evva"},
		{"/home/alice/work/api", "-home-alice-work-api"},
		{"/", ""}, // root: leading dash trimmed to "" since there's nothing else
		{"/a", "-a"},
		{"/a//b/../c", "-a-c"}, // filepath.Clean collapses
		{"/a/b/", "-a-b"},      // trailing slash trimmed
		{"C:\\Users\\Alice\\proj", "C-Users-Alice-proj"},
	}
	for _, tc := range cases {
		got := ProjectKey(tc.in)
		if got != tc.want {
			t.Errorf("ProjectKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
