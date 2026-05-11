package picker

import "testing"

func TestThemeByName_DefaultAndEmpty(t *testing.T) {
	for _, name := range []string{"", "default"} {
		_, ok := ThemeByName("", name)
		if !ok {
			t.Errorf("ThemeByName(%q) ok=false, want true", name)
		}
	}
}

func TestThemeByName_UnknownFallsBackSilently(t *testing.T) {
	theme, ok := ThemeByName("", "no-such-theme-xyz")
	if ok {
		t.Error("ThemeByName for unknown theme returned ok=true")
	}
	// Fallback theme must be usable: Render must not panic and must not return empty.
	if got := theme.Title.Render("hello"); got == "" {
		t.Error("fallback theme returned empty render for non-empty input")
	}
}


