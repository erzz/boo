// Renders a complete boo layout in a new Ghostty window.
//
// Reads from stdin a JSON object of shape:
//
//   {
//     "tabs": [
//       {
//         "name": "edit",
//         "root": <split>
//       },
//       ...
//     ]
//   }
//
// where <split> is recursively either:
//
//   leaf:     { "workingDirectory": "...", "env": {...},
//               "initialInput": "..." }
//   interior: { "direction": "row"|"column", "children": [<split>, <split>],
//               "size": <0..1 fraction for first child, optional> }
//
// Interior nodes always have exactly 2 children — Ghostty's `split` command
// halves a pane and there's no way to ask for a 3-way equal split, so the
// schema (validated Go-side) enforces N=2. Multi-pane shapes are built by
// nesting (e.g. row(A, column(B, C)) gives a 1-on-the-left, 2-stacked-right
// "triple" layout).
//
// Walker contract for `render(node, term)`:
//   - leaf: nothing to do — `term` was created by an ancestor's call to
//     `app.newWindow` / `app.newTab` / `app.split` with this leaf's config.
//   - interior(dir, [a, b]):
//       1. Call `app.split(term, dir, cfg(leftmostLeaf(b)))` → new terminal
//          `t2`. After this `term` occupies the left/top half, `t2` the
//          right/bottom half. `t2` carries b's leftmost-leaf config.
//       2. Recurse into a with `term`. (Any further splits inside a only
//          subdivide the left/top half — t2 is untouched.)
//       3. Recurse into b with `t2`.
//
// After all tabs are rendered and all inputs are flushed, an OPTIONAL resize
// pass honors any `size` fields:
//
//   - Probes the window's pixel bounds via `win.bounds` (Cocoa NSWindow
//     scripting). If unavailable, falls back to a hard-coded approximation
//     so the feature degrades gracefully rather than failing.
//   - For each interior node with `size`, focuses the leftmost-leaf of the
//     SECOND child (b above) and issues `perform action "resize_split:<dir>,N"`
//     where N is computed from `size`, the parent extent in pixels, and the
//     current 50/50 division created by `app.split`.
//   - Walks tree in DFS pre-order so outer dividers move before inner ones;
//     this prevents the cumulative-fraction math from being invalidated by
//     a not-yet-applied outer resize.
//
// Returns {"windowId": "..."} on success, {"error": "..."} on failure. On
// any failure after the window has been created, the JXA helper attempts to
// close the partially-built window so we never leak half-rendered state
// visible to the user.
(() => {
  try {
    ObjC.import('Foundation');
    const handle = $.NSFileHandle.fileHandleWithStandardInput;
    const data = handle.readDataToEndOfFile;
    const raw = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js || "";
    const params = raw.trim().length ? JSON.parse(raw) : {};

    const tabs = Array.isArray(params.tabs) ? params.tabs : [];
    if (tabs.length === 0) return JSON.stringify({ error: "layout has no tabs" });

    const app = Application("Ghostty");
    app.includeStandardAdditions = true;

    function isLeaf(node) {
      return !node || (!node.direction && (!node.children || node.children.length === 0));
    }

    // leftmostLeaf walks down the leftmost / topmost path of the tree
    // until it hits a leaf, returning that leaf node. Used to derive
    // the config for a freshly-split terminal when the corresponding
    // child is itself interior (its real seed terminal is the leaf
    // that will eventually live in its left/top corner).
    function leftmostLeaf(node) {
      let cur = node;
      while (!isLeaf(cur)) {
        cur = cur.children[0];
      }
      return cur;
    }

    function buildCfg(leaf) {
      // IMPORTANT: Ghostty's `new surface configuration` command silently
      // discards properties passed via the constructor's object argument
      // (e.g. `app.newSurfaceConfiguration({initialWorkingDirectory: ...})`).
      // The properties must be assigned to the returned record after
      // creation. This is true as of Ghostty 1.3.x. Re-test if Ghostty's
      // AppleScript dictionary changes.
      const cfg = app.newSurfaceConfiguration();
      if (!leaf) return cfg;
      if (leaf.workingDirectory) cfg.initialWorkingDirectory = leaf.workingDirectory;
      // NOTE: cfg.command and cfg.initialInput are intentionally never set.
      //
      //   - cfg.command launches a stripped-down bash
      //     (`/bin/bash --noprofile --norc -c "exec -l <cmd>"`) that
      //     ignores $SHELL, drops the user's PATH, and exits the surface
      //     when the command exits.
      //   - cfg.initialInput exists in the sdef but in practice nothing
      //     gets typed into the running shell (timing? interaction with
      //     wait-after-command? unclear, but empirically broken in
      //     Ghostty 1.3.x as of this writing).
      //
      // Instead, we collect the desired keystrokes for every leaf during
      // render() and replay them via `app.inputText(text, { to: term })`
      // once the entire window is built and all shells have started.
      // See pendingInputs / flushInputs below.
      if (leaf.env) {
        const envList = [];
        for (const k of Object.keys(leaf.env)) envList.push(`${k}=${leaf.env[k]}`);
        if (envList.length) cfg.environmentVariables = envList;
      }
      return cfg;
    }

    function dirToGhostty(d) {
      // Map boo's row/column to Ghostty's split directions. row =
      // children laid left-to-right → split right. column = stacked
      // top-to-bottom → split down.
      if (d === "row") return "right";
      if (d === "column") return "down";
      throw new Error("invalid split direction: " + String(d));
    }

    // pendingInputs collects { term, text } pairs to be replayed via
    // app.inputText() after the entire window has been built. Capturing
    // them during render() — rather than typing them inline — gives every
    // shell a chance to spawn before we start pasting into it, which
    // avoids races where the input lands before the prompt is ready.
    const pendingInputs = [];

    // recordInput stages a leaf's initialInput for later replay against
    // the terminal it lives on.
    function recordInput(leaf, term) {
      if (leaf && leaf.initialInput && term) {
        pendingInputs.push({ term: term, text: leaf.initialInput });
      }
    }

    // pendingResizes collects { node, term, dir } records for the post-render
    // resize pass. We can't compute pixel deltas during render() because we
    // don't have window bounds yet (the new window may still be laying out).
    // Walk order is DFS pre-order so the outermost dividers come first when
    // applied later — see top-of-file notes for why outer-first matters.
    const pendingResizes = [];

    function recordResize(node, secondChildLeafTerm) {
      if (!node || !node.direction || typeof node.size !== "number") return;
      if (node.size <= 0 || node.size >= 1) return;
      // Direction of the resize_split action: move the SECOND child's near
      // edge in the OPPOSITE direction of the split. row → second child's
      // left edge moves; column → second child's top edge moves. Positive
      // delta from ResizeDeltaPixels means "grow first child"; we map that
      // to "move second child's near edge AWAY from the first child", which
      // is `left` for row and `up` for column.
      const action = node.direction === "row" ? "left" : "up";
      pendingResizes.push({ node: node, term: secondChildLeafTerm, action: action });
    }

    // render walks the tree as described in the file header. As it
    // visits each leaf, it stages that leaf's initialInput against the
    // terminal that materialises it, so flushInputs() can replay the
    // keystrokes once everything is alive. As it visits each interior
    // node, it stages a resize for flushResizes() to apply.
    function render(node, term) {
      if (isLeaf(node)) {
        recordInput(node, term);
        return;
      }
      if (!node.children || node.children.length !== 2) {
        throw new Error("interior node must have exactly 2 children, got " + (node.children ? node.children.length : 0));
      }
      const dir = dirToGhostty(node.direction);
      const left = node.children[0];
      const right = node.children[1];
      // Pre-split: create the right/bottom pane with right's leftmost
      // leaf config. After this, `term` is left/top half, `t2` is
      // right/bottom half.
      const t2 = app.split(term, { direction: dir, withConfiguration: buildCfg(leftmostLeaf(right)) });
      // Record the resize (if any) BEFORE recursing — DFS pre-order on
      // interior nodes is what flushResizes() expects.
      recordResize(node, t2);
      render(left, term);
      render(right, t2);
    }

    // flushInputs replays every staged keystroke into its terminal. We
    // do this after the whole window/all tabs are constructed so each
    // shell has had a moment to print its prompt before we paste.
    function flushInputs() {
      for (let i = 0; i < pendingInputs.length; i++) {
        const p = pendingInputs[i];
        try {
          app.inputText(p.text, { to: p.term });
        } catch (_) { /* best-effort: ignore individual input failures */ }
      }
    }

    // windowExtent returns the window's pixel width or height for the given
    // axis ("row" or "column"). Falls back to a fixed default if bounds is
    // unavailable. The fallback values are deliberately conservative so a
    // missing-bounds environment still produces a visible (if imprecise)
    // resize rather than silently dropping the feature.
    function windowExtent(win, axis) {
      const fallback = axis === "row" ? 1200 : 800;
      try {
        const b = win.bounds();
        if (b && typeof b.width === "number" && typeof b.height === "number") {
          return axis === "row" ? Math.round(b.width) : Math.round(b.height);
        }
      } catch (_) { /* fall through */ }
      return fallback;
    }

    // computeDeltaPx mirrors Go's ghostty.ResizeDeltaPixels — keep in sync.
    // Returns the absolute |delta| in pixels and a flipped flag indicating
    // whether the sign was negative (caller picks an opposite action).
    function computeDeltaPx(parentExtentPx, targetFrac) {
      if (targetFrac <= 0 || targetFrac >= 1) return { abs: 0, flipped: false };
      if (parentExtentPx <= 0) return { abs: 0, flipped: false };
      const delta = (targetFrac - 0.5) * parentExtentPx;
      const rounded = delta >= 0 ? Math.round(delta) : -Math.round(-delta);
      if (rounded === 0) return { abs: 0, flipped: false };
      return { abs: Math.abs(rounded), flipped: rounded < 0 };
    }

    // flippedAction returns the opposite of `action` along its axis.
    // left ↔ right, up ↔ down.
    function flippedAction(action) {
      switch (action) {
        case "left": return "right";
        case "right": return "left";
        case "up": return "down";
        case "down": return "up";
      }
      return action;
    }

    // flushResizes applies every staged resize_split. Per-node parent extent
    // is computed by walking from the tab root to the node, multiplying the
    // window dimension by each ancestor's size on the matching axis (or 0.5
    // when an ancestor has no size). DFS pre-order on pendingResizes means
    // outer dividers move first, but the math here doesn't actually depend
    // on that — the cumulative-product is over PLANNED sizes, which is the
    // post-resize geometry by construction. The pre-order is still preferred
    // because it minimises visual flicker.
    function flushResizes(win, tabRoots) {
      if (pendingResizes.length === 0) return;

      // Build a node→ancestors map per tab. Cheaper than re-walking for
      // each resize; resizes can be many in a deeply-nested layout.
      const ancestors = new Map();
      function indexTree(node, chain) {
        if (!node || isLeaf(node)) return;
        ancestors.set(node, chain);
        const childChain = chain.concat([node]);
        for (let i = 0; i < node.children.length; i++) {
          indexTree(node.children[i], childChain);
        }
      }
      for (let i = 0; i < tabRoots.length; i++) indexTree(tabRoots[i], []);

      const widthPx = windowExtent(win, "row");
      const heightPx = windowExtent(win, "column");

      for (let i = 0; i < pendingResizes.length; i++) {
        const p = pendingResizes[i];
        const baseExtent = p.node.direction === "row" ? widthPx : heightPx;
        let extent = baseExtent;
        const chain = ancestors.get(p.node) || [];
        for (let j = 0; j < chain.length; j++) {
          const a = chain[j];
          // Determine which axis this ancestor cuts along, then which
          // child of the ancestor contains our node, then scale extent
          // by the corresponding fraction. We only shrink extent when
          // the ancestor's axis matches the node's axis OR when the
          // ancestor's axis is irrelevant — actually we always scale,
          // because either axis halves the bounding box for the next
          // level. Easier: every ancestor halves the bounding box (or
          // size/1-size cuts it) on its own axis; on the OTHER axis the
          // bounding box is unchanged. So scale only when ancestor.axis
          // == node.axis is wrong — we need to scale on whichever axis
          // matches our node's chosen extent (row→width, column→height).
          const node = p.node;
          if (axisMatches(a, node)) {
            const frac = childFraction(a, chain[j + 1] || node);
            extent = Math.round(extent * frac);
          }
        }
        const d = computeDeltaPx(extent, p.node.size);
        if (d.abs === 0) continue;
        const action = d.flipped ? flippedAction(p.action) : p.action;
        const cmd = "resize_split:" + action + "," + d.abs;
        try {
          app.perform(cmd, { on: p.term });
        } catch (_) { /* best-effort: a single failed resize shouldn't sink the open */ }
      }
    }

    // axisMatches reports whether ancestor `a` cuts along the same axis
    // (row vs column) as the node we're computing extent for.
    function axisMatches(ancestor, node) {
      return ancestor.direction === node.direction;
    }

    // childFraction returns the fraction of `ancestor`'s extent that
    // `child` occupies. Uses ancestor.size when set, defaulting to 0.5.
    // child is either ancestor.children[0] (fraction = size) or
    // ancestor.children[1] (fraction = 1-size). If the relationship is
    // unclear (shouldn't happen), defaults to 0.5.
    function childFraction(ancestor, child) {
      const sz = (typeof ancestor.size === "number" && ancestor.size > 0 && ancestor.size < 1) ? ancestor.size : 0.5;
      if (ancestor.children[0] === child) return sz;
      if (ancestor.children[1] === child) return 1 - sz;
      return 0.5;
    }

    let win;
    let windowId;
    const tabRoots = [];
    try {
      // 1. Window seeded by the leftmost leaf of the first tab's root.
      const firstTab = tabs[0];
      if (!firstTab.root) {
        return JSON.stringify({ error: "layout tab 0 has no root split" });
      }
      win = app.newWindow({ withConfiguration: buildCfg(leftmostLeaf(firstTab.root)) });
      windowId = String(win.id());

      // 2. Render the first tab's split tree into the seed terminal.
      const firstSeed = win.tabs[0].focusedTerminal();
      render(firstTab.root, firstSeed);
      tabRoots.push(firstTab.root);

      // NOTE on tab.name: Ghostty 1.3.x marks `tab.name` and `terminal.name`
      // as read-only in its AppleScript dictionary. Tab titles are derived
      // from the focused terminal's title, which Ghostty reads from the
      // shell's window-title escape sequence (OSC 0/1/2). There is no
      // `set title` command and `inputText` strips the ESC byte (0x1b),
      // so the only available title-setting action — `prompt_surface_title`
      // — is interactive (opens a dialog), which is unusable here. The
      // `name` field on each tab in the layout YAML is parsed and shipped
      // to JXA but currently ignored. When Ghostty exposes a non-interactive
      // title-setting action, set it here per tab before rendering.

      // 3. Subsequent tabs: each gets a new tab seeded by its own
      //    leftmost leaf, then we render its tree into that seed.
      for (let t = 1; t < tabs.length; t++) {
        const tab = tabs[t];
        if (!tab.root) {
          throw new Error("tab " + t + " has no root split");
        }
        app.newTab({ in: win, withConfiguration: buildCfg(leftmostLeaf(tab.root)) });
        const seed = win.tabs[t].focusedTerminal();
        render(tab.root, seed);
        tabRoots.push(tab.root);
      }

      // 4. All terminals exist; replay each leaf's initialInput as a
      //    paste-style input into its terminal. Done last so every
      //    shell has had time to start before we type into it.
      flushInputs();

      // 5. Apply any size hints via the resize_split action. Best-effort:
      //    a failure here doesn't undo the window, since the layout is
      //    already structurally correct — it just has 50/50 splits.
      flushResizes(win, tabRoots);

      return JSON.stringify({ windowId: windowId });
    } catch (inner) {
      // Best-effort cleanup of the partially-built window.
      if (win) {
        try { app.closeWindow(win); } catch (_) { /* ignore */ }
      }
      return JSON.stringify({ error: String(inner && inner.message ? inner.message : inner) });
    }
  } catch (e) {
    return JSON.stringify({ error: String(e && e.message ? e.message : e) });
  }
})();
