# boo brand

The papa-and-baby-boos mark, in one document.

## Concept

A **papa Ghostty** holds three **baby boos** at his hem. The babies' faces spell out the project name:

- baby 1: **`b`** (a stem and a bowl — the lowercase letter `b` made of eyes)
- baby 2: **`oo`** (two ring eyes — completes the word)
- baby 3: **`^^`** (closed/sleepy eyes — pure personality, doesn't spell anything)

Papa wears a Ghostty `>_` shell prompt as his face, signalling family resemblance to [Ghostty](https://ghostty.org) — boo's host terminal — without being a logo rip.

## Tagline

> a haunt for every project

Used below the mark on the README hero and as a subhead in the TUI's empty state.

## Palette

| token         | hex       | usage                                              |
|---------------|-----------|----------------------------------------------------|
| boo purple    | `#5B5BD6` | papa's outline, prompt face, accent strokes        |
| ink           | `#1a1a1a` | baby outlines, prompt eye/underscore on light bg   |
| paper         | `#ffffff` | papa & baby fills, page background on light themes |

The purple is matched to Ghostty's brand purple as closely as we can read it from public artwork. If Ghostty publishes an exact value, update `docs/brand/boo.svg` and this table together.

## SVG (`docs/brand/boo.svg`)

- `viewBox="0 0 280 340"` — taller than wide, papa-shaped.
- Papa: rounded-top sheet body with a 5-bump wavy hem. Stroke width `9`, fill paper, stroke purple.
- Prompt face: `>` polyline + `_` line, both stroke `9` purple, centred in papa's upper third.
- Babies: ~55w × 75h each, stroke `5` ink, fill paper, 4-bump hem. Positioned at `translate(48,180)`, `(112,185)`, `(176,180)` so they overlap papa's hem by ~15px.
- Faces drawn with stroke `4–5` ink, geometric (rings, stems, polylines) — not freehand.

The SVG is the canonical mark. Everything else is a derivative.

## ASCII variants

The TUI can't render SVG, so the mark exists in two ASCII forms in [`internal/picker/brand.go`](../../internal/picker/brand.go).

### `brandHero` — full mark, 13×7

```
   ╭───────────╮
   │           │
   │    >_     │
   │           │
   │ ╭──┬──┬──╮│
   ╰─┤b │oo│^^├╯
     ╰v─╯v─╯v─╯
```

Papa's body, Ghostty `>_` face, three babies overlapping the hem. Used in the picker's right pane when no projects are registered (the "register your first one" state).

### `brandStrip` — compact 3×10

```
╭──┬──┬──╮
│b │oo│^^│
╰v─╯v─╯v─╯
```

Just the three babies. Used as a header inside the list pane, above the bubbles list title. Auto-suppressed when the inner list pane has fewer than `brandStripMinHeight` rows so it can't starve the items area.

### `brandCursor` — single-cell glyph

```
ᗣ
```

`U+15E3 CANADIAN SYLLABICS NA` — chosen because it renders as a small ghost-shape (rounded top, peaked bottom) in most monospace fonts. Replaces the previous `▌` block as the project-list selection marker. Width is 1 cell + 1 space so column alignment is identical to before.

## Do

- Keep papa visibly Ghostty-adjacent: tall body, wavy hem, `>_` prompt face.
- Keep all three babies present together. They spell `boo` as a unit.
- Use boo purple `#5B5BD6` for papa; ink `#1a1a1a` for babies. Never the other way round.
- Treat the ASCII variants as first-class — they ship in the binary.

## Don't

- Don't drop a baby. Two babies can't spell `boo`.
- Don't replace the `>_` face with eyes. Papa's "family resemblance" comes from the prompt.
- Don't rotate, skew, or recolour papa. He stands upright, paper-on-purple, every time.
- Don't add a fourth baby. Three is the count. Three.

## Where the mark appears

| surface             | asset       | location                                       |
|---------------------|-------------|------------------------------------------------|
| README hero         | SVG         | top of [`README.md`](../../README.md)          |
| TUI empty state     | `brandHero` | right pane when no projects exist              |
| TUI list header     | `brandStrip`| above bubbles list title (when space allows)   |
| TUI selection       | `brandCursor` | every row in the project list                |
