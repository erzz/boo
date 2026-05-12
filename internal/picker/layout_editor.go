package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/layoutpreview"
)

// leafRef points at one leaf in a multi-tab layout. tabIdx and leafIdxInTab
// are needed for the preview-highlight overlay (regions are reported per-tab),
// while split is the mutation target for command edits.
type leafRef struct {
	tabIdx       int
	leafIdxInTab int
	split        *layout.Split
}

// layoutEditorModel is the post-form-submit sub-screen that lets the user
// customise per-pane commands on the resolved layout before the project is
// written. It walks the layout's leaves in DFS order (one flat sequence
// across all tabs) and exposes a single textinput bound to the active leaf's
// Command. Other leaf fields (cwd, env, initial_input) and divider sizes are
// out of scope for v1; users hand-edit YAML for those.
//
// Lifetime: constructed in openLayoutEditor with the resolver's output, then
// either dispatched (Apply) or discarded (Skip) — the resolver returns a
// fresh tree each call, so no defensive cloning is needed.
type layoutEditorModel struct {
	projectName  string
	templateName string
	lay          *layout.Layout
	leaves       []leafRef
	currentIdx   int
	cmdInput     textinput.Model
}

// editorPreviewWidth is the column budget for the embedded layout preview.
// Matches the form's preview width so users see the same shape before/after.
const editorPreviewWidth = 50

// newLayoutEditorModel builds the sub-screen state. Callers must pass a layout
// they own — the editor mutates it in place. An empty `leaves` slice is valid
// (degenerate single-leaf layout still produces one entry).
func newLayoutEditorModel(projectName, templateName string, lay *layout.Layout) layoutEditorModel {
	leaves := collectLeaves(lay)
	ti := textinput.New()
	ti.Placeholder = "(no command — runs $SHELL)"
	ti.Prompt = "  "
	ti.CharLimit = 1024
	ti.Width = editorPreviewWidth
	if len(leaves) > 0 {
		ti.SetValue(leaves[0].split.Command)
	}
	ti.Focus()
	return layoutEditorModel{
		projectName:  projectName,
		templateName: templateName,
		lay:          lay,
		leaves:       leaves,
		currentIdx:   0,
		cmdInput:     ti,
	}
}

// collectLeaves walks every tab in DFS leaf-order. The flat sequence here is
// the editor's cycler order; per-tab indices are kept on each leafRef so the
// preview overlay can find the right region within the active tab.
func collectLeaves(lay *layout.Layout) []leafRef {
	if lay == nil {
		return nil
	}
	var out []leafRef
	for ti := range lay.Tabs {
		ptrs := layout.LeafPointers(&lay.Tabs[ti].Root)
		for li, p := range ptrs {
			out = append(out, leafRef{tabIdx: ti, leafIdxInTab: li, split: p})
		}
	}
	return out
}

// saveCurrent commits the textinput value to the active leaf's Command. Called
// before any leaf transition (cycle, apply) so edits aren't lost when the user
// moves on without an explicit save.
func (e *layoutEditorModel) saveCurrent() {
	if e.currentIdx < 0 || e.currentIdx >= len(e.leaves) {
		return
	}
	e.leaves[e.currentIdx].split.Command = e.cmdInput.Value()
}

// cycle advances by delta (-1/+1) with wraparound, persisting the current
// textinput value first. Resets the textinput to the new leaf's Command so
// each leaf has its own value rather than sharing one buffer.
func (e *layoutEditorModel) cycle(delta int) {
	if len(e.leaves) <= 1 {
		return
	}
	e.saveCurrent()
	n := len(e.leaves)
	e.currentIdx = (e.currentIdx + delta + n) % n
	e.cmdInput.SetValue(e.leaves[e.currentIdx].split.Command)
	e.cmdInput.CursorEnd()
}

// view renders the editor: header, highlighted layout preview, current-leaf
// label, command input, footer. width is the terminal width; passed in so the
// textinput can scale rather than overflow on narrow terminals.
func (e layoutEditorModel) view(t Theme, width int) string {
	var b strings.Builder
	b.WriteString(t.RightPaneTitle.Render(fmt.Sprintf("Customise layout — %s", e.projectName)))
	b.WriteString("\n")
	b.WriteString(t.RightPaneFaint.Render(fmt.Sprintf("template: %s", e.templateName)))
	b.WriteString("\n\n")

	if len(e.leaves) == 0 {
		b.WriteString(t.RightPaneFaint.Render("(layout has no editable panes)\n\n"))
		b.WriteString(t.RightPaneFaint.Render("[esc] back · [ctrl+s] create"))
		return wrapEditorFrame(b.String())
	}

	// Highlighted preview, one block per tab.
	b.WriteString(e.renderPreview(t))
	b.WriteString("\n")

	// Current-leaf label.
	cur := e.leaves[e.currentIdx]
	label := fmt.Sprintf("Pane %d/%d", e.currentIdx+1, len(e.leaves))
	if len(e.lay.Tabs) > 1 {
		label += fmt.Sprintf(" (tab %d)", cur.tabIdx+1)
	}
	b.WriteString(t.RightPaneTitle.Render(label))
	b.WriteString("\n")

	// Resize the textinput to fit the terminal. Mirrors form.setSize math.
	inputWidth := width - 6
	if inputWidth < 20 {
		inputWidth = 20
	}
	e.cmdInput.Width = inputWidth

	b.WriteString(t.RightPaneLabel.Render("  command (blank = $SHELL):"))
	b.WriteString("\n")
	b.WriteString(e.cmdInput.View())
	b.WriteString("\n\n")

	b.WriteString(t.RightPaneFaint.Render(
		"[tab/shift+tab] cycle pane · [ctrl+s] save & create · [esc] back to form"))
	return wrapEditorFrame(b.String())
}

// wrapEditorFrame wraps the editor body in the same rounded border as
// setLayout's view, so both sub-screens read consistently.
func wrapEditorFrame(body string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Render(body)
}

// renderPreview renders each tab as its own annotated block, with the active
// leaf's region overlaid in reverse video so the user can see which pane the
// command input edits.
func (e layoutEditorModel) renderPreview(t Theme) string {
	cur := e.leaves[e.currentIdx]
	var blocks []string
	for ti := range e.lay.Tabs {
		tab := e.lay.Tabs[ti]
		h := tabPreviewHeight(tab)
		grid, regions := layoutpreview.RenderTabAnnotated(tab, editorPreviewWidth, h)
		var highlight *layoutpreview.Region
		if ti == cur.tabIdx {
			for i := range regions {
				if regions[i].LeafIndex == cur.leafIdxInTab {
					r := regions[i]
					highlight = &r
					break
				}
			}
		}
		body := grid
		if highlight != nil {
			body = overlayHighlight(grid, *highlight, t)
		}
		title := fmt.Sprintf("Tab %d", ti+1)
		if tab.Name != "" {
			title = fmt.Sprintf("Tab %d (%s)", ti+1, tab.Name)
		}
		blocks = append(blocks, t.RightPaneFaint.Render(title)+"\n"+body)
	}
	return strings.Join(blocks, "\n\n") + "\n"
}

// tabPreviewHeight chooses a render height proportional to the tab's column
// depth. Mirrors layoutpreview.RenderLayout's internal sizing so tabs that are
// stacked vertically don't get squashed.
func tabPreviewHeight(tab layout.Tab) int {
	d := columnDepth(tab.Root)
	if d < 1 {
		d = 1
	}
	return d * layoutpreview.MinLeafHeight
}

// columnDepth is a local copy of layoutpreview.columnDepth (unexported there).
// Editor-side because exposing it would be one more point of API drift.
func columnDepth(s layout.Split) int {
	if s.IsLeaf() {
		return 1
	}
	if s.Direction == layout.DirColumn {
		total := 0
		for _, c := range s.Children {
			total += columnDepth(c)
		}
		return total
	}
	max := 0
	for _, c := range s.Children {
		if d := columnDepth(c); d > max {
			max = d
		}
	}
	if max == 0 {
		max = 1
	}
	return max
}

// overlayHighlight re-styles every rune inside region r with reverse video so
// the active pane stands out in the ASCII preview. Operates on the rendered
// string by splitting on '\n'; lipgloss styles each cell independently which
// is enough for plain ASCII output (no ANSI sequences in the input).
func overlayHighlight(grid string, r layoutpreview.Region, _ Theme) string {
	style := lipgloss.NewStyle().Reverse(true)
	lines := strings.Split(grid, "\n")
	for y := r.Y; y < r.Y+r.H && y < len(lines); y++ {
		line := []rune(lines[y])
		var b strings.Builder
		for x, ch := range line {
			if x >= r.X && x < r.X+r.W {
				b.WriteString(style.Render(string(ch)))
			} else {
				b.WriteRune(ch)
			}
		}
		lines[y] = b.String()
	}
	return strings.Join(lines, "\n")
}

// updateLayoutEditor handles input on the editor sub-screen.
//
// Apply (ctrl+s): save current input, attach the materialised layout to the
// pending intent, dispatch via runIntent (which will tea.Quit for NewProjectIntent).
// Panes left blank are rendered as plain shells by the JXA walker — there is
// no "skip" path because doing nothing in the editor already produces an
// untouched layout.
//
// Back (esc): drop the in-flight edits and return to the form. The form's
// state is preserved across the editor screen, so users land back on the
// fields they filled in. From the form they can fix something and resubmit
// (re-entering the editor) or cancel out entirely.
//
// Cycle: tab/shift+tab move between leaves, persisting edits across moves.
//
// Anything else: forwarded to the textinput (typing, cursor movement, etc).
func (m *model) updateLayoutEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		pressed := km.String()
		switch {
		case matches(m.keys.LayoutEditCycleNext, pressed):
			m.layoutEditor.cycle(+1)
			return m, nil
		case matches(m.keys.LayoutEditCyclePrev, pressed):
			m.layoutEditor.cycle(-1)
			return m, nil
		case matches(m.keys.LayoutEditApply, pressed):
			return m.applyLayoutEditor()
		case matches(m.keys.LayoutEditBack, pressed):
			return m.backToFormFromEditor()
		}
	}
	var cmd tea.Cmd
	m.layoutEditor.cmdInput, cmd = m.layoutEditor.cmdInput.Update(msg)
	return m, cmd
}

// applyLayoutEditor finalises the editor: persist the in-flight input value,
// attach the materialised layout to the pending intent, then dispatch.
//
// Defensive: if pendingNewProject is nil (shouldn't happen — set in lockstep
// with screen=screenLayoutEditor), fall back to skipping cleanly so the user
// isn't trapped on the sub-screen.
func (m *model) applyLayoutEditor() (tea.Model, tea.Cmd) {
	if m.pendingNewProject == nil {
		m.screen = screenList
		return m, nil
	}
	m.layoutEditor.saveCurrent()
	intent := *m.pendingNewProject
	intent.MaterialisedLayout = m.layoutEditor.lay
	m.pendingNewProject = nil
	m.layoutEditor = layoutEditorModel{}
	return m.runIntent(intent)
}

// backToFormFromEditor drops the editor state and returns to the form so the
// user can amend their submission. The form has been preserved on the model
// since it was last shown, so this is a true "back" — name/dir/template are
// still populated. The pending intent is discarded; if they re-submit, a fresh
// layout will be resolved and a fresh editor opened.
func (m *model) backToFormFromEditor() (tea.Model, tea.Cmd) {
	m.pendingNewProject = nil
	m.layoutEditor = layoutEditorModel{}
	m.screen = screenForm
	return m, nil
}
