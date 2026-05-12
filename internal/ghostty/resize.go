package ghostty

// ResizeDeltaPixels returns the pixel delta to feed Ghostty's `resize_split`
// action so that the FIRST child of an interior split occupies `targetFrac`
// of the parent's extent.
//
// At time of resize, Ghostty has just split a pane in half (the only thing
// `app.split` can do), so the current first-child fraction is always 0.5.
// The delta returned represents how many pixels the divider must move from
// that midpoint:
//
//   - positive → grow the first child (move the divider away from it)
//   - negative → shrink the first child (move the divider toward it)
//
// The caller is responsible for translating the sign into the correct
// `resize_split:<direction>` action: focus the leftmost-leaf of the second
// child and pull its near edge in/out accordingly. See open_layout.js.
//
// parentExtentPx is the parent split's relevant dimension in pixels — width
// for row splits, height for column splits. Callers without a real
// measurement should pass the approximate window dimension and accept the
// error; ResizeDeltaPixels itself does not heuristic anything.
//
// Returns 0 when targetFrac is outside (0,1) or when the resulting delta
// would be smaller than 1px (sub-pixel movement is a no-op). A zero return
// is the caller's signal to skip the action entirely.
func ResizeDeltaPixels(parentExtentPx int, targetFrac float64) int {
	if targetFrac <= 0 || targetFrac >= 1 {
		return 0
	}
	if parentExtentPx <= 0 {
		return 0
	}
	delta := (targetFrac - 0.5) * float64(parentExtentPx)
	// Round to nearest int, preserving sign.
	if delta >= 0 {
		d := int(delta + 0.5)
		if d == 0 {
			return 0
		}
		return d
	}
	d := int(delta - 0.5)
	if d == 0 {
		return 0
	}
	return d
}
