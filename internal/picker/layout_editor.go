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

// interiorRef points at one interior (split) node in a multi-tab layout. Same
// shape as leafRef but indexes against InteriorPointers / Region.NodeIndex.
type interiorRef struct {
	tabIdx       int
	nodeIdxInTab int
	split        *layout.Split
}

// editorMode picks which set of nodes the editor cycles over and which fields
// are bound to keystrokes. Modes are independent — the leaf cursor and divider
// cursor each remember their own position so toggling back and forth doesn't
// reset them.
type editorMode int

const (
	modeLeaf    editorMode = iota // tab cycles leaves, textinput edits Command
	modeDivider                   // tab cycles interior nodes, +/- adjust Size
)

// sizeStep is how much one +/- press changes the first child's share. 0.05
// gives 19 stops between sizeMin and sizeMax — enough granularity for common
// "60/40" or "30/70" layouts without making the user mash the key.
const sizeStep = 0.05

// sizeMin / sizeMax bracket the editable Size range. The validator allows
// (0,1) exclusive; we keep a 5% margin so a one-press accidental adjustment
// doesn't crush a pane to invisibility.
const (
	sizeMin = 0.05
	sizeMax = 0.95
)

// layoutEditorModel is the post-form-submit sub-screen that lets the user
// customise per-pane commands and divider proportions on the resolved layout
// before the project is written. It walks both the layout's leaves and its
// interior nodes in DFS order (one flat sequence per kind, across all tabs);
// the active mode picks which list is being cycled. Other leaf fields
// (cwd, env, initial_input) and structural changes (direction, restructuring)
// are out of scope; users hand-edit YAML for those.
//
// Lifetime: constructed in openLayoutEditor with the resolver's output, then
// either dispatched (Apply) or discarded (Back) — the resolver returns a
// fresh tree each call, so no defensive cloning is needed.
type layoutEditorModel struct {
	projectName  string
	templateName string
	lay          *layout.Layout

	mode editorMode

	leaves     []leafRef
	currentIdx int
	cmdInput   textinput.Model

	interiors   []interiorRef
	interiorIdx int
}

// editorPreviewWidth is the column budget for the embedded layout preview.
// Matches the form's preview width so users see the same shape before/after.
const editorPreviewWidth = 50

// newLayoutEditorModel builds the sub-screen state. Callers must pass a layout
// they own — the editor mutates it in place. An empty `leaves` slice is valid
// (degenerate single-leaf layout still produces one entry); empty `interiors`
// is also valid (single-leaf layouts have no dividers).
func newLayoutEditorModel(projectName, templateName string, lay *layout.Layout) layoutEditorModel {
	leaves := collectLeaves(lay)
	interiors := collectInteriors(lay)
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
		mode:         modeLeaf,
		leaves:       leaves,
		currentIdx:   0,
		cmdInput:     ti,
		interiors:    interiors,
		interiorIdx:  0,
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

// collectInteriors walks every tab in DFS pre-order, recording each interior
// node. Order matches layout.InteriorPointers and the renderer's Region
// NodeIndex sequence so the highlight overlay can find the active interior
// within the active tab.
func collectInteriors(lay *layout.Layout) []interiorRef {
	if lay == nil {
		return nil
	}
	var out []interiorRef
	for ti := range lay.Tabs {
		ptrs := layout.InteriorPointers(&lay.Tabs[ti].Root)
		for ni, p := range ptrs {
			out = append(out, interiorRef{tabIdx: ti, nodeIdxInTab: ni, split: p})
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

// cycle advances by delta (-1/+1) with wraparound. In leaf mode it persists
// the current textinput value first and then loads the new leaf's command into
// the input. In divider mode it simply moves the interior cursor — there is no
// in-flight buffer to flush because Size edits are applied directly.
func (e *layoutEditorModel) cycle(delta int) {
	switch e.mode {
	case modeLeaf:
		if len(e.leaves) <= 1 {
			return
		}
		e.saveCurrent()
		n := len(e.leaves)
		e.currentIdx = (e.currentIdx + delta + n) % n
		e.cmdInput.SetValue(e.leaves[e.currentIdx].split.Command)
		e.cmdInput.CursorEnd()
	case modeDivider:
		if len(e.interiors) <= 1 {
			return
		}
		n := len(e.interiors)
		e.interiorIdx = (e.interiorIdx + delta + n) % n
	}
}

// toggleMode flips between leaf and divider modes. Persists any in-flight
// command edit before leaving leaf mode so the user doesn't lose typed text.
// If the layout has no interiors the toggle still works (the divider view
// will show an empty-state message), but typically users will only encounter
// it in interesting (multi-pane) layouts.
func (e *layoutEditorModel) toggleMode() {
	if e.mode == modeLeaf {
		e.saveCurrent()
		e.mode = modeDivider
		return
	}
	e.mode = modeLeaf
	// Restore the textinput to whatever the active leaf currently holds; the
	// user can't have edited it from divider mode but a future feature might.
	if e.currentIdx >= 0 && e.currentIdx < len(e.leaves) {
		e.cmdInput.SetValue(e.leaves[e.currentIdx].split.Command)
		e.cmdInput.CursorEnd()
	}
}

// bumpSize nudges the active interior's Size by delta, clamped to
// [sizeMin, sizeMax]. A previously-zero Size (meaning "split evenly") is
// promoted to 0.5 before applying the delta so the first press moves
// visibly rather than landing on an off-by-one fraction.
func (e *layoutEditorModel) bumpSize(delta float64) {
	if e.mode != modeDivider {
		return
	}
	if e.interiorIdx < 0 || e.interiorIdx >= len(e.interiors) {
		return
	}
	s := e.interiors[e.interiorIdx].split
	cur := s.Size
	if cur == 0 {
		cur = 0.5
	}
	cur += delta
	if cur < sizeMin {
		cur = sizeMin
	}
	if cur > sizeMax {
		cur = sizeMax
	}
	s.Size = cur
}

// resetSize clears the active interior's Size back to 0 ("split evenly"). The
// JSON tag is `omitempty` so a reset value also drops cleanly out of the YAML.
func (e *layoutEditorModel) resetSize() {
	if e.mode != modeDivider {
		return
	}
	if e.interiorIdx < 0 || e.interiorIdx >= len(e.interiors) {
		return
	}
	e.interiors[e.interiorIdx].split.Size = 0
}

// view renders the editor: header, highlighted layout preview, current-node
// label, mode-specific controls, footer. width is the terminal width; passed
// in so the textinput can scale rather than overflow on narrow terminals.
func (e layoutEditorModel) view(t Theme, width int) string {
	var b strings.Builder
	b.WriteString(t.RightPaneTitle.Render(fmt.Sprintf("Customise layout — %s", e.projectName)))
	b.WriteString("\n")
	b.WriteString(t.RightPaneFaint.Render(fmt.Sprintf("template: %s · mode: %s", e.templateName, modeName(e.mode))))
	b.WriteString("\n\n")

	if len(e.leaves) == 0 {
		b.WriteString(t.RightPaneFaint.Render("(layout has no editable panes)\n\n"))
		b.WriteString(t.RightPaneFaint.Render("[esc] back · [ctrl+s] create"))
		return wrapEditorFrame(b.String())
	}

	// Highlighted preview, one block per tab.
	b.WriteString(e.renderPreview(t))
	b.WriteString("\n")

	switch e.mode {
	case modeLeaf:
		e.renderLeafControls(&b, t, width)
	case modeDivider:
		e.renderDividerControls(&b, t)
	}

	b.WriteString("\n\n")
	b.WriteString(t.RightPaneFaint.Render(
		"[tab/shift+tab] cycle · [ctrl+l] toggle mode · [ctrl+s] save & create · [esc] back to form"))
	return wrapEditorFrame(b.String())
}

// modeName is a short human label for the status header.
func modeName(m editorMode) string {
	switch m {
	case modeDivider:
		return "divider (sizes)"
	default:
		return "leaf (commands)"
	}
}

// renderLeafControls draws the per-leaf label and the command textinput.
// Mutates the receiver's textinput Width as a side effect — same pattern the
// pre-M4 view used; cheap and avoids threading width through the model.
func (e layoutEditorModel) renderLeafControls(b *strings.Builder, t Theme, width int) {
	cur := e.leaves[e.currentIdx]
	label := fmt.Sprintf("Pane %d/%d", e.currentIdx+1, len(e.leaves))
	if len(e.lay.Tabs) > 1 {
		label += fmt.Sprintf(" (tab %d)", cur.tabIdx+1)
	}
	b.WriteString(t.RightPaneTitle.Render(label))
	b.WriteString("\n")

	inputWidth := width - 6
	if inputWidth < 20 {
		inputWidth = 20
	}
	e.cmdInput.Width = inputWidth

	b.WriteString(t.RightPaneLabel.Render("  command (blank = $SHELL):"))
	b.WriteString("\n")
	b.WriteString(e.cmdInput.View())
}

// renderDividerControls draws the per-interior label, current Size as a
// percentage + bar, and the divider keymap. Empty-state when the layout has
// no interiors at all (single-leaf tabs only).
func (e layoutEditorModel) renderDividerControls(b *strings.Builder, t Theme) {
	if len(e.interiors) == 0 {
		b.WriteString(t.RightPaneFaint.Render("(no dividers — layout is all single panes)"))
		return
	}
	cur := e.interiors[e.interiorIdx]
	label := fmt.Sprintf("Divider %d/%d (%s)", e.interiorIdx+1, len(e.interiors), cur.split.Direction)
	if len(e.lay.Tabs) > 1 {
		label += fmt.Sprintf(" (tab %d)", cur.tabIdx+1)
	}
	b.WriteString(t.RightPaneTitle.Render(label))
	b.WriteString("\n")

	frac := cur.split.Size
	explicit := frac != 0
	if !explicit {
		frac = 0.5
	}
	b.WriteString(t.RightPaneLabel.Render("  first child share:"))
	b.WriteString("\n  ")
	b.WriteString(sizeBar(frac, 30))
	fmt.Fprintf(b, "  %d%%", int(frac*100+0.5))
	if !explicit {
		b.WriteString(t.RightPaneFaint.Render("  (default — split evenly)"))
	}
	b.WriteString("\n  ")
	b.WriteString(t.RightPaneFaint.Render("[+/-] adjust 5% · [0] reset to even"))
}

// sizeBar renders a fixed-width bar showing the first child's fractional
// share. The split point is drawn as a vertical bar between the two halves.
func sizeBar(frac float64, width int) string {
	if width < 4 {
		width = 4
	}
	left := int(frac*float64(width) + 0.5)
	if left < 1 {
		left = 1
	}
	if left > width-1 {
		left = width - 1
	}
	right := width - left
	return "[" + strings.Repeat("=", left) + "|" + strings.Repeat("=", right) + "]"
}

// wrapEditorFrame wraps the editor body in the same rounded border as
// setLayout's view, so both sub-screens read consistently.
func wrapEditorFrame(body string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Render(body)
}

// renderPreview renders each tab as its own annotated block. The active node
// (leaf in leaf mode, interior in divider mode) is overlaid in reverse video
// in its tab so the user can see which node the controls below operate on.
func (e layoutEditorModel) renderPreview(t Theme) string {
	activeTab, leafHL, nodeHL := e.activeRegionTargets()
	var blocks []string
	for ti := range e.lay.Tabs {
		tab := e.lay.Tabs[ti]
		h := tabPreviewHeight(tab)
		grid, regions := layoutpreview.RenderTabAnnotated(tab, editorPreviewWidth, h)
		var highlight *layoutpreview.Region
		if ti == activeTab {
			for i := range regions {
				switch {
				case leafHL >= 0 && regions[i].LeafIndex == leafHL:
					r := regions[i]
					highlight = &r
				case nodeHL >= 0 && regions[i].NodeIndex == nodeHL:
					r := regions[i]
					highlight = &r
				}
				if highlight != nil {
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

// activeRegionTargets reports which tab to highlight in, and which kind of
// region to look up. Returns -1 for the kind that isn't active. Returns
// (-1, -1, -1) when there's nothing to highlight (empty leaves / interiors
// in the active mode).
func (e layoutEditorModel) activeRegionTargets() (tabIdx, leafIdx, nodeIdx int) {
	switch e.mode {
	case modeLeaf:
		if len(e.leaves) == 0 {
			return -1, -1, -1
		}
		cur := e.leaves[e.currentIdx]
		return cur.tabIdx, cur.leafIdxInTab, -1
	case modeDivider:
		if len(e.interiors) == 0 {
			return -1, -1, -1
		}
		cur := e.interiors[e.interiorIdx]
		return cur.tabIdx, -1, cur.nodeIdxInTab
	}
	return -1, -1, -1
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
// Apply (ctrl+s): save current input (leaf mode) and dispatch the intent with
// the materialised layout attached. Panes left blank are rendered as plain
// shells by the JXA walker — there is no "skip" path because doing nothing in
// the editor already produces an untouched layout.
//
// Back (esc): drop in-flight edits and return to the form.
//
// Toggle mode (ctrl+l): switch between leaf (commands) and divider (sizes).
//
// Cycle (tab/shift+tab): move between leaves or interiors depending on mode.
//
// Divider mode keys (+ / - / 0): only fire when the editor is in divider mode;
// in leaf mode they fall through to the textinput as plain characters.
//
// Anything else in leaf mode: forwarded to the textinput. Anything else in
// divider mode: ignored.
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
		case matches(m.keys.LayoutEditToggleMode, pressed):
			m.layoutEditor.toggleMode()
			return m, nil
		}
		// Divider-mode-only keys. Gating prevents +/-/0 from being eaten when
		// the user is typing a command in leaf mode.
		if m.layoutEditor.mode == modeDivider {
			switch {
			case matches(m.keys.LayoutEditSizeIncr, pressed):
				m.layoutEditor.bumpSize(+sizeStep)
				return m, nil
			case matches(m.keys.LayoutEditSizeDecr, pressed):
				m.layoutEditor.bumpSize(-sizeStep)
				return m, nil
			case matches(m.keys.LayoutEditSizeReset, pressed):
				m.layoutEditor.resetSize()
				return m, nil
			}
			// Anything else in divider mode is intentionally ignored — there
			// is no textinput to forward to.
			return m, nil
		}
	}
	// Leaf-mode default: forward to the textinput.
	if m.layoutEditor.mode == modeLeaf {
		var cmd tea.Cmd
		m.layoutEditor.cmdInput, cmd = m.layoutEditor.cmdInput.Update(msg)
		return m, cmd
	}
	return m, nil
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
