package ghostty

import "testing"

func TestDetectGhosttyHost(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"empty", nil, false},
		{"resources dir set", map[string]string{"GHOSTTY_RESOURCES_DIR": "/x"}, true},
		{"bin dir set", map[string]string{"GHOSTTY_BIN_DIR": "/x"}, true},
		{"term program ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, true},
		{"term program other", map[string]string{"TERM_PROGRAM": "iTerm.app"}, false},
		{"unrelated vars only", map[string]string{"FOO": "bar"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) string { return tc.env[k] }
			if got := detectGhosttyHost(lookup); got != tc.want {
				t.Fatalf("detectGhosttyHost(%v) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}
