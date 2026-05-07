package cli

import (
	"testing"

	"github.com/erzz/boo/internal/picker"
)

func TestRepoNameFromRemoteURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/owner/repo":         "repo",
		"https://github.com/owner/repo.git":     "repo",
		"https://github.com/owner/repo.git/":    "repo",
		"git@github.com:owner/repo.git":         "repo",
		"git@github.com:owner/repo":             "repo",
		"":                                      "",
		"   ":                                   "",
		"not-a-url":                             "",
		"https://example.com/some-org/My.Repo/": "My.Repo",
	}
	for in, want := range cases {
		got := repoNameFromRemoteURL(in)
		if got != want {
			t.Errorf("repoNameFromRemoteURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateIntent(t *testing.T) {
	cases := []struct {
		name    string
		intent  *picker.NewProjectIntent
		wantErr bool
	}{
		{
			name:    "nil",
			intent:  nil,
			wantErr: true,
		},
		{
			name:    "empty name",
			intent:  &picker.NewProjectIntent{Dir: "/x"},
			wantErr: true,
		},
		{
			name:    "neither dir nor from",
			intent:  &picker.NewProjectIntent{Name: "p"},
			wantErr: true,
		},
		{
			name:    "dir only ok",
			intent:  &picker.NewProjectIntent{Name: "p", Dir: "/x"},
			wantErr: false,
		},
		{
			name:    "from only ok",
			intent:  &picker.NewProjectIntent{Name: "p", From: "https://x/y"},
			wantErr: false,
		},
		{
			name:    "whitespace name treated as empty",
			intent:  &picker.NewProjectIntent{Name: "  ", Dir: "/x"},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateIntent(c.intent)
			if (err != nil) != c.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestDefaultsToIntent_PreservesFields(t *testing.T) {
	d := picker.FormDefaults{
		Name:                "alpha",
		Dir:                 "/some/path",
		From:                "https://x",
		Template:            "dev",
		GitRemote:           "origin → ...",     // info-only; not in intent
		AlreadyRegisteredAs: "should-not-leak", // ditto
	}
	got := defaultsToIntent(d)
	if got.Name != "alpha" || got.Dir != "/some/path" || got.From != "https://x" || got.Template != "dev" {
		t.Fatalf("intent fields wrong: %+v", got)
	}
}

func TestFirstArg(t *testing.T) {
	if got := firstArg(nil); got != "" {
		t.Errorf("nil → %q, want empty", got)
	}
	if got := firstArg([]string{}); got != "" {
		t.Errorf("[] → %q, want empty", got)
	}
	if got := firstArg([]string{"only"}); got != "only" {
		t.Errorf("[only] → %q, want 'only'", got)
	}
	if got := firstArg([]string{"first", "second"}); got != "first" {
		t.Errorf("[first second] → %q, want 'first'", got)
	}
}
