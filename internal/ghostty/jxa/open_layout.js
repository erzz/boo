// Renders a complete boo layout in a new Ghostty window.
//
// Reads from stdin a JSON object of shape:
//
//   {
//     "tabs": [
//       {
//         "name": "edit",
//         "splits": [
//           { "workingDirectory": "/abs/path", "command": "...", "env": {...}, "initialInput": "..." },
//           { "direction": "right", "workingDirectory": "...", ... },
//           ...
//         ]
//       },
//       ...
//     ]
//   }
//
// The first split of the first tab seeds the new window. Each subsequent split
// in a tab is created via app.split(focusedTerminal, ...). Each tab after the
// first is created via app.newTab(...). Returns {"windowId": "..."} on
// success, {"error": "..."} on failure.
//
// Safety: if any step fails after the window has been created, we attempt to
// close the partially-built window so we never leak half-rendered state.
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

    function buildCfg(s) {
      const fields = {};
      if (s.workingDirectory) fields.initialWorkingDirectory = s.workingDirectory;
      if (s.command)          fields.command = s.command;
      if (s.initialInput)     fields.initialInput = s.initialInput;
      if (s.env) {
        const envList = [];
        for (const k of Object.keys(s.env)) envList.push(`${k}=${s.env[k]}`);
        if (envList.length) fields.environmentVariables = envList;
      }
      return app.newSurfaceConfiguration(fields);
    }

    let win;
    let windowId;
    try {
      // 1. Window seeded by first split of first tab.
      const firstTab = tabs[0];
      if (!firstTab.splits || firstTab.splits.length === 0) {
        return JSON.stringify({ error: "layout tab 0 has no splits" });
      }
      win = app.newWindow({ withConfiguration: buildCfg(firstTab.splits[0]) });
      windowId = String(win.id());

      // 2. Additional splits in the first tab.
      for (let s = 1; s < firstTab.splits.length; s++) {
        const sp = firstTab.splits[s];
        if (!sp.direction) throw new Error("non-primary split missing direction (tab 0 split " + s + ")");
        const term = win.tabs[0].focusedTerminal();
        app.split(term, { direction: sp.direction, withConfiguration: buildCfg(sp) });
      }

      // 3. Subsequent tabs.
      for (let t = 1; t < tabs.length; t++) {
        const tab = tabs[t];
        if (!tab.splits || tab.splits.length === 0) {
          throw new Error("tab " + t + " has no splits");
        }
        app.newTab({ in: win, withConfiguration: buildCfg(tab.splits[0]) });
        for (let s = 1; s < tab.splits.length; s++) {
          const sp = tab.splits[s];
          if (!sp.direction) throw new Error("non-primary split missing direction (tab " + t + " split " + s + ")");
          // newTab focuses the new tab; its focused terminal is the seed surface.
          const term = win.tabs[t].focusedTerminal();
          app.split(term, { direction: sp.direction, withConfiguration: buildCfg(sp) });
        }
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
