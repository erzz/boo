package layout

import (
	"strings"
	"testing"
)

func TestDefault_IsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default invalid: %v", err)
	}
}

func TestParse_RoundTrip(t *testing.T) {
	in := Layout{
		Name: "web",
		Tabs: []Tab{
			{Name: "edit", Splits: []Split{{Cwd: ".", Command: "nvim ."}}},
			{Name: "dev", Splits: []Split{
				{Cwd: "."},
				{Direction: DirRight, Cwd: ".", Command: "npm run dev"},
				{Direction: DirDown, Cwd: "logs", Command: "tail -f app.log"},
			}},
		},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, data)
	}
	if out.Name != in.Name || len(out.Tabs) != 2 || len(out.Tabs[1].Splits) != 3 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name string
		l    Layout
		want string
	}{
		{"no name", Layout{Tabs: []Tab{{Name: "x", Splits: []Split{{}}}}}, "name is required"},
		{"no tabs", Layout{Name: "x"}, "at least one tab"},
		{"no splits", Layout{Name: "x", Tabs: []Tab{{Name: "t"}}}, "at least one split"},
		{"primary with direction", Layout{Name: "x", Tabs: []Tab{{Name: "t", Splits: []Split{{Direction: "right"}}}}}, "primary split must not have a direction"},
		{"bad direction", Layout{Name: "x", Tabs: []Tab{{Name: "t", Splits: []Split{{}, {Direction: "sideways"}}}}}, "is not one of"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.l.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected error containing %q, got %v", c.want, err)
			}
		})
	}
}
