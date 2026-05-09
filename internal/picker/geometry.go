package picker

// Geometry constants for the split-pane layout. Centralised here so
// pane-sizing formulas reference named values instead of bare integers,
// making the intent auditable and future refactors safe.
//
// The layout is:
//
//	┌──────────────────────────┐  ┌──────────────────────┐
//	│ list pane (bordered)     │  │ right pane (bordered) │
//	│  border(1) + pad(1) +    │  │  border(1) + pad(1) + │
//	│  content + pad(1) +      │  │  content + pad(1) +   │
//	│  border(1)               │  │  border(1)            │
//	└──────────────────────────┘  └──────────────────────┘
//	status bar (1 line)

// rightPaneBorderCols is the number of horizontal columns consumed by
// the right pane's rounded border (one column on each side). lipgloss
// Width() sets the content+padding width (border is added on top), so
// the border cost must be subtracted when computing what Width() value
// to pass.
const rightPaneBorderCols = 2

// rightPanePaddingCols is the number of horizontal columns consumed by
// the right pane's Padding(0,1) style (one column of padding on each
// side of the content area).
const rightPanePaddingCols = 2

// rightPaneInnerWidth is the usable content width inside the right pane
// after accounting for its border and padding overhead.
//
// NOTE: picker_helpers.go (package cli) keeps its own local copy of
// this value (const rightPaneInnerWidth = 36) because package-private
// constants are not accessible across package boundaries. The two values
// must be kept in sync if rightPaneWidth, rightPaneBorderCols, or
// rightPanePaddingCols ever change.
const rightPaneInnerWidth = rightPaneWidth - rightPaneBorderCols - rightPanePaddingCols

// listPanePaddingCols is the number of horizontal columns consumed by
// the list pane's Padding(0,1) style (one column on each side). When
// calling lipgloss.Width(innerListWidth + N), N must equal this value
// so the lipgloss-rendered content area matches what list.SetSize was
// told.
const listPanePaddingCols = 2

// borderCorrectionPx is the +1 height bonus added to the right pane's
// lipgloss Height so its bottom border aligns with the list pane's
// visual bottom at runtime. The list pane's content (brand strip +
// bubbles list) renders 1 visual row taller than its declared Height —
// a lipgloss padding/wrap interaction that is not reproducible in unit
// tests but is visible at runtime in real Ghostty panes. Without this
// correction the right pane's bottom border sits 1 row above the list
// pane's, breaking the two-column visual frame.
const borderCorrectionPx = 1
