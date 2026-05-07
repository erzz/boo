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
//   leaf:     { "workingDirectory": "...", "command": "...", "env": {...},
//               "initialInput": "..." }
//   interior: { "direction": "row"|"column", "children": [<split>, <split>] }
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
      if (leaf.command)          cfg.command = leaf.command;
      if (leaf.initialInput)     cfg.initialInput = leaf.initialInput;
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

    // render walks the tree as described in the file header.
    function render(node, term) {
      if (isLeaf(node)) return;
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
      render(left, term);
      render(right, t2);
    }

    let win;
    let windowId;
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
      }

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
