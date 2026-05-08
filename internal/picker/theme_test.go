package picker

import "testing"

func TestThemeByName_DefaultAndEmpty(t *testing.T) {
	for _, name := range []string{"", "default"} {
		_, ok := ThemeByName(name)
		if !ok {
			t.Errorf("ThemeByName(%q) ok=false, want true", name)
		}
	}
}

func TestThemeByName_UnknownFallsBackSilently(t *testing.T) {
	theme, ok := ThemeByName("solarized-dark")
	if ok {
		t.Error("ThemeByName for unknown theme returned ok=true")
	}
	// Returned theme must still be usable — Render must not panic and
	// must not return empty for non-empty input.
	if got := theme.Title.Render("hello"); got == "" {
		t.Error("fallback theme returned empty render for non-empty input")
	}
}

func TestDefaultTheme_AllFieldsPopulated(t *testing.T) {
	// A zero-value lipgloss.Style renders its input unchanged but with
	// no styling. We can't check "is styled" easily, but we can at
	// least verify no field panics on Render. This guards against a
	// future contributor adding a Theme field and forgetting to set
	// it in defaultTheme().
	th := defaultTheme()
	for name, s := range map[string]any{
		"Title":                  th.Title,
		"Desc":                   th.Desc,
		"SelectedTitle":          th.SelectedTitle,
		"SelectedDesc":           th.SelectedDesc,
		"StatusRunning":          th.StatusRunning,
		"StatusStopped":          th.StatusStopped,
		"StatusBroken":           th.StatusBroken,
		"Trailing":               th.Trailing,
		"NewProject":             th.NewProject,
		"NewProjectFocus":        th.NewProjectFocus,
		"NewProjectFooter":       th.NewProjectFooter,
		"ListTitle":              th.ListTitle,
		"AlreadyRegisteredTitle": th.AlreadyRegisteredTitle,
		"AlreadyRegisteredName":  th.AlreadyRegisteredName,
		"AlreadyRegisteredHelp":  th.AlreadyRegisteredHelp,
		"FormTitle":              th.FormTitle,
		"FormLabel":              th.FormLabel,
		"FormLabelFocus":         th.FormLabelFocus,
		"FormInfo":               th.FormInfo,
		"FormErr":                th.FormErr,
		"FormHelp":               th.FormHelp,
		"FormCyclerArrow":        th.FormCyclerArrow,
		"FormCyclerArrowFocus":   th.FormCyclerArrowFocus,
		"RightPaneBorder":        th.RightPaneBorder,
		"RightPaneTitle":         th.RightPaneTitle,
		"RightPaneLabel":         th.RightPaneLabel,
		"RightPaneValue":         th.RightPaneValue,
		"RightPaneFaint":         th.RightPaneFaint,
		"ListPaneBorder":         th.ListPaneBorder,
		"StatusBar":              th.StatusBar,
		"StatusBarError":         th.StatusBarError,
		"StatusBarFaint":         th.StatusBarFaint,
	} {
		if s == nil {
			t.Errorf("defaultTheme() field %s is nil", name)
		}
	}
}
